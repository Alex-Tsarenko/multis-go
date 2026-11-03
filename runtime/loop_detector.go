package runtime

import (
	"encoding/json"
	"fmt"
)

const defaultMaxConsecutiveRepeats = 3

// LoopDetector отслеживает зацикливание вызовов инструментов агентом
type LoopDetector struct {
	maxRepeats      int
	lastActionKey   string
	consecutiveCount int
}

// NewLoopDetector создаёт новый LoopDetector с заданным порогом повторений подряд
func NewLoopDetector(maxRepeats int) *LoopDetector {
	if maxRepeats <= 0 {
		maxRepeats = defaultMaxConsecutiveRepeats
	}
	return &LoopDetector{
		maxRepeats: maxRepeats,
	}
}

// Record регистрирует вызов инструмента. Возвращает true, если зафиксировано зацикливание.
func (ld *LoopDetector) Record(toolName string, args map[string]any) bool {
	actionKey := ld.buildActionKey(toolName, args)

	if actionKey == ld.lastActionKey {
		ld.consecutiveCount++
	} else {
		ld.lastActionKey = actionKey
		ld.consecutiveCount = 1
	}

	return ld.consecutiveCount >= ld.maxRepeats
}

// Reset сбрасывает счетчик детектора
func (ld *LoopDetector) Reset() {
	ld.lastActionKey = ""
	ld.consecutiveCount = 0
}

func (ld *LoopDetector) buildActionKey(toolName string, args map[string]any) string {
	rawArgs, err := json.Marshal(args)
	if err != nil {
		return fmt.Sprintf("%s:%v", toolName, args)
	}
	return fmt.Sprintf("%s:%s", toolName, string(rawArgs))
}

