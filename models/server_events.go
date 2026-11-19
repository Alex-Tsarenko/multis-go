package models

// ServerEventTypes типы SSE событий
type ServerEventType string

const (
	EventTypeReasoning  ServerEventType = "reasoning"
	EventTypeMessage    ServerEventType = "message"
	EventTypeToolCall   ServerEventType = "tool_call"
	EventTypeToolResult ServerEventType = "tool_result"
	EventTypeError      ServerEventType = "error"
	EventTypeDone       ServerEventType = "done"
)

// ServerEvent базовая структура SSE события
type ServerEvent struct {
	Type ServerEventType `json:"type"`
	Data interface{}  `json:"data"`
}

// ReasoningData данные события рассуждения
type ReasoningData struct {
	Content string `json:"content"`
}

// MessageData данные события сообщения
type MessageData struct {
	Content string `json:"content"`
}

// ToolCallData данные события вызова инструмента
type ToolCallData struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// ToolResultData данные события результата инструмента
type ToolResultData struct {
	Status string `json:"status"`
	Output string `json:"output"`
}

// ErrorData данные события ошибки
type ErrorData struct {
	Message string `json:"message"`
}

// DoneData данные события завершения
type DoneData struct {
	FinishReason string `json:"finish_reason"`
}

// FinishReason причины завершения
type FinishReason string

const (
	FinishReasonStop         FinishReason = "stop"
	FinishReasonMaxSteps     FinishReason = "max_steps"
	FinishReasonLoopDetected FinishReason = "loop_detected"
	FinishReasonError        FinishReason = "error"
)
