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

// Tool псевдоним типа из runtime
type Tool = runtime.Tool

// StreamResponse псевдоним типа из runtime
type StreamResponse = runtime.StreamResponse

// FinishReason псевдоним типа из runtime
type FinishReason = runtime.FinishReason

const (
	FinishReasonStop      = runtime.FinishReasonStop
	FinishReasonToolCalls = runtime.FinishReasonToolCalls
	FinishReasonLength    = runtime.FinishReasonLength
	FinishReasonUnknown   = runtime.FinishReasonUnknown
)

// normalizeFinishReason приводит finish_reason провайдера к внутреннему значению
func normalizeFinishReason(raw string) FinishReason {
	return runtime.NormalizeFinishReason(raw)
}

// AIClient интерфейс для взаимодействия с LLM API
type AIClient interface {
	StreamLLMResponse(ctx context.Context, messages []Message, tools map[string]Tool, eventChan chan<- models.ServerEvent) (*StreamResponse, error)
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
func toFunctionTools(tools map[string]Tool) []map[string]any {
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
