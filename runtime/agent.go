package runtime

import (
	"context"
	"fmt"
	"log"

	"multis-go/models"
	"multis-go/sandbox"
)

// Tool описание инструмента для агента
type Tool struct {
	Name        string
	Description string
	Parameters  map[string]any
	Execute     func(ctx context.Context, args map[string]any) (string, error)
}

// Agent агент с ReAct-циклом
type Agent struct {
	fileSystem      sandbox.FileSystem
	commandExecutor sandbox.CommandExecutor
	aiClient        AIClient
	tools           map[string]Tool
	toolSchemas     []ToolSchema
	maxSteps        int
	loopDetector    *LoopDetector
}

// NewAgent создаёт новый агент
func NewAgent(
	fs sandbox.FileSystem,
	ce sandbox.CommandExecutor,
	client AIClient,
	maxSteps int,
) *Agent {
	agent := &Agent{
		fileSystem:      fs,
		commandExecutor: ce,
		aiClient:        client,
		tools:           make(map[string]Tool),
		maxSteps:        maxSteps,
		loopDetector:    NewLoopDetector(3),
	}
	agent.registerTools()
	return agent
}

// registerTools регистрирует доступные инструменты
func (a *Agent) registerTools() {
	pathParam := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the file, relative to the workspace",
			},
		},
		"required": []string{"path"},
	}

	a.tools["read_file"] = Tool{
		Name:        "read_file",
		Description: "Read the contents of a file",
		Parameters:  pathParam,
		Execute:     a.toolReadFile,
	}
	a.tools["write_file"] = Tool{
		Name:        "write_file",
		Description: "Write content to a file",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":    map[string]any{"type": "string", "description": "Path to the file, relative to the workspace"},
				"content": map[string]any{"type": "string", "description": "Content to write to the file"},
			},
			"required": []string{"path", "content"},
		},
		Execute: a.toolWriteFile,
	}
	a.tools["list_directory"] = Tool{
		Name:        "list_directory",
		Description: "List files in a directory",
		Parameters:  pathParam,
		Execute:     a.toolListDirectory,
	}
	a.tools["execute_command"] = Tool{
		Name:        "execute_command",
		Description: "Execute a shell command in the workspace (supports pipes, &&, redirects)",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{"type": "string", "description": "Command to execute"},
				"args":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Command arguments"},
			},
			"required": []string{"command"},
		},
		Execute: a.toolExecuteCommand,
	}

	for _, name := range []string{"read_file", "write_file", "list_directory", "execute_command"} {
		t := a.tools[name]
		a.toolSchemas = append(a.toolSchemas, ToolSchema{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Parameters,
		})
	}
}

