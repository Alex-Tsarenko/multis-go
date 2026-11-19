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
)

// OpenRouterClient реализация AIClient для OpenRouter API
type OpenRouterClient struct {
	apiKey         string
	model          string
	baseURL        string
	fallbackModels []string
}

// NewOpenRouterClient создаёт новый OpenRouterClient
func NewOpenRouterClient(apiKey, model, baseURL string) *OpenRouterClient {
	if baseURL == "" {
		baseURL = "https://openrouter.ai/api/v1"
	}
	return &OpenRouterClient{
		apiKey:  apiKey,
		model:   model,
		baseURL: baseURL,
	}
}

// SetFallbackModels задает локальный список моделей для провайдера
func (c *OpenRouterClient) SetFallbackModels(models []string) {
	c.fallbackModels = models
}

// WithModel возвращает копию клиента, привязанной к указанной модели
func (c *OpenRouterClient) WithModel(model string) AIClient {
	clone := *c
	clone.model = model
	return &clone
}

// StreamLLMResponse выполняет стриминговый запрос к OpenRouter API
func (c *OpenRouterClient) StreamLLMResponse(ctx context.Context, messages []Message, tools map[string]Tool, eventChan chan<- models.SSEEvent) (*StreamResponse, error) {
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
					Reasoning string `json:"reasoning"`
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

// ListModels возвращает список доступных моделей
func (c *OpenRouterClient) ListModels(ctx context.Context) ([]string, error) {
	if len(c.fallbackModels) > 0 {
		return c.fallbackModels, nil
	}
	if cfg := FindProvidersConfig(""); cfg != nil {
		if models := cfg.GetConfiguredModels("openrouter"); len(models) > 0 {
			return models, nil
		}
	}

	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
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

	var data struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	names := make([]string, 0, len(data.Data))
	for _, m := range data.Data {
		if m.ID != "" {
			names = append(names, m.ID)
		} else if m.Name != "" {
			names = append(names, m.Name)
		}
	}
	return names, nil
}