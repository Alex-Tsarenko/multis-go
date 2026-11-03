package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"multis-go/models"
)

// OllamaClient реализация AIClient для Ollama API
type OllamaClient struct {
	baseURL string
	model   string
}

// NewOllamaClient создаёт новый OllamaClient
func NewOllamaClient(baseURL, model string) *OllamaClient {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	return &OllamaClient{
		baseURL: baseURL,
		model:   model,
	}
}

// StreamCompletion выполняет стриминговый запрос к Ollama API
func (c *OllamaClient) StreamCompletion(ctx context.Context, messages []Message, eventChan chan<- models.SSEEvent) (*StreamResponse, error) {
	requestBody := map[string]any{
		"model":    c.model,
		"messages": messages,
		"stream":   true,
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
				Role     string `json:"role"`
				Content  string `json:"content"`
				Thinking string `json:"thinking"`
			} `json:"message"`
			Done bool `json:"done"`
		}

		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			log.Printf("[Ollama] Parse error on line %d: %v", lineCount, err)
			continue
		}

		log.Printf("[Ollama] Parsed chunk: role=%s, content=%q, thinking=%q, done=%v", chunk.Message.Role, chunk.Message.Content, chunk.Message.Thinking, chunk.Done)

		if chunk.Message.Thinking != "" {
			eventChan <- models.SSEEvent{
				Type: models.EventTypeReasoning,
				Data: models.ReasoningData{Content: chunk.Message.Thinking},
			}
		}

		if chunk.Message.Content != "" {
			eventChan <- models.SSEEvent{
				Type: models.EventTypeMessage,
				Data: models.MessageData{Content: chunk.Message.Content},
			}
		}

		if chunk.Done {
			response.FinishReason = "stop"
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("[Ollama] Scanner error: %v", err)
		return nil, err
	}

	log.Printf("[Ollama] Stream completed with %d lines", lineCount)
	return response, nil
}
