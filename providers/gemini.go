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
	"strings"
)

// GeminiClient реализация AIClient для Google Gemini API (v1beta)
type GeminiClient struct {
	apiKey         string
	model          string
	baseURL        string
	fallbackModels []string
}

// NewGeminiClient создаёт новый GeminiClient
func NewGeminiClient(apiKey, model, baseURL string) *GeminiClient {
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com/v1beta"
	}
	if model == "" {
		model = "gemini-2.5-flash"
	}
	return &GeminiClient{
		apiKey:  apiKey,
		model:   model,
		baseURL: baseURL,
	}
}

// SetFallbackModels задает локальный список моделей для провайдера
func (c *GeminiClient) SetFallbackModels(models []string) {
	c.fallbackModels = models
}

// WithModel возвращает копию клиента, привязанную к указанной модели
func (c *GeminiClient) WithModel(model string) AIClient {
	clone := *c
	clone.model = model
	return &clone
}

// ListModels возвращает список поддерживаемых моделей Gemini
func (c *GeminiClient) ListModels(ctx context.Context) ([]string, error) {
	if len(c.fallbackModels) > 0 {
		return c.fallbackModels, nil
	}
	if cfg := FindProvidersConfig(""); cfg != nil {
		if models := cfg.GetConfiguredModels("gemini"); len(models) > 0 {
			return models, nil
		}
	}
	return []string{
		"gemini-2.5-flash",
		"gemini-2.5-pro",
		"gemini-1.5-flash",
		"gemini-1.5-pro",
	}, nil
}

type geminiPart struct {
	Text             string                  `json:"text,omitempty"`
	FunctionCall     *geminiFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *geminiFunctionResponse `json:"functionResponse,omitempty"`
}

type geminiFunctionCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

type geminiFunctionResponse struct {
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

type geminiContent struct {
	Role  string       `json:"role"`
	Parts []geminiPart `json:"parts"`
}

func toGeminiMessages(messages []Message) []geminiContent {
	result := make([]geminiContent, 0, len(messages))
	for _, m := range messages {
		role := m.Role
		if role == "assistant" {
			role = "model"
		} else if role == "system" || role == "tool" {
			role = "user"
		}

		var parts []geminiPart
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			if m.Content != "" {
				parts = append(parts, geminiPart{Text: m.Content})
			}
			for _, tc := range m.ToolCalls {
				parts = append(parts, geminiPart{
					FunctionCall: &geminiFunctionCall{
						Name: tc.Name,
						Args: tc.Arguments,
					},
				})
			}
		} else if m.Role == "tool" {
			respMap := map[string]any{"output": m.Content}
			if err := json.Unmarshal([]byte(m.Content), &respMap); err != nil {
				respMap = map[string]any{"result": m.Content}
			}
			parts = append(parts, geminiPart{
				FunctionResponse: &geminiFunctionResponse{
					Name:     m.ToolName,
					Response: respMap,
				},
			})
		} else {
			parts = append(parts, geminiPart{Text: m.Content})
		}

		result = append(result, geminiContent{
			Role:  role,
			Parts: parts,
		})
	}
	return result
}

func toGeminiTools(tools map[string]Tool) []map[string]any {
	var declarations []map[string]any
	for _, t := range tools {
		declarations = append(declarations, map[string]any{
			"name":        t.Name,
			"description": t.Description,
			"parameters":  t.Parameters,
		})
	}
	return []map[string]any{
		{
			"functionDeclarations": declarations,
		},
	}
}

func (c *GeminiClient) StreamLLMResponse(ctx context.Context, messages []Message, tools map[string]Tool, eventChan chan<- models.ServerEvent) (*StreamResponse, error) {
	requestBody := map[string]any{
		"contents": toGeminiMessages(messages),
	}
	if len(tools) > 0 {
		requestBody["tools"] = toGeminiTools(tools)
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/models/%s:streamGenerateContent?alt=sse", c.baseURL, c.model)
	if c.apiKey != "" {
		if strings.Contains(url, "?") {
			url += "&key=" + c.apiKey
		} else {
			url += "?key=" + c.apiKey
		}
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("x-goog-api-key", c.apiKey)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Gemini API error (status %d): %s", resp.StatusCode, string(body))
	}

	response := &StreamResponse{
		FinishReason: FinishReasonStop,
	}
	scanner := bufio.NewScanner(resp.Body)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		if len(line) < 6 || line[:6] != "data: " {
			continue
		}
		data := line[6:]

		var chunk struct {
			Candidates []struct {
				Content struct {
					Parts []struct {
						Text         string              `json:"text"`
						FunctionCall *geminiFunctionCall `json:"functionCall"`
					} `json:"parts"`
					Role string `json:"role"`
				} `json:"content"`
				FinishReason string `json:"finishReason"`
			} `json:"candidates"`
		}

		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		for _, cand := range chunk.Candidates {
			if cand.FinishReason != "" {
				response.FinishReason = normalizeFinishReason(cand.FinishReason)
			}
			for _, part := range cand.Content.Parts {
				if part.Text != "" {
					response.Content += part.Text
					eventChan <- models.ServerEvent{
						Type: models.EventTypeMessage,
						Data: models.MessageData{Content: part.Text},
					}
				}
				if part.FunctionCall != nil {
					fc := part.FunctionCall
					toolCall := ToolCall{
						Name:      fc.Name,
						Arguments: fc.Args,
					}
					response.ToolCalls = append(response.ToolCalls, toolCall)
					eventChan <- models.ServerEvent{
						Type: models.EventTypeToolCall,
						Data: models.ToolCallData{
							Name:      toolCall.Name,
							Arguments: toolCall.Arguments,
						},
					}
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return response, nil
}
