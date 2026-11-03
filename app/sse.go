package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// SSEWriter вспомогательные функции для SSE
type SSEWriter struct {
	w       io.Writer
	flusher http.Flusher
}

// NewSSEWriter создаёт новый SSEWriter
func NewSSEWriter(w http.ResponseWriter) (*SSEWriter, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("streaming not supported")
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	return &SSEWriter{
		w:       w,
		flusher: flusher,
	}, nil
}

// WriteEvent записывает SSE событие
func (s *SSEWriter) WriteEvent(eventType string, data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", eventType, jsonData)
	if err != nil {
		return err
	}
	s.flusher.Flush()
	return nil
}

// WriteComment записывает SSE комментарий
func (s *SSEWriter) WriteComment(comment string) error {
	_, err := fmt.Fprintf(s.w, ": %s\n\n", comment)
	if err != nil {
		return err
	}
	s.flusher.Flush()
	return nil
}
