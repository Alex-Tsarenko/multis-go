package sandbox

import (
	"context"
	"testing"
	"time"
)

func TestSafeCommandExecutor_BlocksDangerous(t *testing.T) {
	ce := NewSafeCommandExecutor(".")

	dangerousCmds := []string{"bash", "sh", "vim", "nano", "less", "python"}
	for _, cmd := range dangerousCmds {
		_, err := ce.Execute(context.Background(), cmd, nil, 5*time.Second)
		if err == nil {
			t.Errorf("expected error for command %s, got nil", cmd)
		}
	}
}

func TestSafeCommandExecutor_AllowedCommand(t *testing.T) {
	ce := NewSafeCommandExecutor(".")
	out, err := ce.Execute(context.Background(), "echo", []string{"hello"}, 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "hello\n" && out != "hello\r\n" {
		t.Errorf("expected 'hello', got %q", out)
	}
}

