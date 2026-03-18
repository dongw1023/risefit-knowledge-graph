package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/risefit/knowledge-graph/pkg/api"
	"github.com/risefit/knowledge-graph/pkg/config"
	"github.com/risefit/knowledge-graph/pkg/vectorstore"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	apiKey := cfg.GoogleAPIKey
	apiKeyName := "GOOGLE_API_KEY"
	model := ""
	if cfg.EmbeddingProvider == string(vectorstore.ProviderOpenAI) {
		apiKey = cfg.OpenAIAPIKey
		apiKeyName = "OPENAI_API_KEY"
		model = cfg.OpenAIEmbeddingModel
	}

	if apiKey == "" {
		log.Fatalf("Config error: %s is required for provider %s", apiKeyName, cfg.EmbeddingProvider)
	}
	if cfg.QdrantURL == "" {
		log.Fatal("Config error: QDRANT_URL is required")
	}
	if cfg.QdrantCollectionName == "" {
		log.Fatal("Config error: QDRANT_COLLECTION_NAME is required")
	}

	ctx := context.Background()

	// 1. Initialize Vector Store
	embedConfig := vectorstore.EmbedderConfig{
		Provider: vectorstore.Provider(cfg.EmbeddingProvider),
		APIKey:   apiKey,
		Model:    model,
	}

	store, err := vectorstore.NewQdrantStore(ctx, embedConfig, cfg.QdrantURL, cfg.QdrantAPIKey, cfg.QdrantCollectionName, cfg.QdrantRegistryCollection)
	if err != nil {
		log.Fatalf("Error initializing vector store: %v", err)
	}

	// 2. Start API Server
	handler := api.NewHandler(store)
	http.HandleFunc("/search", handler.Search)

	fmt.Printf("Search API server starting on :%s\n", cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, nil))
}
