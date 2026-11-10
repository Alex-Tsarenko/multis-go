package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"multis-go/models"
	"multis-go/providers"
	"multis-go/runtime"
	"multis-go/sandbox"
)

// Handler HTTP обработчики
type Handler struct {
	sessions       map[string]*SessionState
	sessionsMutex  sync.RWMutex
	fs             sandbox.FileSystem
	ce             sandbox.CommandExecutor
	aiClient       providers.AIClient
	promptHistory  []models.PromptHistoryItem
	promptsMutex   sync.RWMutex
	settings       models.AppSettings
	settingsMutex  sync.RWMutex
	lastOllamaName string
	ollamaMutex    sync.Mutex
}

// SessionState состояние сессии
type SessionState struct {
	Session       models.Session
	CancelFunc    context.CancelFunc
	EventChannels map[chan<- models.SSEEvent]bool
}

// NewHandler создаёт новый Handler
func NewHandler(fs sandbox.FileSystem, ce sandbox.CommandExecutor, client providers.AIClient) *Handler {
	return &Handler{
		sessions:      make(map[string]*SessionState),
		fs:            fs,
		ce:            ce,
		aiClient:      client,
		promptHistory: make([]models.PromptHistoryItem, 0),
		settings: models.AppSettings{
			SecurityMode: models.SecurityModeDefault,
		},
	}
}

// CreateSession создаёт новую сессию
func (h *Handler) CreateSession(w http.ResponseWriter, r *http.Request) {
	log.Printf("[UserAction] POST /session from %s", r.RemoteAddr)
	var req models.CreateSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[UserAction] Failed to decode CreateSession payload: %v", err)
		h.sendError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	maxSteps := req.MaxSteps
	if maxSteps <= 0 {
		maxSteps = 50
	}

	secMode := req.SecurityMode
	if secMode == "" {
		h.settingsMutex.RLock()
		secMode = h.settings.SecurityMode
		h.settingsMutex.RUnlock()
		if secMode == "" {
			secMode = models.SecurityModeDefault
		}
	}

	// Запоминаем последнюю выбранную модель и выгружаем предыдущую ollama-модель при смене
	if req.Model != "" {
		h.settingsMutex.Lock()
		h.settings.LastModel = req.Model
		h.settingsMutex.Unlock()

		h.handleOllamaModelSwitch(r.Context(), req.Model)
	}

	sessionID := generateSessionID()
	log.Printf("[UserAction] Created session %s (model=%s, max_steps=%d, workspace=%s, security_mode=%s)",
		sessionID, req.Model, maxSteps, req.Workspace, secMode)

	session := models.Session{
		ID:           sessionID,
		Model:        req.Model,
		MaxSteps:     maxSteps,
		Workspace:    req.Workspace,
		SecurityMode: secMode,
		Status:       models.StatusIdle,
		CreatedAt:    time.Now(),
	}

	h.sessionsMutex.Lock()
	h.sessions[sessionID] = &SessionState{
		Session:       session,
		EventChannels: make(map[chan<- models.SSEEvent]bool),
	}
	h.sessionsMutex.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(session)
}

