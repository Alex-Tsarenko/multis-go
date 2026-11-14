package runtime

import (
	"context"

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

// StreamResponse ответ от стриминга модели
type StreamResponse struct {
	FinishReason string
	Content      string
	ToolCalls    []ToolCall
}

// AIClient интерфейс взаимодействия с LLM, определяемый потребителем (Runtime)
type AIClient interface {
	StreamCompletion(ctx context.Context, messages []Message, tools map[string]Tool, eventChan chan<- models.SSEEvent) (*StreamResponse, error)
}
