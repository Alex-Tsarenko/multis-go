package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"multis-go/models"
	"net/http"
	"sort"
	"strings"
)

// DeepSeekClient реализация AIClient для DeepSeek API
type DeepSeekClient struct {
	apiKey  string
	model   string
	baseURL string
}

// NewDeepSeekClient создаёт новый DeepSeekClient
func NewDeepSeekClient(apiKey, model string) *DeepSeekClient {
	return &DeepSeekClient{
		apiKey:  apiKey,
		model:   model,
		baseURL: "https://api.deepseek.com/v1",
	}
}

// WithModel возвращает копию клиента, привязанную к указанной модели
func (c *DeepSeekClient) WithModel(model string) AIClient {
	clone := *c
	clone.model = model
	return &clone
}

// openAIToolCall сообщение ассистента с вызовами инструментов (формат OpenAI API)
type openAIToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// openAIMessage сообщение в формате OpenAI Chat Completions API
type openAIMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

// toOpenAIMessages конвертирует сообщения runtime в формат OpenAI API
func toOpenAIMessages(messages []Message) []openAIMessage {
	result := make([]openAIMessage, 0, len(messages))
	for _, m := range messages {
		msg := openAIMessage{Role: m.Role, Content: m.Content}
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			for _, tc := range m.ToolCalls {
				call := openAIToolCall{ID: tc.ID, Type: "function"}
				call.Function.Name = tc.Name
				args := []byte("{}")
				if tc.Arguments != nil {
					if b, err := json.Marshal(tc.Arguments); err == nil {
						args = b
					}
				}
				call.Function.Arguments = string(args)
				msg.ToolCalls = append(msg.ToolCalls, call)
			}
		}
		if m.Role == "tool" {
			msg.ToolCallID = m.ToolCallID
		}
		result = append(result, msg)
	}
	return result
}

// accumulatedToolCall накопленный вызов инструмента из стриминговых дельт
type accumulatedToolCall struct {
	id        string
	name      string
	arguments strings.Builder
}

// StreamCompletion выполняет стриминговый запрос к DeepSeek API
func (c *DeepSeekClient) StreamCompletion(ctx context.Context, messages []Message, tools []ToolSchema, eventChan chan<- models.SSEEvent) (*StreamResponse, error) {
	requestBody := map[string]any{
		"model":    c.model,
		"messages": toOpenAIMessages(messages),
		"stream":   true,
	}
	if len(tools) > 0 {
		requestBody["tools"] = toFunctionTools(tools)
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error: %s", string(body))
	}

	response := &StreamResponse{}
	pendingCalls := make(map[int]*accumulatedToolCall)
	scanner := bufio.NewScanner(resp.Body)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		if line == "data: [DONE]" {
			break
		}
		if len(line) < 6 || line[:6] != "data: " {
			continue
		}

		data := line[6:]
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					Reasoning string `json:"reasoning_content"`
					ToolCalls []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Type     string `json:"type"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
		}

		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		if len(chunk.Choices) == 0 {
			continue
		}

		delta := chunk.Choices[0].Delta

		if delta.Reasoning != "" {
			eventChan <- models.SSEEvent{
				Type: models.EventTypeReasoning,
				Data: models.ReasoningData{Content: delta.Reasoning},
			}
		}

		if delta.Content != "" {
			response.Content += delta.Content
			eventChan <- models.SSEEvent{
				Type: models.EventTypeMessage,
				Data: models.MessageData{Content: delta.Content},
			}
		}

		for _, tc := range delta.ToolCalls {
			call, ok := pendingCalls[tc.Index]
			if !ok {
				call = &accumulatedToolCall{}
				pendingCalls[tc.Index] = call
			}
			if tc.ID != "" {
				call.id = tc.ID
			}
			if tc.Function.Name != "" {
				call.name = tc.Function.Name
			}
			call.arguments.WriteString(tc.Function.Arguments)
		}

		if chunk.Choices[0].FinishReason != "" {
			response.FinishReason = chunk.Choices[0].FinishReason
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	indexes := make([]int, 0, len(pendingCalls))
	for idx := range pendingCalls {
		indexes = append(indexes, idx)
	}
	sort.Ints(indexes)

	for _, idx := range indexes {
		call := pendingCalls[idx]
		toolCall := ToolCall{ID: call.id, Name: call.name}

		argsStr := call.arguments.String()
		toolCall.Arguments = make(map[string]any)
		if argsStr != "" {
			if err := json.Unmarshal([]byte(argsStr), &toolCall.Arguments); err != nil {
				toolCall.Arguments = map[string]any{"_raw_arguments": argsStr}
			}
		}

		response.ToolCalls = append(response.ToolCalls, toolCall)

		eventChan <- models.SSEEvent{
			Type: models.EventTypeToolCall,
			Data: models.ToolCallData{
				ID:        toolCall.ID,
				Name:      toolCall.Name,
				Arguments: toolCall.Arguments,
			},
		}
	}

	return response, nil
}
