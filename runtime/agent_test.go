package runtime_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"multis-go/models"
	"multis-go/runtime"
)

type mockFileSystem struct {
	files map[string][]byte
}

func newMockFileSystem() *mockFileSystem {
	return &mockFileSystem{files: make(map[string][]byte)}
}

func (m *mockFileSystem) Read(ctx context.Context, path string) ([]byte, error) {
	data, ok := m.files[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return data, nil
}

func (m *mockFileSystem) Write(ctx context.Context, path string, data []byte) error {
	m.files[path] = data
	return nil
}

func (m *mockFileSystem) Exists(ctx context.Context, path string) (bool, error) {
	_, ok := m.files[path]
	return ok, nil
}

func (m *mockFileSystem) List(ctx context.Context, path string) ([]string, error) {
	keys := make([]string, 0, len(m.files))
	for k := range m.files {
		keys = append(keys, k)
	}
	return keys, nil
}

func (m *mockFileSystem) Metadata(ctx context.Context, path string) (os.FileInfo, error) {
	return nil, nil
}

type mockCommandExecutor struct {
	lastCmd string
}

func (m *mockCommandExecutor) Execute(ctx context.Context, command string, args []string, timeout time.Duration) (string, error) {
	m.lastCmd = command
	return "mock output", nil
}

type mockAIClient struct {
	calls []runtime.Message
	steps []*runtime.StreamResponse
	index int
}

func (m *mockAIClient) StreamLLMResponse(ctx context.Context, messages []runtime.Message, tools map[string]runtime.Tool, eventChan chan<- models.SSEEvent) (*runtime.StreamResponse, error) {
	m.calls = append(m.calls, messages...)
	if m.index >= len(m.steps) {
		return &runtime.StreamResponse{FinishReason: "stop"}, nil
	}
	res := m.steps[m.index]
	m.index++
	return res, nil
}

func TestAgent_ReActLoop_WithToolCalls(t *testing.T) {
	fs := newMockFileSystem()
	ce := &mockCommandExecutor{}

	ai := &mockAIClient{
		steps: []*runtime.StreamResponse{
			{
				FinishReason: "tool_calls",
				ToolCalls: []runtime.ToolCall{
					{
						ID:   "call_1",
						Name: "write_file",
						Arguments: map[string]any{
							"path":    "hello.txt",
							"content": "test file content",
						},
					},
				},
			},
			{
				FinishReason: "stop",
			},
		},
	}

	agent := runtime.NewAgent(fs, ce, ai, 5)

	eventChan := make(chan models.SSEEvent, 20)
	_, err := agent.Run(context.Background(), "test_session", nil, "write hello.txt", eventChan)
	if err != nil {
		t.Fatalf("agent run failed: %v", err)
	}
	close(eventChan)

	var foundToolResult bool
	var foundDone bool
	for ev := range eventChan {
		if ev.Type == models.EventTypeToolResult {
			foundToolResult = true
		}
		if ev.Type == models.EventTypeDone {
			foundDone = true
		}
	}

	if !foundToolResult {
		t.Errorf("expected tool_result event in stream")
	}
	if !foundDone {
		t.Errorf("expected done event in stream")
	}

	content, err := fs.Read(context.Background(), "hello.txt")
	if err != nil {
		t.Fatalf("file hello.txt was not created: %v", err)
	}
	if string(content) != "test file content" {
		t.Errorf("expected 'test file content', got %s", string(content))
	}
}

func TestAgent_PreservesHistoryAcrossRuns(t *testing.T) {
	fs := newMockFileSystem()
	ce := &mockCommandExecutor{}

	ai := &mockAIClient{
		steps: []*runtime.StreamResponse{
			{FinishReason: "stop", Content: "first answer"},
			{FinishReason: "stop", Content: "second answer"},
		},
	}

	agent := runtime.NewAgent(fs, ce, ai, 5)
	eventChan := make(chan models.SSEEvent, 20)

	history, err := agent.Run(context.Background(), "s", nil, "first question", eventChan)
	if err != nil {
		t.Fatalf("first run failed: %v", err)
	}

	// Второй запуск должен видеть историю первого
	callsBefore := len(ai.calls)
	_, err = agent.Run(context.Background(), "s", history, "second question", eventChan)
	if err != nil {
		t.Fatalf("second run failed: %v", err)
	}

	secondCallMessages := ai.calls[callsBefore:]
	var foundFirst, foundSecond bool
	for _, m := range secondCallMessages {
		if m.Role == "user" && m.Content == "first question" {
			foundFirst = true
		}
		if m.Role == "assistant" && m.Content == "first answer" {
			foundSecond = true
		}
	}
	if !foundFirst || !foundSecond {
		t.Errorf("second run lost history: first_question=%v first_answer=%v", foundFirst, foundSecond)
	}
}

func TestPromptExecutionContext_TruncatesOutput(t *testing.T) {
	cm := runtime.NewPromptExecutionContext(nil)
	cm.Initialize("hi", nil)

	hugeOutput := strings.Repeat("A", 12000)
	cm.AddToolResult("call1", "tool", hugeOutput, nil)

	msgs := cm.GetMessages()
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	lastMsg := msgs[2].Content
	if len(lastMsg) > 10100 {
		t.Errorf("output was not truncated, length is %d", len(lastMsg))
	}
	if !strings.HasSuffix(lastMsg, "... (truncated)") {
		t.Errorf("missing truncation suffix")
	}
}
