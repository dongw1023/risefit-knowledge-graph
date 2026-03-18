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
	model := ""
	if cfg.EmbeddingProvider == string(vectorstore.ProviderOpenAI) {
		apiKey = cfg.OpenAIAPIKey
		model = cfg.OpenAIEmbeddingModel
	}

	if apiKey == "" || cfg.QdrantURL == "" || cfg.QdrantCollectionName == "" {
		log.Fatalf("Required environment variables (API_KEY, QDRANT_URL, QDRANT_COLLECTION_NAME) are not set for provider %s", cfg.EmbeddingProvider)
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
