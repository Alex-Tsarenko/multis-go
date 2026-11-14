package models

import "time"

// CreateSessionRequest запрос на создание сессии
type CreateSessionRequest struct {
	Model        string       `json:"model"`
	MaxSteps     int          `json:"max_steps"`
	Workspace    string       `json:"workspace"`
	SecurityMode SecurityMode `json:"security_mode,omitempty"`
}

// SendMessageRequest запрос на отправку сообщения
type SendMessageRequest struct {
	Content   string `json:"content"`
	Model     string `json:"model,omitempty"`
	Workspace string `json:"workspace,omitempty"`
}

// ErrorResponse ответ с ошибкой
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// PromptHistoryItem элемент истории промптов
type PromptHistoryItem struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id,omitempty"`
	Prompt    string    `json:"prompt"`
	CreatedAt time.Time `json:"created_at"`
}

// AppSettings настройки приложения
type AppSettings struct {
	LastModel     string       `json:"last_model,omitempty"`
	LastWorkspace string       `json:"last_workspace,omitempty"`
	SecurityMode  SecurityMode `json:"security_mode,omitempty"`
}

// UpdateAppSettingsRequest запрос на обновление настроек
type UpdateAppSettingsRequest struct {
	LastModel     *string       `json:"last_model,omitempty"`
	LastWorkspace *string       `json:"last_workspace,omitempty"`
	SecurityMode  *SecurityMode `json:"security_mode,omitempty"`
}

// UnloadModelRequest запрос на выгрузку модели
type UnloadModelRequest struct {
	Model string `json:"model"`
}
