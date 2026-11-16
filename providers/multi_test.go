package providers

import (
	"context"
	"testing"
)

func TestMultiProviderClient_ListModels(t *testing.T) {
	cfg := &ProvidersConfig{
		Providers: map[string]ProviderConfig{
			"deepseek": {
				APIKey: "test",
				Models: []string{"deepseek-chat", "deepseek-reasoner"},
			},
			"ollama": {
				BaseURL: "http://localhost:11434",
				Models:  []string{"qwen3.5:4b", "mistral-nemo:12b"},
			},
			"openrouter": {
				APIKey: "test",
				Models: []string{"openai/gpt-4o-mini"},
			},
		},
	}

	mp := NewMultiProviderClient(cfg, "openrouter", "openai/gpt-4o-mini", "test", "http://localhost:11434", 32768, "https://openrouter.ai/api/v1")
	models, err := mp.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels failed: %v", err)
	}

	expected := []string{"openai/gpt-4o-mini", "qwen3.5:4b", "mistral-nemo:12b", "deepseek-chat", "deepseek-reasoner"}
	if len(models) != len(expected) {
		t.Fatalf("expected %d models, got %d: %v", len(expected), len(models), models)
	}

	c := mp.clientForModel("qwen3.5:4b")
	if _, ok := c.(*OllamaClient); !ok {
		t.Errorf("expected *OllamaClient for qwen3.5:4b, got %T", c)
	}

	cDs := mp.clientForModel("deepseek-chat")
	if _, ok := cDs.(*DeepSeekClient); !ok {
		t.Errorf("expected *DeepSeekClient for deepseek-chat, got %T", cDs)
	}

	cOr := mp.clientForModel("openai/gpt-4o-mini")
	if _, ok := cOr.(*OpenRouterClient); !ok {
		t.Errorf("expected *OpenRouterClient for openai/gpt-4o-mini, got %T", cOr)
	}
}