// GetSession возвращает информацию о сессии
func (h *Handler) GetSession(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	log.Printf("[UserAction] GET /session/%s from %s", sessionID, r.RemoteAddr)

	h.sessionsMutex.RLock()
	state, ok := h.sessions[sessionID]
	h.sessionsMutex.RUnlock()

	if !ok {
		log.Printf("[UserAction] Session %s not found on GET", sessionID)
		h.sendError(w, http.StatusNotFound, "not_found", "session not found")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(state.Session)
}

// DeleteSession удаляет сессию
func (h *Handler) DeleteSession(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	log.Printf("[UserAction] DELETE /session/%s from %s", sessionID, r.RemoteAddr)

	h.sessionsMutex.Lock()
	defer h.sessionsMutex.Unlock()

	state, ok := h.sessions[sessionID]
	if !ok {
		log.Printf("[UserAction] Session %s not found on DELETE", sessionID)
		h.sendError(w, http.StatusNotFound, "not_found", "session not found")
		return
	}

	if state.CancelFunc != nil {
		log.Printf("[UserAction] Cancelling active context for session %s", sessionID)
		state.CancelFunc()
	}

	for ch := range state.EventChannels {
		close(ch)
		delete(state.EventChannels, ch)
	}

	delete(h.sessions, sessionID)
	log.Printf("[UserAction] Session %s deleted", sessionID)
	w.WriteHeader(http.StatusNoContent)
}

// SendMessage отправляет сообщение в сессию
func (h *Handler) SendMessage(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	log.Printf("[UserAction] POST /session/%s/message from %s", sessionID, r.RemoteAddr)

	var req models.SendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[UserAction] Failed to decode SendMessage payload for session %s: %v", sessionID, err)
		h.sendError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	log.Printf("[UserAction] Session %s received message: %q", sessionID, req.Content)

	// Добавляем в историю промптов
	if req.Content != "" {
		h.promptsMutex.Lock()
		h.promptHistory = append(h.promptHistory, models.PromptHistoryItem{
			ID:        fmt.Sprintf("prompt_%d", time.Now().UnixNano()),
			SessionID: sessionID,
			Prompt:    req.Content,
			CreatedAt: time.Now(),
		})
		h.promptsMutex.Unlock()
	}

	h.sessionsMutex.Lock()
	state, ok := h.sessions[sessionID]
	if !ok {
		h.sessionsMutex.Unlock()
		log.Printf("[UserAction] Session %s not found on SendMessage", sessionID)
		h.sendError(w, http.StatusNotFound, "not_found", "session not found")
		return
	}

	state.Session.Status = models.StatusRunning
	h.sessionsMutex.Unlock()

	go h.runAgent(sessionID, req.Content)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
}

// StreamSession SSE поток сессии
func (h *Handler) StreamSession(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	log.Printf("[Handler] Stream request for session %s", sessionID)

	h.sessionsMutex.RLock()
	state, ok := h.sessions[sessionID]
	h.sessionsMutex.RUnlock()

	if !ok {
		log.Printf("[Handler] Session %s not found", sessionID)
		h.sendError(w, http.StatusNotFound, "not_found", "session not found")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		h.sendError(w, http.StatusInternalServerError, "server_error", "streaming not supported")
		return
	}

	eventChan := make(chan models.SSEEvent, 100)
	h.sessionsMutex.Lock()
	state.EventChannels[eventChan] = true
	h.sessionsMutex.Unlock()

	log.Printf("[Handler] SSE channel opened for session %s", sessionID)

	defer func() {
		h.sessionsMutex.Lock()
		delete(state.EventChannels, eventChan)
		h.sessionsMutex.Unlock()
		close(eventChan)
		log.Printf("[Handler] SSE channel closed for session %s", sessionID)
	}()

	eventCount := 0
	for event := range eventChan {
		eventCount++
		log.Printf("[Handler] Sending SSE event %d: type=%s", eventCount, event.Type)
		data, err := json.Marshal(event.Data)
		if err != nil {
			log.Printf("[Handler] Failed to marshal event data: %v", err)
			continue
		}
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, data)
		flusher.Flush()
	}
	log.Printf("[Handler] Stream ended for session %s, sent %d events", sessionID, eventCount)
}

// StopSession останавливает выполнение
func (h *Handler) StopSession(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	log.Printf("[UserAction] POST /session/%s/stop from %s", sessionID, r.RemoteAddr)

	h.sessionsMutex.Lock()
	state, ok := h.sessions[sessionID]
	if !ok {
		h.sessionsMutex.Unlock()
		log.Printf("[UserAction] Session %s not found on StopSession", sessionID)
		h.sendError(w, http.StatusNotFound, "not_found", "session not found")
		return
	}

	if state.CancelFunc != nil {
		log.Printf("[UserAction] Cancelling running execution for session %s", sessionID)
		state.CancelFunc()
	}

	state.Session.Status = models.StatusStopped
	h.sessionsMutex.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "stopped"})
}

// runAgent запускает агента в отдельной горутине
func (h *Handler) runAgent(sessionID, userMessage string) {
	h.sessionsMutex.RLock()
	state := h.sessions[sessionID]
	h.sessionsMutex.RUnlock()

	ctx, cancel := context.WithCancel(context.Background())
	state.CancelFunc = cancel

	fs, err := sandbox.NewSafeFileSystem(state.Session.Workspace)
	if err != nil {
		h.broadcastEvent(sessionID, models.SSEEvent{
			Type: models.EventTypeError,
			Data: models.ErrorData{Message: err.Error()},
		})
		return
	}

	ce := sandbox.NewSafeCommandExecutor(state.Session.Workspace)
	agent := runtime.NewAgent(fs, ce, h.aiClient, state.Session.MaxSteps)

	eventChan := make(chan models.SSEEvent, 100)
	go func() {
		for event := range eventChan {
			h.broadcastEvent(sessionID, event)
		}
	}()

	err = agent.Run(ctx, sessionID, userMessage, eventChan)
	if err != nil {
		h.broadcastEvent(sessionID, models.SSEEvent{
			Type: models.EventTypeError,
			Data: models.ErrorData{Message: err.Error()},
		})
	}

	h.sessionsMutex.Lock()
	state.Session.Status = models.StatusIdle
	h.sessionsMutex.Unlock()

	close(eventChan)
}

