package providers

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ProviderConfig конфигурация отдельного провайдера
type ProviderConfig struct {
	APIKey   string   `yaml:"api_key"`
	BaseURL  string   `yaml:"base_url"`
	NumCtx   int      `yaml:"num_ctx"`
	Models   []string `yaml:"models"`
}

// ProvidersConfig структура файла .providers
type ProvidersConfig struct {
	Providers map[string]ProviderConfig `yaml:"providers"`
}

// LoadProvidersConfig загружает конфигурацию провайдеров из указанного пути.
// Если путь пустой, ищет .providers в текущей директории.
func LoadProvidersConfig(path string) (*ProvidersConfig, error) {
	if path == "" {
		path = ".providers"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg ProvidersConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// FindProvidersConfig ищет .providers в workspace или в текущей рабочей директории.
// Возвращает nil, если файл не найден.
func FindProvidersConfig(workspace string) *ProvidersConfig {
	candidates := []string{}
	if workspace != "" {
		candidates = append(candidates, filepath.Join(workspace, ".providers"))
	}
	candidates = append(candidates, ".providers")

	for _, p := range candidates {
		if cfg, err := LoadProvidersConfig(p); err == nil {
			return cfg
		}
	}
	return nil
}

// ExpandEnvString подставляет переменные окружения формата ${VAR} или $VAR
func ExpandEnvString(s string) string {
	return os.ExpandEnv(s)
}

// GetConfiguredModels возвращает список моделей для указанного провайдера
func (cfg *ProvidersConfig) GetConfiguredModels(providerName string) []string {
	if cfg == nil || cfg.Providers == nil {
		return nil
	}
	p, ok := cfg.Providers[strings.ToLower(providerName)]
	if !ok {
		return nil
	}
	return p.Models
}

