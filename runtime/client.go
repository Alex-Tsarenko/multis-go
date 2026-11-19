package runtime

import (
	"context"
	"strings"

	"multis-go/models"
)

// Message сообщение в диалоге агента
type Message struct {
	Role       string
	Content    string
	ToolCalls  []ToolCall // вызовы инструментов ассистента (role=assistant)
	ToolCallID string     // ID вызова инструмента для role=tool (OpenAI-совместимые API)
	ToolName   string     // имя инструмента для role=tool (Ollama)
}

// ToolCall вызов инструмента моделью
type ToolCall struct {
	ID        string
	Name      string
	Arguments map[string]any
}

// FinishReason нормализованная причина завершения ответа LLM
type FinishReason string

const (
	FinishReasonStop      FinishReason = "stop"
	FinishReasonToolCalls FinishReason = "tool_calls"
	FinishReasonLength    FinishReason = "length"
	FinishReasonUnknown   FinishReason = "unknown"
)

// NormalizeFinishReason приводит finish_reason провайдера к внутреннему значению
func NormalizeFinishReason(raw string) FinishReason {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "stop", "end_turn":
		return FinishReasonStop
	case "tool_calls", "tool_use":
		return FinishReasonToolCalls
	case "length", "max_tokens", "max_output_tokens":
		return FinishReasonLength
	default:
		return FinishReasonUnknown
	}
}

// StreamResponse ответ от стриминга модели
type StreamResponse struct {
	FinishReason FinishReason
	Content      string
	ToolCalls    []ToolCall
}

// AIClient интерфейс взаимодействия с LLM, определяемый потребителем (Runtime)
type AIClient interface {
	StreamLLMResponse(ctx context.Context, messages []Message, tools map[string]Tool, eventChan chan<- models.ServerEvent) (*StreamResponse, error)
}
