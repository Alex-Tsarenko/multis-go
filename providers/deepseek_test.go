package providers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"multis-go/models"
)

func TestDeepSeek_StreamLLMResponse_ToolCalls(t *testing.T) {
	var gotBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &gotBody)

		w.Header().Set("Content-Type", "text/event-stream")
		chunks := []string{
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"read_file","arguments":""}}]},"finish_reason":null}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"pa"}}]},"finish_reason":null}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"th\":\"a.txt\"}"}}]},"finish_reason":null}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":1,"id":"call_2","type":"function","function":{"name":"list_directory","arguments":"{\"path\":\".\"}"}}]},"finish_reason":"tool_calls"}]}`,
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

	client := &DeepSeekClient{apiKey: "test-key", model: "deepseek-chat", baseURL: server.URL}

	tools := map[string]Tool{
		"read_file": {
			Name:        "read_file",
			Description: "Read a file",
			Parameters:  map[string]any{"type": "object"},
		},
	}

	eventChan := make(chan models.SSEEvent, 20)
	resp, err := client.StreamLLMResponse(context.Background(), []Message{{Role: "user", Content: "hi"}}, tools, eventChan)
	close(eventChan)
	if err != nil {
		t.Fatalf("StreamCompletion failed: %v", err)
	}

	if len(resp.ToolCalls) != 2 {
		t.Fatalf("expected 2 tool calls, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].ID != "call_1" || resp.ToolCalls[0].Name != "read_file" {
		t.Errorf("unexpected first tool call: %+v", resp.ToolCalls[0])
	}
	if resp.ToolCalls[0].Arguments["path"] != "a.txt" {
		t.Errorf("expected accumulated arguments path=a.txt, got %v", resp.ToolCalls[0].Arguments)
	}
	if resp.ToolCalls[1].Name != "list_directory" {
		t.Errorf("unexpected second tool call: %+v", resp.ToolCalls[1])
	}
	if resp.FinishReason != "tool_calls" {
		t.Errorf("expected finish_reason tool_calls, got %q", resp.FinishReason)
	}

	// Проверяем, что tools и messages ушли в правильном формате
	sentTools, ok := gotBody["tools"].([]any)
	if !ok || len(sentTools) != 1 {
		t.Fatalf("expected 1 tool in request, got %v", gotBody["tools"])
	}
	toolFn := sentTools[0].(map[string]any)["function"].(map[string]any)
	if toolFn["name"] != "read_file" {
		t.Errorf("expected tool name read_file, got %v", toolFn["name"])
	}
}

func TestDeepSeek_ListModels(t *testing.T) {
	client := NewDeepSeekClient("test-key", "deepseek-chat")
	models, err := client.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels failed: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("expected at least 1 model")
	}
}