// broadcastEvent отправляет событие всем подключённым SSE клиентам
func (h *Handler) broadcastEvent(sessionID string, event models.SSEEvent) {
	h.sessionsMutex.RLock()
	state, ok := h.sessions[sessionID]
	h.sessionsMutex.RUnlock()

	if !ok {
		return
	}

	for ch := range state.EventChannels {
		select {
		case ch <- event:
		default:
		}
	}
}

// sendError отправляет ошибку
func (h *Handler) sendError(w http.ResponseWriter, status int, errorCode, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(models.ErrorResponse{
		Error:   errorCode,
		Message: message,
	})
}

// GetPromptHistory возвращает историю промптов
func (h *Handler) GetPromptHistory(w http.ResponseWriter, r *http.Request) {
	log.Printf("[UserAction] GET /prompts/history from %s", r.RemoteAddr)

	h.promptsMutex.RLock()
	defer h.promptsMutex.RUnlock()

	limit := 50
	if queryLimit := r.URL.Query().Get("limit"); queryLimit != "" {
		if l, err := fmt.Sscanf(queryLimit, "%d", &limit); err != nil || l != 1 || limit <= 0 {
			limit = 50
		}
	}

	result := make([]models.PromptHistoryItem, 0)
	total := len(h.promptHistory)
	start := 0
	if total > limit {
		start = total - limit
	}
	for i := total - 1; i >= start; i-- {
		result = append(result, h.promptHistory[i])
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// ClearPromptHistory очищает историю промптов
func (h *Handler) ClearPromptHistory(w http.ResponseWriter, r *http.Request) {
	log.Printf("[UserAction] DELETE /prompts/history from %s", r.RemoteAddr)

	h.promptsMutex.Lock()
	h.promptHistory = make([]models.PromptHistoryItem, 0)
	h.promptsMutex.Unlock()

	w.WriteHeader(http.StatusNoContent)
}

// GetSettings возвращает текущие настройки
func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	log.Printf("[UserAction] GET /settings from %s", r.RemoteAddr)

	h.settingsMutex.RLock()
	settings := h.settings
	h.settingsMutex.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(settings)
}

// UpdateSettings обновляет настройки
func (h *Handler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	log.Printf("[UserAction] PUT /settings from %s", r.RemoteAddr)

	var req models.UpdateAppSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	h.settingsMutex.Lock()
	if req.LastModel != nil {
		newModel := *req.LastModel
		h.settings.LastModel = newModel
		h.handleOllamaModelSwitch(r.Context(), newModel)
	}
	if req.SecurityMode != nil {
		h.settings.SecurityMode = *req.SecurityMode
	}
	current := h.settings
	h.settingsMutex.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(current)
}

// UnloadModel выгружает модель из памяти
func (h *Handler) UnloadModel(w http.ResponseWriter, r *http.Request) {
	log.Printf("[UserAction] POST /models/unload from %s", r.RemoteAddr)

	var req models.UnloadModelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Model == "" {
		h.sendError(w, http.StatusBadRequest, "invalid_request", "model is required")
		return
	}

	unloader, ok := h.aiClient.(providers.ModelUnloader)
	if !ok {
		// Если провайдер не поддерживает выгрузку, возвращаем 400
		h.sendError(w, http.StatusBadRequest, "unsupported_provider", "current provider does not support model unloading")
		return
	}

	if err := unloader.UnloadModel(r.Context(), req.Model); err != nil {
		log.Printf("[Handler] Failed to unload model %s: %v", req.Model, err)
		h.sendError(w, http.StatusInternalServerError, "unload_failed", err.Error())
		return
	}

	h.ollamaMutex.Lock()
	if h.lastOllamaName == req.Model {
		h.lastOllamaName = ""
	}
	h.ollamaMutex.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "unloaded",
		"model":  req.Model,
	})
}

// handleOllamaModelSwitch выгружает старую ollama-модель при переключении на новую
func (h *Handler) handleOllamaModelSwitch(ctx context.Context, newModel string) {
	unloader, ok := h.aiClient.(providers.ModelUnloader)
	if !ok {
		return
	}

	h.ollamaMutex.Lock()
	defer h.ollamaMutex.Unlock()

	if h.lastOllamaName != "" && h.lastOllamaName != newModel {
		oldModel := h.lastOllamaName
		log.Printf("[Handler] Ollama model changed from %s to %s, unloading previous model...", oldModel, newModel)
		go func(m string) {
			unloadCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := unloader.UnloadModel(unloadCtx, m); err != nil {
				log.Printf("[Handler] Failed to automatically unload Ollama model %s: %v", m, err)
			} else {
				log.Printf("[Handler] Successfully unloaded Ollama model %s", m)
			}
		}(oldModel)
	}

	h.lastOllamaName = newModel
}

// generateSessionID генерирует ID сессии
func generateSessionID() string {
	return fmt.Sprintf("sess_%d", time.Now().UnixNano())
}
