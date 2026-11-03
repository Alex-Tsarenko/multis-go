package providers

import (
	"context"

	"multis-go/models"
	"multis-go/runtime"
)

// Message псевдоним типа из runtime для обратной совместимости
type Message = runtime.Message

// ToolCall псевдоним типа из runtime
type ToolCall = runtime.ToolCall

// StreamResponse псевдоним типа из runtime
type StreamResponse = runtime.StreamResponse

// AIClient интерфейс для взаимодействия с LLM API
type AIClient interface {
	StreamCompletion(ctx context.Context, messages []Message, eventChan chan<- models.SSEEvent) (*StreamResponse, error)
}
