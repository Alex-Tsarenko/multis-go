package models

// CreateSessionRequest запрос на создание сессии
type CreateSessionRequest struct {
	Model     string `json:"model"`
	MaxSteps  int    `json:"max_steps"`
	Workspace string `json:"workspace"`
}

// SendMessageRequest запрос на отправку сообщения
type SendMessageRequest struct {
	Content string `json:"content"`
}

// ErrorResponse ответ с ошибкой
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}