// Run запускает ReAct-цикл агента
func (a *Agent) Run(ctx context.Context, sessionID string, userMessage string, eventChan chan<- models.SSEEvent) error {
	log.Printf("[Agent] Starting agent for session %s", sessionID)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	manager := NewContextManager(a.aiClient)
	manager.Initialize(sessionID, userMessage)

	for step := 0; step < a.maxSteps; step++ {
		log.Printf("[Agent] Step %d/%d", step+1, a.maxSteps)
		select {
		case <-ctx.Done():
			log.Printf("[Agent] Context cancelled")
			return ctx.Err()
		default:
		}

		messages := manager.GetMessages()
		log.Printf("[Agent] Sending %d messages to AI", len(messages))

		response, err := a.aiClient.StreamCompletion(ctx, messages, a.toolSchemas, eventChan)
		if err != nil {
			log.Printf("[Agent] AI completion error: %v", err)
			eventChan <- models.SSEEvent{
				Type: models.EventTypeError,
				Data: models.ErrorData{Message: err.Error()},
			}
			return err
		}

		log.Printf("[Agent] Response finish_reason: %s, tool_calls: %d", response.FinishReason, len(response.ToolCalls))

		if response.Content != "" || len(response.ToolCalls) > 0 {
			manager.AddAssistantMessage(response.Content, response.ToolCalls)
		}

		if len(response.ToolCalls) > 0 {
			for _, toolCall := range response.ToolCalls {
				log.Printf("[Agent] Tool call: %s with args %v", toolCall.Name, toolCall.Arguments)

				if a.loopDetector.Record(toolCall.Name, toolCall.Arguments) {
					log.Printf("[Agent] Loop detected on tool %s", toolCall.Name)
					eventChan <- models.SSEEvent{
						Type: models.EventTypeError,
						Data: models.ErrorData{
							Message: fmt.Sprintf("loop detected: repeated tool '%s' with identical arguments", toolCall.Name),
						},
					}
					eventChan <- models.SSEEvent{
						Type: models.EventTypeDone,
						Data: models.DoneData{FinishReason: string(models.FinishReasonLoopDetected)},
					}
					return nil
				}

				tool, ok := a.tools[toolCall.Name]
				if !ok {
					log.Printf("[Agent] Unknown tool: %s", toolCall.Name)
					eventChan <- models.SSEEvent{
						Type: models.EventTypeError,
						Data: models.ErrorData{Message: fmt.Sprintf("unknown tool: %s", toolCall.Name)},
					}
					manager.AddToolResult(toolCall.ID, toolCall.Name, "", fmt.Errorf("unknown tool: %s", toolCall.Name))
					continue
				}

				output, err := tool.Execute(ctx, toolCall.Arguments)
				status := "success"
				if err != nil {
					status = "error"
					output = err.Error()
					log.Printf("[Agent] Tool execution error: %v", err)
				} else {
					log.Printf("[Agent] Tool executed successfully, output length: %d", len(output))
				}

				eventChan <- models.SSEEvent{
					Type: models.EventTypeToolResult,
					Data: models.ToolResultData{
						Status: status,
						Output: output,
					},
				}

				manager.AddToolResult(toolCall.ID, toolCall.Name, output, err)
			}
			continue
		}

		if response.FinishReason == "stop" || response.FinishReason == "" {
			eventChan <- models.SSEEvent{
				Type: models.EventTypeDone,
				Data: models.DoneData{FinishReason: string(models.FinishReasonStop)},
			}
			log.Printf("[Agent] Agent finished normally")
			return nil
		}
	}

	eventChan <- models.SSEEvent{
		Type: models.EventTypeDone,
		Data: models.DoneData{FinishReason: string(models.FinishReasonMaxSteps)},
	}
	log.Printf("[Agent] Max steps reached")
	return nil
}

// toolReadFile реализация инструмента чтения файла
func (a *Agent) toolReadFile(ctx context.Context, args map[string]any) (string, error) {
	path, ok := args["path"].(string)
	if !ok {
		return "", fmt.Errorf("missing or invalid path argument")
	}
	data, err := a.fileSystem.Read(ctx, path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// toolWriteFile реализация инструмента записи файла
func (a *Agent) toolWriteFile(ctx context.Context, args map[string]any) (string, error) {
	path, ok := args["path"].(string)
	if !ok {
		return "", fmt.Errorf("missing or invalid path argument")
	}
	content, ok := args["content"].(string)
	if !ok {
		return "", fmt.Errorf("missing or invalid content argument")
	}
	err := a.fileSystem.Write(ctx, path, []byte(content))
	if err != nil {
		return "", err
	}
	return "file written successfully", nil
}

// toolListDirectory реализация инструмента списка директории
func (a *Agent) toolListDirectory(ctx context.Context, args map[string]any) (string, error) {
	path, ok := args["path"].(string)
	if !ok {
		return "", fmt.Errorf("missing or invalid path argument")
	}
	files, err := a.fileSystem.List(ctx, path)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%v", files), nil
}

// toolExecuteCommand реализация инструмента выполнения команды
func (a *Agent) toolExecuteCommand(ctx context.Context, args map[string]any) (string, error) {
	command, ok := args["command"].(string)
	if !ok {
		return "", fmt.Errorf("missing or invalid command argument")
	}
	cmdArgs := toStringSlice(args["args"])
	return a.commandExecutor.Execute(ctx, command, cmdArgs, 30000000000)
}

// toStringSlice приводит значение аргумента к []string (JSON даёт []any)
func toStringSlice(v any) []string {
	result := []string{}
	switch items := v.(type) {
	case []string:
		result = append(result, items...)
	case []any:
		for _, item := range items {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
	}
	return result
}
