package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"multis-go/models"
)

func TestOpenRouter_StreamCompletion_ToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		chunks := []string{
			`{"choices":[{"delta":{"reasoning":"Let me think about this.","content":"","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"read_file","arguments":""}}]},"finish_reason":null}]}`,
			`{"choices":[{"delta":{"content":"Hello!","tool_calls":[{"index":0,"function":{"arguments":"{\"path\":\"a.txt\"}"}}]},"finish_reason":null}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":1,"id":"call_2","type":"function","function":{"name":"list_directory","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`,
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

	client := &OpenRouterClient{apiKey: "test-key", model: "openai/gpt-4o-mini", baseURL: server.URL}

	tools := map[string]Tool{
		"read_file": {
			Name:        "read_file",
			Description: "Read a file",
			Parameters:  map[string]any{"type": "object"},
		},
	}

	eventChan := make(chan models.ServerEvent, 20)
	resp, err := client.StreamLLMResponse(context.Background(), []Message{{Role: "user", Content: "hi"}}, tools, eventChan)
	if err != nil {
		t.Fatalf("StreamCompletion failed: %v", err)
	}
	close(eventChan)

	if len(resp.ToolCalls) != 2 {
		t.Fatalf("expected 2 tool calls, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].ID != "call_1" || resp.ToolCalls[0].Name != "read_file" {
		t.Fatalf("unexpected first tool call: %+v", resp.ToolCalls[0])
	}
	if resp.ToolCalls[0].Arguments["path"] != "a.txt" {
		t.Errorf("expected path 'a.txt', got %v", resp.ToolCalls[0].Arguments["path"])
	}
	if resp.ToolCalls[1].Name != "list_directory" {
		t.Errorf("unexpected second tool call: %+v", resp.ToolCalls[1])
	}
	if resp.FinishReason != "tool_calls" {
		t.Errorf("expected finish_reason tool_calls, got %q", resp.FinishReason)
	}
}

func TestOpenRouter_StreamCompletion_Reasoning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		chunks := []string{
			`{"choices":[{"delta":{"reasoning":"First thought.","content":""}}]}`,
			`{"choices":[{"delta":{"reasoning":"Second thought.","content":"Thanks"}}]}`,
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

	client := &OpenRouterClient{apiKey: "test-key", model: "openai/gpt-4o-mini", baseURL: server.URL}

	eventChan := make(chan models.ServerEvent, 20)
	if _, err := client.StreamLLMResponse(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil, eventChan); err != nil {
		t.Fatalf("StreamCompletion failed: %v", err)
	}
	close(eventChan)

	var gotReasoning, gotMessage bool
	var gotReasoningContent, gotMessageContent string
	for ev := range eventChan {
		switch ev.Type {
		case models.EventTypeReasoning:
			if rd, ok := ev.Data.(models.ReasoningData); ok {
				gotReasoningContent = rd.Content
				gotReasoning = true
			}
		case models.EventTypeMessage:
			if md, ok := ev.Data.(models.MessageData); ok {
				gotMessageContent = md.Content
				gotMessage = true
			}
		}
	}

	if !gotReasoning {
		t.Error("expected reasoning event")
	}
	if gotReasoningContent != "Second thought." {
		t.Errorf("expected reasoning 'Second thought.', got '%s'", gotReasoningContent)
	}
	if !gotMessage {
		t.Error("expected message event")
	}
	if gotMessageContent != "Thanks" {
		t.Errorf("expected message 'Thanks', got '%s'", gotMessageContent)
	}
}

func TestOpenRouter_ListModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "anthropic/claude-3-opus-20240229"},
				{"id": "anthropic/claude-3-sonnet-20240229"},
				{"id": "openai/gpt-4o-mini"},
			},
		})
	}))
	defer server.Close()

	client := &OpenRouterClient{apiKey: "test-key", model: "test-model", baseURL: server.URL}

	modelsList, err := client.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels failed: %v", err)
	}

	if len(modelsList) != 3 {
		t.Fatalf("expected 3 models, got %d", len(modelsList))
	}
	if modelsList[0] != "anthropic/claude-3-opus-20240229" {
		t.Errorf("first model expected 'anthropic/claude-3-opus-20240229', got '%s'", modelsList[0])
	}
}