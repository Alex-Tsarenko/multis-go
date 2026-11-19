package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
	EventChannels map[chan<- models.ServerEvent]bool
	History       []runtime.Message
	RunMutex      sync.Mutex
}

// NewHandler создаёт новый Handler
func NewHandler(fs sandbox.FileSystem, ce sandbox.CommandExecutor, client providers.AIClient) *Handler {
	h := &Handler{
		sessions:      make(map[string]*SessionState),
		fs:            fs,
		ce:            ce,
		aiClient:      client,
		promptHistory: make([]models.PromptHistoryItem, 0),
		settings: models.AppSettings{
			SecurityMode: models.SecurityModeDefault,
		},
	}
	h.loadPersistedSettings()
	h.loadPersistedPrompts()
	return h
}

// promptsFilePath возвращает путь к файлу истории промптов
func promptsFilePath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "multis-agent", "prompts.json")
}

// loadPersistedPrompts загружает историю промптов из файла пользователя
func (h *Handler) loadPersistedPrompts() {
	path := promptsFilePath()
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var prompts []models.PromptHistoryItem
	if err := json.Unmarshal(data, &prompts); err != nil {
		return
	}
	h.promptsMutex.Lock()
	h.promptHistory = append(h.promptHistory, prompts...)
	h.promptsMutex.Unlock()
}

// persistPrompts сохраняет историю промптов в файл пользователя (хранит последние 500)
func (h *Handler) persistPrompts() {
	path := promptsFilePath()
	if path == "" {
		return
	}
	h.promptsMutex.RLock()
	history := h.promptHistory
	if len(history) > 500 {
		history = history[len(history)-500:]
	}
	data, err := json.Marshal(history)
	h.promptsMutex.RUnlock()
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		log.Printf("[Handler] Failed to persist prompts: %v", err)
	}
}

// settingsFilePath возвращает путь к файлу настроек пользователя
func settingsFilePath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "multis-agent", "settings.json")
}

// loadPersistedSettings загружает настройки из файла пользователя
func (h *Handler) loadPersistedSettings() {
	path := settingsFilePath()
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var saved models.AppSettings
	if err := json.Unmarshal(data, &saved); err != nil {
		return
	}
	h.settingsMutex.Lock()
	if saved.LastModel != "" {
		h.settings.LastModel = saved.LastModel
	}
	if saved.LastWorkspace != "" {
		h.settings.LastWorkspace = saved.LastWorkspace
	}
	if saved.SecurityMode != "" {
		h.settings.SecurityMode = saved.SecurityMode
	}
	h.settingsMutex.Unlock()
}

// persistSettings сохраняет настройки в файл пользователя
func (h *Handler) persistSettings() {
	path := settingsFilePath()
	if path == "" {
		return
	}
	h.settingsMutex.RLock()
	data, err := json.Marshal(h.settings)
	h.settingsMutex.RUnlock()
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		log.Printf("[Handler] Failed to persist settings: %v", err)
	}
}

