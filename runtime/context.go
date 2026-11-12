package runtime

const (
	maxOutputLength = 10000
)

// ContextManager управляет контекстом и историей сообщений
type ContextManager struct {
	client   AIClient
	messages []Message
}

// NewContextManager создаёт новый ContextManager
func NewContextManager(client AIClient) *ContextManager {
	return &ContextManager{
		client:   client,
		messages: make([]Message, 0),
	}
}

// Initialize инициализирует контекст с пользовательским сообщением
func (cm *ContextManager) Initialize(sessionID, userMessage string) []Message {
	cm.messages = []Message{
		{Role: "system", Content: `You are a helpful AI assistant with access to file system and command execution tools.

Rules:
- Use tools only when they are needed to complete the user's request.
- Never create or modify files unless the user explicitly asks for it.
- Do not explore the workspace unless the task requires it.
- If the request can be answered directly, answer without using tools.`},
		{Role: "user", Content: userMessage},
	}
	return cm.messages
}

// AddToolResult добавляет результат выполнения инструмента в историю
func (cm *ContextManager) AddToolResult(toolID, toolName, output string, err error) {
	content := output
	if err != nil {
		content = "Error: " + err.Error()
	}

	if len(content) > maxOutputLength {
		content = content[:maxOutputLength] + "... (truncated)"
	}

	cm.messages = append(cm.messages, Message{
		Role:       "tool",
		Content:    content,
		ToolCallID: toolID,
		ToolName:   toolName,
	})
}

// AddAssistantMessage добавляет ответ ассистента в историю
func (cm *ContextManager) AddAssistantMessage(content string, toolCalls []ToolCall) {
	cm.messages = append(cm.messages, Message{
		Role:      "assistant",
		Content:   content,
		ToolCalls: toolCalls,
	})
}

// GetMessages возвращает текущую историю сообщений
func (cm *ContextManager) GetMessages() []Message {
	return cm.messages
}
