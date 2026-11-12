package sandbox

import (
	"context"
	"strings"
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

func TestSafeCommandExecutor_SplitsCommandString(t *testing.T) {
	ce := NewSafeCommandExecutor(".")
	out, err := ce.Execute(context.Background(), "echo hello world", nil, 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(out) != "hello world" {
		t.Errorf("expected 'hello world', got %q", out)
	}
}

func TestSafeCommandExecutor_ShellScriptAllowed(t *testing.T) {
	ce := NewSafeCommandExecutor(".")
	out, err := ce.Execute(context.Background(), "sh -c 'echo hi'", nil, 5*time.Second)
	if err == nil {
		t.Fatalf("expected error for sh -c, got output %q", out)
	}

	out, err = ce.Execute(context.Background(), "sh", []string{"-c", "echo hi"}, 5*time.Second)
	if err == nil {
		t.Fatalf("expected error for sh -c args, got output %q", out)
	}
}

func TestSafeCommandExecutor_BareShellBlocked(t *testing.T) {
	ce := NewSafeCommandExecutor(".")
	if _, err := ce.Execute(context.Background(), "bash", nil, time.Second); err == nil {
		t.Fatal("expected error for bare interactive shell")
	}
}

func TestSafeCommandExecutor_ShellOperators(t *testing.T) {
	ce := NewSafeCommandExecutor(".")
	out, err := ce.Execute(context.Background(), "echo a && echo b | tr a-z A-Z", nil, 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(out) != "a\nB" {
		t.Errorf("expected shell operators to work, got %q", out)
	}
}

func TestSafeCommandExecutor_ArgsAreQuoted(t *testing.T) {
	ce := NewSafeCommandExecutor(".")
	out, err := ce.Execute(context.Background(), "printf %s", []string{"a b"}, 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "a b" {
		t.Errorf("expected 'a b', got %q", out)
	}
}