// SetDefaultModel устанавливает модель по умолчанию
func (h *Handler) SetDefaultModel(model string) {
	h.settingsMutex.Lock()
	defer h.settingsMutex.Unlock()
	if h.settings.LastModel == "" {
		h.settings.LastModel = model
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
	if req.Workspace != "" {
		h.settingsMutex.Lock()
		h.settings.LastWorkspace = req.Workspace
		h.settingsMutex.Unlock()
	}
	h.persistSettings()

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
		Messages:     make([]models.SessionMessage, 0),
		CreatedAt:    time.Now(),
	}

	h.sessionsMutex.Lock()
	h.sessions[sessionID] = &SessionState{
		Session:       session,
		EventChannels: make(map[chan<- models.ServerEvent]bool),
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
		h.persistPrompts()
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
	if req.Model != "" && req.Model != state.Session.Model {
		state.Session.Model = req.Model
		h.handleOllamaModelSwitch(r.Context(), req.Model)
		h.settingsMutex.Lock()
		h.settings.LastModel = req.Model
		h.settingsMutex.Unlock()
		h.persistSettings()
	}
	if req.Workspace != "" && req.Workspace != state.Session.Workspace {
		state.Session.Workspace = req.Workspace
		h.settingsMutex.Lock()
		h.settings.LastWorkspace = req.Workspace
		h.settingsMutex.Unlock()
		h.persistSettings()
	}
	state.Session.Messages = append(state.Session.Messages, models.SessionMessage{
		Role:    "user",
		Content: req.Content,
		Time:    time.Now(),
	})
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

	eventChan := make(chan models.ServerEvent, 100)
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

	// Сериализуем запуски агента внутри сессии и сохраняем историю между сообщениями
	state.RunMutex.Lock()
	defer state.RunMutex.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	state.CancelFunc = cancel

	fs, err := sandbox.NewSafeFileSystem(state.Session.Workspace)
	if err != nil {
		h.broadcastEvent(sessionID, models.ServerEvent{
			Type: models.EventTypeError,
			Data: models.ErrorData{Message: err.Error()},
		})
		return
	}

	ce := sandbox.NewSafeCommandExecutor(state.Session.Workspace)

	// Выбранная в сессии модель имеет приоритет над моделью клиента по умолчанию
	client := h.aiClient
	if state.Session.Model != "" {
		if configurable, ok := client.(providers.ModelConfigurable); ok {
			client = configurable.WithModel(state.Session.Model)
		}
	}

	agent := runtime.NewAgent(fs, ce, client, state.Session.MaxSteps)

	eventChan := make(chan models.ServerEvent, 100)
	var assistantReply strings.Builder
	go func() {
		for event := range eventChan {
			if event.Type == models.EventTypeMessage {
				if msgData, ok := event.Data.(models.MessageData); ok {
					assistantReply.WriteString(msgData.Content)
				}
			}
			h.broadcastEvent(sessionID, event)
		}
	}()

	history := trimHistory(state.History)
	finalMessages, err := agent.Run(ctx, sessionID, history, userMessage, eventChan)
	state.History = finalMessages
	if err != nil {
		h.broadcastEvent(sessionID, models.ServerEvent{
			Type: models.EventTypeError,
			Data: models.ErrorData{Message: err.Error()},
		})
	}

	h.sessionsMutex.Lock()
	state.Session.Status = models.StatusIdle
	if reply := assistantReply.String(); reply != "" {
		state.Session.Messages = append(state.Session.Messages, models.SessionMessage{
			Role:    "assistant",
			Content: reply,
			Time:    time.Now(),
		})
	}
	h.sessionsMutex.Unlock()

	close(eventChan)
}

// broadcastEvent отправляет событие всем подключённым SSE клиентам
func (h *Handler) broadcastEvent(sessionID string, event models.ServerEvent) {
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
	h.persistPrompts()

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
	if req.LastWorkspace != nil {
		h.settings.LastWorkspace = *req.LastWorkspace
	}
	if req.SecurityMode != nil {
		h.settings.SecurityMode = *req.SecurityMode
	}
	current := h.settings
	h.settingsMutex.Unlock()
	h.persistSettings()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(current)
}

// ListModels возвращает список доступных моделей
func (h *Handler) ListModels(w http.ResponseWriter, r *http.Request) {
	log.Printf("[UserAction] GET /models from %s", r.RemoteAddr)

	var modelsList []string
	if lister, ok := h.aiClient.(providers.ModelLister); ok {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		list, err := lister.ListModels(ctx)
		if err != nil {
			log.Printf("[Handler] Failed to list models from provider: %v", err)
		} else {
			modelsList = list
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"models": modelsList,
	})
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

// ListDirectories возвращает список поддиректорий для выбора workspace
func (h *Handler) ListDirectories(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		path = "."
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		h.sendError(w, http.StatusBadRequest, "invalid_path", err.Error())
		return
	}

	entries, err := os.ReadDir(abs)
	if err != nil {
		h.sendError(w, http.StatusBadRequest, "read_failed", err.Error())
		return
	}

	type dirEntry struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	dirs := make([]dirEntry, 0)
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, dirEntry{
				Name: e.Name(),
				Path: filepath.Join(abs, e.Name()),
			})
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Name < dirs[j].Name })

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"path":   abs,
		"parent": filepath.Dir(abs),
		"dirs":   dirs,
	})
}

// maxHistoryMessages ограничивает длину истории, передаваемой модели
const maxHistoryMessages = 200

// trimHistory обрезает историю: сохраняет system-сообщение и последние N сообщений
func trimHistory(history []runtime.Message) []runtime.Message {
	if len(history) <= maxHistoryMessages {
		return history
	}
	trimmed := make([]runtime.Message, 0, maxHistoryMessages)
	if len(history) > 0 && history[0].Role == "system" {
		trimmed = append(trimmed, history[0])
		history = history[1:]
	}
	start := len(history) - (maxHistoryMessages - len(trimmed))
	return append(trimmed, history[start:]...)
}

// generateSessionID генерирует ID сессии
func generateSessionID() string {
	return fmt.Sprintf("sess_%d", time.Now().UnixNano())
}
