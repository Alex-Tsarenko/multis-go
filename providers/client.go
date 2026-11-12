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

// ToolSchema псевдоним типа из runtime
type ToolSchema = runtime.ToolSchema

// StreamResponse псевдоним типа из runtime
type StreamResponse = runtime.StreamResponse

// AIClient интерфейс для взаимодействия с LLM API
type AIClient interface {
	StreamCompletion(ctx context.Context, messages []Message, tools []ToolSchema, eventChan chan<- models.SSEEvent) (*StreamResponse, error)
}

// ModelLister опциональный интерфейс для провайдеров, умеющих возвращать список доступных моделей
type ModelLister interface {
	ListModels(ctx context.Context) ([]string, error)
}

// ModelUnloader опциональный интерфейс для провайдеров, умеющих выгружать модели из памяти (например, Ollama)
type ModelUnloader interface {
	UnloadModel(ctx context.Context, modelName string) error
}

// ModelConfigurable опциональный интерфейс для провайдеров, поддерживающих
// создание клиента с другой моделью (per-session model selection)
type ModelConfigurable interface {
	WithModel(model string) AIClient
}

// toFunctionTools формирует массив tools в формате function calling
// (общий для OpenAI-совместимых API и Ollama)
func toFunctionTools(tools []ToolSchema) []map[string]any {
	result := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		result = append(result, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  t.Parameters,
			},
		})
	}
	return result
}
