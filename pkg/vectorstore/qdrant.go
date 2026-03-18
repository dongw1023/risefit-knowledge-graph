package vectorstore

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	"github.com/google/uuid"
	"github.com/qdrant/go-client/qdrant"
	"github.com/risefit/knowledge-graph/pkg/schema"
)

type Provider string

const (
	ProviderOpenAI Provider = "openai"
	ProviderGoogle Provider = "googleai"
)

type EmbedderConfig struct {
	Provider Provider
	APIKey   string
	Model    string
}

type Store struct {
	client         *qdrant.Client
	embedder       Embedder
	collectionName string
}

func NewQdrantStore(ctx context.Context, embedConfig EmbedderConfig, qdrantURLStr, qdrantAPIKey, collectionName string) (*Store, error) {
	var embedder Embedder
	var err error

	switch embedConfig.Provider {
	case ProviderOpenAI:
		embedder = NewOpenAIEmbedder(embedConfig.APIKey, embedConfig.Model)
	case ProviderGoogle:
		embedder, err = NewGoogleEmbedder(ctx, embedConfig.APIKey, embedConfig.Model)
		if err != nil {
			return nil, fmt.Errorf("failed to create googleai embedder: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported embedding provider: %s", embedConfig.Provider)
	}

	u, err := url.Parse(qdrantURLStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse qdrant url: %w", err)
	}

	host := u.Hostname()
	portStr := u.Port()
	if portStr == "" {
		portStr = "6334" // Default gRPC port
	}
	port, _ := strconv.Atoi(portStr)

	client, err := qdrant.NewClient(&qdrant.Config{
		Host:   host,
		Port:   port,
		APIKey: qdrantAPIKey,
		UseTLS: u.Scheme == "https",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create qdrant client: %w", err)
	}

	return &Store{
		client:         client,
		embedder:       embedder,
		collectionName: collectionName,
	}, nil
}

func (s *Store) AddDocuments(ctx context.Context, docs []schema.Document) error {
	if len(docs) == 0 {
		return nil
	}
	texts := make([]string, len(docs))
	for i, doc := range docs {
		texts[i] = doc.PageContent
	}

	embeddings, err := s.embedder.EmbedDocuments(ctx, texts)
	if err != nil {
		return fmt.Errorf("failed to embed documents: %w", err)
	}

	points := make([]*qdrant.PointStruct, len(docs))
	for i, doc := range docs {
		payload := make(map[string]any)
		for k, v := range doc.Metadata {
			payload[k] = v
		}
		payload["page_content"] = doc.PageContent

		points[i] = &qdrant.PointStruct{
			Id: qdrant.NewIDUUID(uuid.New().String()),
			Vectors: qdrant.NewVectorsMap(map[string]*qdrant.Vector{
				"dense-vector": qdrant.NewVector(embeddings[i]...),
			}),
			Payload: qdrant.NewValueMap(payload),
		}
	}

	_, err = s.client.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: s.collectionName,
		Points:         points,
	})
	if err != nil {
		return fmt.Errorf("failed to upsert points to qdrant: %w", err)
	}

	return nil
}

func (s *Store) SimilaritySearch(ctx context.Context, query string, numResults int) ([]schema.Document, error) {
	embedding, err := s.embedder.EmbedQuery(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to embed query: %w", err)
	}

	res, err := s.client.Query(ctx, &qdrant.QueryPoints{
		CollectionName: s.collectionName,
		Query:          qdrant.NewQuery(embedding...),
		Using:          ptr("dense-vector"),
		Limit:          ptr(uint64(numResults)),
		WithPayload:    qdrant.NewWithPayload(true),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query qdrant: %w", err)
	}

	docs := make([]schema.Document, len(res))
	for i, point := range res {
		payload := point.Payload
		content := ""
		if v, ok := payload["page_content"]; ok {
			content = v.GetStringValue()
		}

		metadata := make(map[string]any)
		for k, v := range payload {
			if k == "page_content" {
				continue
			}
			metadata[k] = asInterface(v)
		}

		docs[i] = schema.Document{
			PageContent: content,
			Metadata:    metadata,
			Score:       point.Score,
		}
	}

	return docs, nil
}

func asInterface(v *qdrant.Value) any {
	if v == nil {
		return nil
	}
	switch k := v.Kind.(type) {
	case *qdrant.Value_NullValue:
		return nil
	case *qdrant.Value_DoubleValue:
		return k.DoubleValue
	case *qdrant.Value_IntegerValue:
		return k.IntegerValue
	case *qdrant.Value_StringValue:
		return k.StringValue
	case *qdrant.Value_BoolValue:
		return k.BoolValue
	case *qdrant.Value_StructValue:
		res := make(map[string]any)
		for key, val := range k.StructValue.Fields {
			res[key] = asInterface(val)
		}
		return res
	case *qdrant.Value_ListValue:
		res := make([]any, len(k.ListValue.Values))
		for i, val := range k.ListValue.Values {
			res[i] = asInterface(val)
		}
		return res
	default:
		return nil
	}
}

func ptr[T any](v T) *T {
	return &v
}
