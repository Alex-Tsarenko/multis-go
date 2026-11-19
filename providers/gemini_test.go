package providers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"multis-go/models"
)

func TestGemini_StreamLLMResponse_ToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		chunks := []string{
			`{"candidates":[{"content":{"parts":[{"text":"Thinking..."}],"role":"model"}}]}`,
			`{"candidates":[{"content":{"parts":[{"functionCall":{"name":"read_file","args":{"path":"test.txt"}}],"role":"model"},"finishReason":"STOP"}]}`,
			`data: [DONE]`,
		}
		for _, c := range chunks {
			if strings.HasPrefix(c, "data:") {
				w.Write([]byte(c + "\n\n"))
			} else {
				w.Write([]byte("data: " + c + "\n\n"))
			}
		}
	}))
	defer server.Close()

	client := NewGeminiClient("test-key", "gemini-2.5-flash", server.URL)

	tools := map[string]Tool{
		"read_file": {
			Name:        "read_file",
			Description: "Read file",
			Parameters:  map[string]any{"type": "object"},
		},
	}

	eventChan := make(chan models.ServerEvent, 20)
	resp, err := client.StreamLLMResponse(context.Background(), []Message{{Role: "user", Content: "read file"}}, tools, eventChan)
	close(eventChan)
	if err != nil {
		t.Fatalf("StreamLLMResponse failed: %v", err)
	}

	if resp.Content != "Thinking..." {
		t.Errorf("expected content 'Thinking...', got %q", resp.Content)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "read_file" {
		t.Errorf("expected tool name 'read_file', got %q", resp.ToolCalls[0].Name)
	}
	if resp.ToolCalls[0].Arguments["path"] != "test.txt" {
		t.Errorf("expected argument path='test.txt', got %v", resp.ToolCalls[0].Arguments["path"])
	}
}

func TestGemini_ListModels(t *testing.T) {
	client := NewGeminiClient("test-key", "gemini-2.5-flash", "")
	modelsList, err := client.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels failed: %v", err)
	}
	if len(modelsList) == 0 {
		t.Fatal("expected non-empty model list")
	}
}
