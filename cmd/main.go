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
)

//go:embed ui
var uiFS embed.FS

var (
	port      = flag.String("port", "8080", "Port to listen on")
	workspace = flag.String("workspace", ".", "Workspace directory")
	config    = flag.String("config", "", "Path to .providers config file")
)

func main() {
	flag.Parse()

	fileSystem, err := sandbox.NewSafeFileSystem(*workspace)
	if err != nil {
		log.Fatalf("Failed to initialize filesystem: %v", err)
	}

	ce := sandbox.NewSafeCommandExecutor(*workspace)

	var providersCfg *providers.ProvidersConfig
	if *config != "" {
		cfg, err := providers.LoadProvidersConfig(*config)
		if err != nil {
			log.Fatalf("Failed to load providers config %s: %v", *config, err)
		}
		providersCfg = cfg
	} else {
		providersCfg = providers.FindProvidersConfig(*workspace)
	}

	client := providers.NewMultiProviderClient(providersCfg, "", "", "", "", 0, "")

	handler := app.NewHandler(fileSystem, ce, client)
	handler.SetDefaultModel(client.DefaultModel())

	server := app.NewServer(":" + *port)
	server.RegisterRoutes(handler)

	uiSub, _ := fs.Sub(uiFS, "ui")
	server.RegisterStatic("/", http.FileServer(http.FS(uiSub)))

	fmt.Printf("Starting Multis.Agent on http://localhost:%s\n", *port)
	fmt.Printf("Workspace: %s\n", *workspace)
	fmt.Printf("Default Model: %s\n", client.DefaultModel())

	if err := server.Start(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
