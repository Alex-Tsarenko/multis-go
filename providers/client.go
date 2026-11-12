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

// ModelLister опциональный интерфейс для провайдеров, умеющих возвращать список доступных моделей
type ModelLister interface {
	ListModels(ctx context.Context) ([]string, error)
}

// ModelUnloader опциональный интерфейс для провайдеров, умеющих выгружать модели из памяти (например, Ollama)
type ModelUnloader interface {
	UnloadModel(ctx context.Context, modelName string) error
}
