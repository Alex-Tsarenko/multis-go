package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"multis-go/models"
	"net/http"
)

// OllamaClient реализация AIClient для Ollama API
type OllamaClient struct {
	baseURL         string
	model           string
	numCtx          int
	configuredModels []string
}

// NewOllamaClient создаёт новый OllamaClient
func NewOllamaClient(baseURL, model string, numCtx int) *OllamaClient {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	if numCtx <= 0 {
		numCtx = 32768
	}
	return &OllamaClient{
		baseURL: baseURL,
		model:   model,
		numCtx:  numCtx,
	}
}

// SetConfiguredModels задает локальный список моделей для провайдера
func (c *OllamaClient) SetConfiguredModels(models []string) {
	c.configuredModels = models
}

// WithModel возвращает копию клиента, привязанную к указанной модели
func (c *OllamaClient) WithModel(model string) AIClient {
	clone := *c
	clone.model = model
	return &clone
}

// ListModels возвращает список установленных моделей в Ollama
func (c *OllamaClient) ListModels(ctx context.Context) ([]string, error) {
	if len(c.configuredModels) > 0 {
		return c.configuredModels, nil
	}
	if cfg := FindProvidersConfig(""); cfg != nil {
		if models := cfg.GetConfiguredModels("ollama"); len(models) > 0 {
			return models, nil
		}
	}

	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/tags", nil)
	if err != nil {
		return c.fallbackModels(), nil
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return c.fallbackModels(), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.fallbackModels(), nil
	}

	var data struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return c.fallbackModels(), nil
	}

	names := make([]string, 0, len(data.Models))
	for _, m := range data.Models {
		if m.Name != "" {
			names = append(names, m.Name)
		}
	}
	if len(names) == 0 {
		return c.fallbackModels(), nil
	}
	return names, nil
}

func (c *OllamaClient) fallbackModels() []string {
	if len(c.configuredModels) > 0 {
		return c.configuredModels
	}
	if cfg := FindProvidersConfig(""); cfg != nil {
		if models := cfg.GetConfiguredModels("ollama"); len(models) > 0 {
			return models
		}
	}
	return nil
}

// UnloadModel выгружает указанную модель Ollama из памяти (GPU/RAM)
func (c *OllamaClient) UnloadModel(ctx context.Context, modelName string) error {
	if modelName == "" {
		modelName = c.model
	}
	if modelName == "" {
		return fmt.Errorf("model name is required")
	}

	requestBody := map[string]any{
		"model":      modelName,
		"keep_alive": 0,
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/generate", bytes.NewReader(jsonBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call ollama: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ollama unload error (%d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// ollamaToolCall вызов инструмента в формате Ollama API
type ollamaToolCall struct {
	Function struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	} `json:"function"`
}

// ollamaMessage сообщение в формате Ollama API
type ollamaMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
	ToolName  string           `json:"tool_name,omitempty"`
}

// toOllamaMessages конвертирует сообщения runtime в формат Ollama API
func toOllamaMessages(messages []Message) []ollamaMessage {
	result := make([]ollamaMessage, 0, len(messages))
	for _, m := range messages {
		msg := ollamaMessage{Role: m.Role, Content: m.Content}
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			for _, tc := range m.ToolCalls {
				var call ollamaToolCall
				call.Function.Name = tc.Name
				call.Function.Arguments = tc.Arguments
				msg.ToolCalls = append(msg.ToolCalls, call)
			}
		}
		if m.Role == "tool" {
			msg.ToolName = m.ToolName
		}
		result = append(result, msg)
	}
	return result
}

// StreamLLMResponse выполняет стриминговый запрос к Ollama API
func (c *OllamaClient) StreamLLMResponse(ctx context.Context, messages []Message, tools map[string]Tool, eventChan chan<- models.ServerEvent) (*StreamResponse, error) {
	requestBody := map[string]any{
		"model":    c.model,
		"messages": toOllamaMessages(messages),
		"stream":   true,
		"options": map[string]any{
			"num_ctx": c.numCtx,
		},
	}
	if len(tools) > 0 {
		requestBody["tools"] = toFunctionTools(tools)
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, err
	}

	log.Printf("[Ollama] Request: %s", string(jsonBody))

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/chat", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("[Ollama] Request error: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	log.Printf("[Ollama] Response status: %d", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("[Ollama] Error response: %s", string(body))
		return nil, fmt.Errorf("API error: %s", string(body))
	}

	response := &StreamResponse{}
	seenToolCalls := make(map[string]bool)
	scanner := bufio.NewScanner(resp.Body)

	lineCount := 0
	for scanner.Scan() {
		line := scanner.Text()
		lineCount++
		if line == "" {
			continue
		}

		log.Printf("[Ollama] Line %d: %s", lineCount, line)

		var chunk struct {
			Message struct {
				Role      string           `json:"role"`
				Content   string           `json:"content"`
				Thinking  string           `json:"thinking"`
				ToolCalls []ollamaToolCall `json:"tool_calls"`
			} `json:"message"`
			Done       bool   `json:"done"`
			DoneReason string `json:"done_reason"`
		}

		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			log.Printf("[Ollama] Parse error on line %d: %v", lineCount, err)
			continue
		}

		log.Printf("[Ollama] Parsed chunk: role=%s, content=%q, thinking=%q, tool_calls=%d, done=%v", chunk.Message.Role, chunk.Message.Content, chunk.Message.Thinking, len(chunk.Message.ToolCalls), chunk.Done)

		if chunk.Message.Thinking != "" {
			eventChan <- models.ServerEvent{
				Type: models.EventTypeReasoning,
				Data: models.ReasoningData{Content: chunk.Message.Thinking},
			}
		}

		if chunk.Message.Content != "" {
			response.Content += chunk.Message.Content
			eventChan <- models.ServerEvent{
				Type: models.EventTypeMessage,
				Data: models.MessageData{Content: chunk.Message.Content},
			}
		}

		for _, tc := range chunk.Message.ToolCalls {
			argsKey, _ := json.Marshal(tc.Function.Arguments)
			dedupeKey := tc.Function.Name + string(argsKey)
			if seenToolCalls[dedupeKey] {
				continue
			}
			seenToolCalls[dedupeKey] = true

			toolCall := ToolCall{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
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

		if chunk.Done {
			response.FinishReason = normalizeFinishReason(chunk.DoneReason)
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("[Ollama] Scanner error: %v", err)
		return nil, err
	}

	log.Printf("[Ollama] Stream completed with %d lines", lineCount)
	return response, nil
}
