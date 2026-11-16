package providers

import (
	"context"
	"fmt"
	"os"
	"strings"

	"multis-go/models"
)

// MultiProviderClient маршрутизирует запросы к конкретным AIClient на основе выбранной модели
type MultiProviderClient struct {
	defaultProvider string
	defaultModel    string
	clients         map[string]AIClient
	modelToClient   map[string]AIClient
}

// DefaultModel возвращает модель по умолчанию
func (mp *MultiProviderClient) DefaultModel() string {
	return mp.defaultModel
}

// NewMultiProviderClient инициализирует клиентов на основе .providers конфигурации и параметров флагов
func NewMultiProviderClient(cfg *ProvidersConfig, defaultProvider, defaultModel, defaultAPIKey, defaultOllamaURL string, defaultNumCtx int, defaultORouterURL string) *MultiProviderClient {
	if defaultOllamaURL == "" {
		defaultOllamaURL = "http://localhost:11434"
	}
	if defaultNumCtx <= 0 {
		defaultNumCtx = 32768
	}
	if defaultORouterURL == "" {
		defaultORouterURL = "https://openrouter.ai/api/v1"
	}

	mp := &MultiProviderClient{
		defaultProvider: defaultProvider,
		defaultModel:    defaultModel,
		clients:         make(map[string]AIClient),
		modelToClient:   make(map[string]AIClient),
	}

	// 1. Инициализация провайдеров из .providers, если конфигурация доступна
	if cfg != nil && cfg.Providers != nil {
		for name, pCfg := range cfg.Providers {
			nameLower := strings.ToLower(name)
			apiKey := ExpandEnvString(pCfg.APIKey)
			baseURL := pCfg.BaseURL
			numCtx := pCfg.NumCtx
			if numCtx <= 0 {
				numCtx = defaultNumCtx
			}

			var client AIClient
			switch nameLower {
			case "deepseek":
				if apiKey == "" {
					apiKey = os.Getenv("DEEPSEEK_API_KEY")
				}
				if apiKey == "" && defaultProvider == "deepseek" {
					apiKey = defaultAPIKey
				}
				ds := NewDeepSeekClient(apiKey, defaultModel)
				if baseURL != "" {
					ds.baseURL = baseURL
				}
				ds.SetFallbackModels(pCfg.Models)
				client = ds
			case "ollama":
				url := baseURL
				if url == "" {
					url = defaultOllamaURL
				}
				ol := NewOllamaClient(url, defaultModel, numCtx)
				ol.SetConfiguredModels(pCfg.Models)
				client = ol
			case "openrouter":
				if apiKey == "" {
					apiKey = os.Getenv("OPENROUTER_API_KEY")
				}
				if apiKey == "" && defaultProvider == "openrouter" {
					apiKey = defaultAPIKey
				}
				url := baseURL
				if url == "" {
					url = defaultORouterURL
				}
				orClient := NewOpenRouterClient(apiKey, defaultModel, url)
				orClient.SetFallbackModels(pCfg.Models)
				client = orClient
			}

			if client != nil {
				mp.clients[nameLower] = client
				for _, m := range pCfg.Models {
					mp.modelToClient[m] = client
					if mp.defaultModel == "" {
						mp.defaultModel = m
					}
				}
			}
		}
	}

	// Если defaultProvider не задан или не найден, выбираем первого доступного
	if mp.defaultProvider == "" || mp.clients[mp.defaultProvider] == nil {
		for _, name := range []string{"openrouter", "ollama", "deepseek"} {
			if _, ok := mp.clients[name]; ok {
				mp.defaultProvider = name
				break
			}
		}
	}

	// 2. Гарантируем, что провайдер по умолчанию из командной строки инициализирован, если был явно указан
	if mp.defaultProvider != "" {
		if _, ok := mp.clients[mp.defaultProvider]; !ok {
			switch mp.defaultProvider {
			case "deepseek":
				key := defaultAPIKey
				if key == "" {
					key = os.Getenv("DEEPSEEK_API_KEY")
				}
				mp.clients["deepseek"] = NewDeepSeekClient(key, defaultModel)
			case "ollama":
				mp.clients["ollama"] = NewOllamaClient(defaultOllamaURL, defaultModel, defaultNumCtx)
			case "openrouter":
				key := defaultAPIKey
				if key == "" {
					key = os.Getenv("OPENROUTER_API_KEY")
				}
				mp.clients["openrouter"] = NewOpenRouterClient(key, defaultModel, defaultORouterURL)
			}
		}
	}

	if mp.defaultModel == "" {
		mp.defaultModel = "deepseek-chat"
	}

	return mp
}

