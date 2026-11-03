package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"multis-go/models"
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

// StreamCompletion выполняет стриминговый запрос к DeepSeek API
func (c *DeepSeekClient) StreamCompletion(ctx context.Context, messages []Message, eventChan chan<- models.SSEEvent) (*StreamResponse, error) {
	requestBody := map[string]any{
		"model":    c.model,
		"messages": messages,
		"stream":   true,
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
					Content      string `json:"content"`
					Reasoning    string `json:"reasoning_content"`
					ToolCalls    []struct {
						ID        string          `json:"id"`
						Type      string          `json:"type"`
						Function  json.RawMessage `json:"function"`
					} `json:"tool_calls"`
					FinishReason string `json:"finish_reason"`
				} `json:"delta"`
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
			eventChan <- models.SSEEvent{
				Type: models.EventTypeMessage,
				Data: models.MessageData{Content: delta.Content},
			}
		}

		if len(delta.ToolCalls) > 0 {
			for _, tc := range delta.ToolCalls {
				var functionData struct {
					Name      string         `json:"name"`
					Arguments map[string]any `json:"arguments"`
				}
				json.Unmarshal(tc.Function, &functionData)

				eventChan <- models.SSEEvent{
					Type: models.EventTypeToolCall,
					Data: models.ToolCallData{
						ID:        tc.ID,
						Name:      functionData.Name,
						Arguments: functionData.Arguments,
					},
				}

				response.ToolCalls = append(response.ToolCalls, ToolCall{
					ID:        tc.ID,
					Name:      functionData.Name,
					Arguments: functionData.Arguments,
				})
			}
		}

		if delta.FinishReason != "" {
			response.FinishReason = delta.FinishReason
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return response, nil
}
