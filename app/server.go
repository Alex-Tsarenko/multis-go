package app

import (
	"net/http"
)

// Server HTTP сервер
type Server struct {
	addr string
	mux  *http.ServeMux
}

// NewServer создаёт новый сервер
func NewServer(addr string) *Server {
	return &Server{
		addr: addr,
		mux:  http.NewServeMux(),
	}
}

// Start запускает сервер
func (s *Server) Start() error {
	return http.ListenAndServe(s.addr, s.mux)
}

// RegisterRoutes регистрирует маршруты API
func (s *Server) RegisterRoutes(handler *Handler) {
	s.mux.HandleFunc("POST /session", handler.CreateSession)
	s.mux.HandleFunc("GET /session/{id}", handler.GetSession)
	s.mux.HandleFunc("DELETE /session/{id}", handler.DeleteSession)
	s.mux.HandleFunc("POST /session/{id}/message", handler.SendMessage)
	s.mux.HandleFunc("GET /session/{id}/stream", handler.StreamSession)
	s.mux.HandleFunc("POST /session/{id}/stop", handler.StopSession)

	// Новые эндпоинты
	s.mux.HandleFunc("GET /prompts/history", handler.GetPromptHistory)
	s.mux.HandleFunc("DELETE /prompts/history", handler.ClearPromptHistory)
	s.mux.HandleFunc("GET /settings", handler.GetSettings)
	s.mux.HandleFunc("PUT /settings", handler.UpdateSettings)
	s.mux.HandleFunc("POST /models/unload", handler.UnloadModel)
}

// RegisterStatic регистрирует статические файлы
func (s *Server) RegisterStatic(pattern string, handler http.Handler) {
	s.mux.Handle(pattern, handler)
}
