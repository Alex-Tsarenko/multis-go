package models

import "time"

// SessionStatus статус сессии
type SessionStatus string

const (
	StatusIdle    SessionStatus = "idle"
	StatusRunning SessionStatus = "running"
	StatusStopped SessionStatus = "stopped"
	StatusError   SessionStatus = "error"
)

// SecurityMode режим безопасности агента
type SecurityMode string

const (
	SecurityModeDefault SecurityMode = "default"
	SecurityModeAsk     SecurityMode = "ask"
	SecurityModeAuto    SecurityMode = "auto"
)

// Session состояние сессии
type Session struct {
	ID           string        `json:"id"`
	Model        string        `json:"model"`
	MaxSteps     int           `json:"max_steps"`
	Workspace    string        `json:"workspace"`
	SecurityMode SecurityMode  `json:"security_mode,omitempty"`
	Status       SessionStatus `json:"status"`
	CreatedAt    time.Time     `json:"created_at"`
}
