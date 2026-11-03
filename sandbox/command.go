package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"time"
)

// CommandExecutor интерфейс для безопасного выполнения команд
type CommandExecutor interface {
	Execute(ctx context.Context, command string, args []string, timeout time.Duration) (string, error)
}

// SafeCommandExecutor безопасная реализация CommandExecutor
type SafeCommandExecutor struct {
	workspace string
}

// NewSafeCommandExecutor создаёт новый SafeCommandExecutor
func NewSafeCommandExecutor(workspace string) *SafeCommandExecutor {
	return &SafeCommandExecutor{workspace: workspace}
}

// Execute выполняет команду с ограничением по времени и буферизацией вывода
func (ce *SafeCommandExecutor) Execute(ctx context.Context, command string, args []string, timeout time.Duration) (string, error) {
	if err := ce.validateCommand(command); err != nil {
		return "", err
	}

	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, command, args...)
	cmd.Dir = ce.workspace

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return fmt.Sprintf("%s%s", stdout.String(), stderr.String()), err
	}

	return stdout.String(), nil
}

// validateCommand проверяет команду на опасность
func (ce *SafeCommandExecutor) validateCommand(command string) error {
	dangerousCommands := []string{
		"vim", "vi", "nano", "emacs",
		"less", "more",
		"bash", "sh", "zsh", "fish",
		"python", "python3", "node",
		"tmux", "screen",
	}

	baseCommand := filepath.Base(command)
	for _, dangerous := range dangerousCommands {
		if baseCommand == dangerous {
			return &CommandValidationError{Command: command, Reason: "interactive or dangerous command not allowed"}
		}
	}

	return nil
}

// CommandValidationError ошибка валидации команды
type CommandValidationError struct {
	Command string
	Reason  string
}

func (e *CommandValidationError) Error() string {
	return "command validation failed: " + e.Reason + " (command: " + e.Command + ")"
}
