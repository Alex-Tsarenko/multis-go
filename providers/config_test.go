package providers

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadProvidersConfig(t *testing.T) {
	yamlContent := `providers:
  deepseek:
    api_key: "${DEEPSEEK_API_KEY}"
    base_url: "https://api.deepseek.com/v1"
    models:
      - "deepseek-chat"
      - "deepseek-reasoner"
  ollama:
    base_url: "http://localhost:11434"
    num_ctx: 32768
    models:
      - "qwen3.5:4b"
`
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, ".providers")
	if err := os.WriteFile(filePath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	cfg, err := LoadProvidersConfig(filePath)
	if err != nil {
		t.Fatalf("LoadProvidersConfig failed: %v", err)
	}

	if len(cfg.Providers) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(cfg.Providers))
	}

	dsModels := cfg.GetConfiguredModels("deepseek")
	if len(dsModels) != 2 || dsModels[0] != "deepseek-chat" {
		t.Errorf("unexpected deepseek models: %v", dsModels)
	}

	ollamaModels := cfg.GetConfiguredModels("ollama")
	if len(ollamaModels) != 1 || ollamaModels[0] != "qwen3.5:4b" {
		t.Errorf("unexpected ollama models: %v", ollamaModels)
	}

	if nonExistent := cfg.GetConfiguredModels("unknown"); nonExistent != nil {
		t.Errorf("expected nil for unknown provider, got %v", nonExistent)
	}
}

