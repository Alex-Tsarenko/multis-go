package runtime_test

import (
	"context"
	"testing"

	"multis-go/models"
	"multis-go/runtime"
)

func TestLoopDetector_DetectsConsecutiveRepeats(t *testing.T) {
	ld := runtime.NewLoopDetector(3)

	args := map[string]any{"path": "main.go"}

	if ld.Check("read_file", args) {
		t.Fatalf("unexpected loop on 1st call")
	}
	if ld.Check("read_file", args) {
		t.Fatalf("unexpected loop on 2nd call")
	}
	if !ld.Check("read_file", args) {
		t.Fatalf("expected loop on 3rd call")
	}
}

func TestLoopDetector_ResetsOnDifferentAction(t *testing.T) {
	ld := runtime.NewLoopDetector(3)

	args1 := map[string]any{"path": "main.go"}
	args2 := map[string]any{"path": "other.go"}

	ld.Check("read_file", args1)
	ld.Check("read_file", args1)
	// Different argument interrupts the streak
	if ld.Check("read_file", args2) {
		t.Fatalf("unexpected loop after argument change")
	}
}

func TestAgent_TerminatesOnLoopDetected(t *testing.T) {
	fs := newMockFileSystem()
	ce := &mockCommandExecutor{}

	// AI repeats the exact same tool call 3 times
	ai := &mockAIClient{
		steps: []*runtime.StreamResponse{
			{
				FinishReason: "tool_calls",
				ToolCalls: []runtime.ToolCall{
					{ID: "1", Name: "read_file", Arguments: map[string]any{"path": "file.txt"}},
				},
			},
			{
				FinishReason: "tool_calls",
				ToolCalls: []runtime.ToolCall{
					{ID: "2", Name: "read_file", Arguments: map[string]any{"path": "file.txt"}},
				},
			},
			{
				FinishReason: "tool_calls",
				ToolCalls: []runtime.ToolCall{
					{ID: "3", Name: "read_file", Arguments: map[string]any{"path": "file.txt"}},
				},
			},
		},
	}

	agent := runtime.NewAgent(fs, ce, ai, 50)
	eventChan := make(chan models.ServerEvent, 20)

	_, err := agent.Run(context.Background(), "session_loop", nil, "read file", eventChan)
	if err != nil {
		t.Fatalf("unexpected agent error: %v", err)
	}
	close(eventChan)

	var loopDetected bool
	for ev := range eventChan {
		if ev.Type == models.EventTypeDone {
			if doneData, ok := ev.Data.(models.DoneData); ok && doneData.FinishReason == string(models.FinishReasonLoopDetected) {
				loopDetected = true
			}
		}
	}

	if !loopDetected {
		t.Errorf("expected finish_reason 'loop_detected' in stream")
	}
}
