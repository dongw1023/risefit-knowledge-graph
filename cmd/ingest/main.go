package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/risefit/knowledge-graph/pkg/pdf"
	"github.com/risefit/knowledge-graph/pkg/vectorstore"
)

func main() {
	// Load .env file
	err := godotenv.Load()
	if err != nil {
		log.Printf("Warning: .env file not found")
	}

	provider := os.Getenv("EMBEDDING_PROVIDER")
	if provider == "" {
		provider = "googleai" // Default
	}

	apiKey := ""
	model := ""
	if provider == string(vectorstore.ProviderOpenAI) {
		apiKey = os.Getenv("OPENAI_API_KEY")
		model = os.Getenv("OPENAI_EMBEDDING_MODEL")
		if model == "" {
			model = "text-embedding-3-small"
		}
	} else if provider == string(vectorstore.ProviderGoogle) {
		apiKey = os.Getenv("GOOGLE_API_KEY")
	}

	qdrantURL := os.Getenv("QDRANT_URL")
	qdrantAPIKey := os.Getenv("QDRANT_API_KEY")
	collectionName := os.Getenv("QDRANT_COLLECTION_NAME")

	if apiKey == "" || qdrantURL == "" || collectionName == "" {
		log.Fatalf("Required environment variables (API_KEY, QDRANT_URL, QDRANT_COLLECTION_NAME) are not set for provider %s", provider)
	}

	ctx := context.Background()

	// 1. Initialize Vector Store
	embedConfig := vectorstore.EmbedderConfig{
		Provider: vectorstore.Provider(provider),
		APIKey:   apiKey,
		Model:    model,
	}

	store, err := vectorstore.NewQdrantStore(ctx, embedConfig, qdrantURL, qdrantAPIKey, collectionName)
	if err != nil {
		log.Fatalf("Error initializing vector store: %v", err)
	}

	// 2. Process PDF
	pdfPath := "sample.pdf"
	if len(os.Args) > 1 {
		pdfPath = os.Args[1]
	}

	fmt.Printf("Processing PDF: %s using provider: %s\n", pdfPath, provider)

	// Initialize PDF Processor
	processor := pdf.NewProcessor(1000, 100)

	docs, err := processor.LoadAndSplit(ctx, pdfPath)
	if err != nil {
		log.Fatalf("Error processing PDF: %v", err)
	}

	fmt.Printf("Split into %d chunks\n", len(docs))

	// Add to Vector Store
	err = store.AddDocuments(ctx, docs)
	if err != nil {
		log.Fatalf("Error adding documents to store: %v", err)
	}

	fmt.Println("Successfully processed and stored PDF content!")
}
