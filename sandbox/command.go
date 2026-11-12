package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
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

// Execute выполняет команду с ограничением по времени и буферизацией вывода.
// Команда выполняется через sh -c, поэтому поддерживаются shell-конструкции
// (&&, |, редиректы, кавычки), которые модели естественно генерируют.
func (ce *SafeCommandExecutor) Execute(ctx context.Context, command string, args []string, timeout time.Duration) (string, error) {
	if strings.TrimSpace(command) == "" {
		return "", fmt.Errorf("empty command")
	}

	fields := strings.Fields(command)
	name := filepath.Base(fields[0])

	// Для валидации шеллов считаем хвост command-строки частью аргументов
	shellArgs := make([]string, 0, len(fields)-1+len(args))
	shellArgs = append(shellArgs, fields[1:]...)
	shellArgs = append(shellArgs, args...)

	if err := ce.validateCommand(name, shellArgs); err != nil {
		return "", err
	}

	var cmdLine strings.Builder
	cmdLine.WriteString(command)
	for _, arg := range args {
		cmdLine.WriteByte(' ')
		cmdLine.WriteString(shellQuote(arg))
	}

	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "sh", "-c", cmdLine.String())
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

// shellQuote экранирует аргумент для безопасной подстановки в sh -c
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

var (
	// interactiveCommands полностью запрещены (интерактивный UI или REPL)
	interactiveCommands = []string{
		"vim", "vi", "nano", "emacs",
		"less", "more",
		"tmux", "screen",
	}
	// blockedCommands интерпретаторы, запускающие произвольный код
	blockedCommands = []string{
		"python", "python3", "node",
	}
	// shellCommands разрешены только для запуска скриптов (без -c/-i)
	shellCommands = []string{
		"bash", "sh", "zsh", "fish",
	}
)

// validateCommand проверяет команду на опасность
func (ce *SafeCommandExecutor) validateCommand(name string, args []string) error {
	for _, blocked := range interactiveCommands {
		if name == blocked {
			return &CommandValidationError{Command: name, Reason: "interactive or dangerous command not allowed"}
		}
	}
	for _, blocked := range blockedCommands {
		if name == blocked {
			return &CommandValidationError{Command: name, Reason: "interactive or dangerous command not allowed"}
		}
	}

	if isShellCommand(name) {
		return validateShellUsage(name, args)
	}

	return nil
}

func isShellCommand(name string) bool {
	for _, shell := range shellCommands {
		if name == shell {
			return true
		}
	}
	return false
}

// validateShellUsage разрешает запуск скрипта через шелл (sh script.sh args),
// но запрещает флаги -c/-i до имени скрипта и голый интерактивный шелл
func validateShellUsage(command string, args []string) error {
	scriptSeen := false
	for _, arg := range args {
		if scriptSeen {
			continue
		}
		if strings.HasPrefix(arg, "-") {
			if isDangerousShellFlag(arg) {
				return &CommandValidationError{Command: command, Reason: "shell command execution flags (-c/-i) not allowed"}
			}
			continue
		}
		scriptSeen = true
	}
	if !scriptSeen {
		return &CommandValidationError{Command: command, Reason: "interactive shell not allowed"}
	}
	return nil
}

// isDangerousShellFlag определяет флаги шелла, выполняющие код или открывающие интерактивный режим
func isDangerousShellFlag(arg string) bool {
	if arg == "-c" || arg == "-i" || arg == "--command" {
		return true
	}
	// комбинированные короткие флаги вида -xc, -ic
	if strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") {
		return strings.ContainsAny(arg[1:], "ci")
	}
	return false
}

// CommandValidationError ошибка валидации команды
type CommandValidationError struct {
	Command string
	Reason  string
}

func (e *CommandValidationError) Error() string {
	return "command validation failed: " + e.Reason + " (command: " + e.Command + ")"
}
