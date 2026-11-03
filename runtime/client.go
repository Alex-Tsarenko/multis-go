package runtime

import (
	"context"

	"multis-go/models"
)

// Message сообщение в диалоге агента
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ToolCall вызов инструмента моделью
type ToolCall struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// StreamResponse ответ от стриминга модели
type StreamResponse struct {
	FinishReason string
	ToolCalls    []ToolCall
}

// AIClient интерфейс взаимодействия с LLM, определяемый потребителем (Runtime)
type AIClient interface {
	StreamCompletion(ctx context.Context, messages []Message, eventChan chan<- models.SSEEvent) (*StreamResponse, error)
}