// clientForModel возвращает подходящий AIClient для указанной модели
func (mp *MultiProviderClient) clientForModel(model string) AIClient {
	if model != "" {
		if c, ok := mp.modelToClient[model]; ok {
			return c
		}
	}
	if def, ok := mp.clients[mp.defaultProvider]; ok {
		return def
	}
	for _, c := range mp.clients {
		return c
	}
	return nil
}

// StreamCompletion перенаправляет запрос клиенту выбранной модели
func (mp *MultiProviderClient) StreamCompletion(ctx context.Context, messages []Message, tools map[string]Tool, eventChan chan<- models.SSEEvent) (*StreamResponse, error) {
	target := mp.clientForModel(mp.defaultModel)
	if target == nil {
		return nil, fmt.Errorf("no AI provider available")
	}
	return target.StreamCompletion(ctx, messages, tools, eventChan)
}

// WithModel возвращает копию роутера с установленной конкретной моделью
func (mp *MultiProviderClient) WithModel(model string) AIClient {
	targetClient := mp.clientForModel(model)
	if configurable, ok := targetClient.(ModelConfigurable); ok {
		return configurable.WithModel(model)
	}
	clone := *mp
	clone.defaultModel = model
	return &clone
}

// ListModels собирает список всех моделей из всех зарегистрированных провайдеров
func (mp *MultiProviderClient) ListModels(ctx context.Context) ([]string, error) {
	seen := make(map[string]bool)
	var allModels []string

	// Сначала собираем модели от всех провайдеров через ListModels
	providerOrder := []string{"openrouter", "ollama", "deepseek"}
	for _, p := range providerOrder {
		c, ok := mp.clients[p]
		if !ok {
			continue
		}
		if lister, ok := c.(ModelLister); ok {
			if list, err := lister.ListModels(ctx); err == nil {
				for _, m := range list {
					if !seen[m] {
						seen[m] = true
						allModels = append(allModels, m)
						mp.modelToClient[m] = c
					}
				}
			}
		}
	}

	// Также проверяем оставшиеся клиенты, если есть
	for p, c := range mp.clients {
		found := false
		for _, po := range providerOrder {
			if po == p {
				found = true
				break
			}
		}
		if found {
			continue
		}
		if lister, ok := c.(ModelLister); ok {
			if list, err := lister.ListModels(ctx); err == nil {
				for _, m := range list {
					if !seen[m] {
						seen[m] = true
						allModels = append(allModels, m)
						mp.modelToClient[m] = c
					}
				}
			}
		}
	}

	return allModels, nil
}

// UnloadModel перенаправляет выгрузку модели соответствующему провайдеру (например, Ollama)
func (mp *MultiProviderClient) UnloadModel(ctx context.Context, modelName string) error {
	client := mp.clientForModel(modelName)
	if unloader, ok := client.(ModelUnloader); ok {
		return unloader.UnloadModel(ctx, modelName)
	}
	// Если клиент не unloader, ищем среди всех клиентов тот, который поддерживает ModelUnloader (Ollama)
	for _, c := range mp.clients {
		if unloader, ok := c.(ModelUnloader); ok {
			return unloader.UnloadModel(ctx, modelName)
		}
	}
	return fmt.Errorf("current provider does not support model unloading")
}

