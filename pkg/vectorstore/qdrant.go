package vectorstore

import (
	"context"
	"fmt"
	"net/url"

	"github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/llms/googleai"
	"github.com/tmc/langchaingo/llms/openai"
	"github.com/tmc/langchaingo/schema"
	"github.com/tmc/langchaingo/vectorstores/qdrant"
)

type Provider string

const (
	ProviderOpenAI Provider = "openai"
	ProviderGoogle Provider = "googleai"
)

type EmbedderConfig struct {
	Provider Provider
	APIKey   string
	Model    string // Optional: e.g., "text-embedding-3-small" for OpenAI
}

type Store struct {
	store *qdrant.Store
}

func NewQdrantStore(ctx context.Context, embedConfig EmbedderConfig, qdrantURLStr, qdrantAPIKey, collectionName string) (*Store, error) {
	var embedder embeddings.Embedder
	var err error

	switch embedConfig.Provider {
	case ProviderOpenAI:
		opts := []openai.Option{openai.WithToken(embedConfig.APIKey)}
		if embedConfig.Model != "" {
			opts = append(opts, openai.WithEmbeddingModel(embedConfig.Model))
		}
		llm, err := openai.New(opts...)
		if err != nil {
			return nil, fmt.Errorf("failed to create openai llm: %w", err)
		}
		embedder, err = embeddings.NewEmbedder(llm)
		if err != nil {
			return nil, fmt.Errorf("failed to create openai embedder: %w", err)
		}

	case ProviderGoogle:
		llm, err := googleai.New(ctx, googleai.WithAPIKey(embedConfig.APIKey))
		if err != nil {
			return nil, fmt.Errorf("failed to create googleai llm: %w", err)
		}
		embedder, err = embeddings.NewEmbedder(llm)
		if err != nil {
			return nil, fmt.Errorf("failed to create googleai embedder: %w", err)
		}

	default:
		return nil, fmt.Errorf("unsupported embedding provider: %s", embedConfig.Provider)
	}

	// Initialize Qdrant Vector Store
	qdrantURL, err := url.Parse(qdrantURLStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse qdrant url: %w", err)
	}

	qs, err := qdrant.New(
		qdrant.WithURL(*qdrantURL),
		qdrant.WithAPIKey(qdrantAPIKey),
		qdrant.WithCollectionName(collectionName),
		qdrant.WithEmbedder(embedder),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create qdrant store: %w", err)
	}

	return &Store{store: &qs}, nil
}

func (s *Store) AddDocuments(ctx context.Context, docs []schema.Document) error {
	_, err := s.store.AddDocuments(ctx, docs)
	if err != nil {
		return fmt.Errorf("failed to add documents to qdrant: %w", err)
	}
	return nil
}

func (s *Store) SimilaritySearch(ctx context.Context, query string, numResults int) ([]schema.Document, error) {
	docs, err := s.store.SimilaritySearch(ctx, query, numResults)
	if err != nil {
		return nil, fmt.Errorf("failed to search documents in qdrant: %w", err)
	}
	return docs, nil
}
