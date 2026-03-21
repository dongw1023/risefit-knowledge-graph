package vectorstore

import (
	"context"
	"fmt"
	"log"
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

type Filter struct {
	Category       string
	Intent         string
	TargetAudience string
	EvidenceLevel  string
}

type Store struct {
	client                 *qdrant.Client
	embedder               Embedder
	collectionName         string
	registryCollectionName string
}

func NewQdrantStore(ctx context.Context, embedConfig EmbedderConfig, qdrantURLStr, qdrantAPIKey, collectionName, registryCollection string) (*Store, error) {
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
		client:                 client,
		embedder:               embedder,
		collectionName:         collectionName,
		registryCollectionName: registryCollection,
	}, nil
}

func (s *Store) GetDocumentRecord(ctx context.Context, id string) (*schema.DocumentRecord, error) {
	if s.registryCollectionName == "" {
		return nil, nil // No registry configured
	}

	log.Printf("Getting document record for ID: %s", id)
	points, err := s.client.Get(ctx, &qdrant.GetPoints{
		CollectionName: s.registryCollectionName,
		Ids:            []*qdrant.PointId{qdrant.NewIDUUID(id)},
		WithPayload:    qdrant.NewWithPayload(true),
	})
	if err != nil {
		log.Printf("Failed to get document record %s: %v", id, err)
		return nil, err
	}

	if len(points) == 0 {
		log.Printf("Document record %s not found", id)
		return nil, nil // Not found
	}

	p := points[0].Payload
	record := &schema.DocumentRecord{
		ID:             id,
		DocumentTitle:  p["document_title"].GetStringValue(),
		Path:           p["path"].GetStringValue(),
		Category:       p["category"].GetStringValue(),
		Intent:         p["intent"].GetStringValue(),
		TargetAudience: p["target_audience"].GetStringValue(),
		EvidenceLevel:  p["evidence_level"].GetStringValue(),
		Status:         p["status"].GetStringValue(),
		PageCount:      int(p["page_count"].GetIntegerValue()),
		CreatedAt:      p["created_at"].GetStringValue(),
		UpdatedAt:      p["updated_at"].GetStringValue(),
	}

	log.Printf("Found document record: %s (status: %s)", record.DocumentTitle, record.Status)
	return record, nil
}

func (s *Store) UpsertDocumentRecord(ctx context.Context, record schema.DocumentRecord) error {
	if s.registryCollectionName == "" {
		return nil // No registry configured
	}

	log.Printf("Upserting document record: %s (status: %s)", record.DocumentTitle, record.Status)
	payload := map[string]any{
		"document_title":  record.DocumentTitle,
		"path":            record.Path,
		"category":        record.Category,
		"intent":          record.Intent,
		"target_audience": record.TargetAudience,
		"evidence_level":  record.EvidenceLevel,
		"status":          record.Status,
		"page_count":      record.PageCount,
		"created_at":      record.CreatedAt,
		"updated_at":      record.UpdatedAt,
	}

	for k, v := range record.Metadata {
		payload[k] = v
	}

	_, err := s.client.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: s.registryCollectionName,
		Points: []*qdrant.PointStruct{
			{
				Id:      qdrant.NewIDUUID(record.ID),
				Payload: qdrant.NewValueMap(payload),
				Vectors: qdrant.NewVectorsMap(map[string]*qdrant.Vector{
					"none": qdrant.NewVector(0), // Dummy single-dim vector
				}),
			},
		},
	})

	if err != nil {
		log.Printf("Failed to upsert document record %s: %v", record.ID, err)
	}

	return err
}

func (s *Store) AddDocuments(ctx context.Context, docs []schema.Document) error {
	if len(docs) == 0 {
		return nil
	}
	log.Printf("Adding %d documents to collection %s", len(docs), s.collectionName)
	texts := make([]string, len(docs))
	for i, doc := range docs {
		texts[i] = doc.PageContent
	}

	log.Printf("Embedding %d documents...", len(texts))
	embeddings, err := s.embedder.EmbedDocuments(ctx, texts)
	if err != nil {
		log.Printf("Failed to embed documents: %v", err)
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

	log.Printf("Upserting %d points to Qdrant...", len(points))
	_, err = s.client.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: s.collectionName,
		Points:         points,
	})
	if err != nil {
		log.Printf("Failed to upsert points: %v", err)
		return fmt.Errorf("failed to upsert points to qdrant: %w", err)
	}

	log.Printf("Successfully added %d documents", len(docs))
	return nil
}

func (s *Store) SimilaritySearch(ctx context.Context, query string, numResults int, filter Filter) ([]schema.Document, error) {
	log.Printf("Similarity search: query=%q, limit=%d, filter=%+v", query, numResults, filter)
	embedding, err := s.embedder.EmbedQuery(ctx, query)
	if err != nil {
		log.Printf("Failed to embed query: %v", err)
		return nil, fmt.Errorf("failed to embed query: %w", err)
	}

	var qdrantFilter *qdrant.Filter
	var conditions []*qdrant.Condition

	if filter.Category != "" {
		conditions = append(conditions, qdrant.NewMatchKeyword("category", filter.Category))
	}
	if filter.Intent != "" {
		conditions = append(conditions, qdrant.NewMatchKeyword("intent", filter.Intent))
	}
	if filter.TargetAudience != "" {
		conditions = append(conditions, qdrant.NewMatchKeyword("target_audience", filter.TargetAudience))
	}
	if filter.EvidenceLevel != "" {
		conditions = append(conditions, qdrant.NewMatchKeyword("evidence_level", filter.EvidenceLevel))
	}

	if len(conditions) > 0 {
		qdrantFilter = &qdrant.Filter{
			Must: conditions,
		}
	}

	log.Printf("Querying Qdrant collection %s...", s.collectionName)
	res, err := s.client.Query(ctx, &qdrant.QueryPoints{
		CollectionName: s.collectionName,
		Query:          qdrant.NewQuery(embedding...),
		Using:          ptr("dense-vector"),
		Filter:         qdrantFilter,
		Limit:          ptr(uint64(numResults)),
		WithPayload:    qdrant.NewWithPayload(true),
	})
	if err != nil {
		log.Printf("Failed to query Qdrant: %v", err)
		return nil, fmt.Errorf("failed to query qdrant: %w", err)
	}

	log.Printf("Qdrant returned %d results", len(res))
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
