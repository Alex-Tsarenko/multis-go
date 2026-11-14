package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"multis-go/app"
	"multis-go/providers"
	"multis-go/sandbox"
	"net/http"
	"os"
)

//go:embed ui
var uiFS embed.FS

var (
	provider  = flag.String("provider", "deepseek", "AI provider: deepseek or ollama")
	apiKey    = flag.String("api-key", "", "DeepSeek API key")
	model     = flag.String("model", "deepseek-chat", "Model to use")
	ollamaURL = flag.String("ollama-url", "http://localhost:11434", "Ollama server URL")
	numCtx    = flag.Int("num-ctx", 32768, "Ollama context window size (num_ctx)")
	port      = flag.String("port", "8080", "Port to listen on")
	workspace = flag.String("workspace", ".", "Workspace directory")
)

func main() {
	flag.Parse()

	if *apiKey == "" {
		*apiKey = os.Getenv("DEEPSEEK_API_KEY")
	}

	fileSystem, err := sandbox.NewSafeFileSystem(*workspace)
	if err != nil {
		log.Fatalf("Failed to initialize filesystem: %v", err)
	}

	ce := sandbox.NewSafeCommandExecutor(*workspace)

	var client providers.AIClient
	switch *provider {
	case "deepseek":
		if *apiKey == "" {
			log.Fatal("API key is required for DeepSeek. Provide it via -api-key flag or DEEPSEEK_API_KEY environment variable")
		}
		client = providers.NewDeepSeekClient(*apiKey, *model)
	case "ollama":
		client = providers.NewOllamaClient(*ollamaURL, *model, *numCtx)
	default:
		log.Fatalf("Unknown provider: %s. Use 'deepseek' or 'ollama'", *provider)
	}

	handler := app.NewHandler(fileSystem, ce, client)
	handler.SetDefaultModel(*model)

	server := app.NewServer(":" + *port)
	server.RegisterRoutes(handler)

	uiSub, _ := fs.Sub(uiFS, "ui")
	server.RegisterStatic("/", http.FileServer(http.FS(uiSub)))

	fmt.Printf("Starting Multis.Agent on http://localhost:%s\n", *port)
	fmt.Printf("Workspace: %s\n", *workspace)
	fmt.Printf("Model: %s\n", *model)

	if err := server.Start(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
