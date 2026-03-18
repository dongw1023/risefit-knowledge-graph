package vectorstore

import (
	"context"
	"fmt"

	"github.com/google/generative-ai-go/genai"
	"github.com/openai/openai-go"
	openaiopt "github.com/openai/openai-go/option"
	googleopt "google.golang.org/api/option"
)

type Embedder interface {
	EmbedQuery(ctx context.Context, text string) ([]float32, error)
	EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error)
}

type OpenAIEmbedder struct {
	client *openai.Client
	model  string
}

func NewOpenAIEmbedder(apiKey, model string) *OpenAIEmbedder {
	if model == "" {
		model = string(openai.EmbeddingModelTextEmbedding3Small)
	}
	client := openai.NewClient(openaiopt.WithAPIKey(apiKey))
	return &OpenAIEmbedder{
		client: &client,
		model:  model,
	}
}

func (e *OpenAIEmbedder) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	res, err := e.client.Embeddings.New(ctx, openai.EmbeddingNewParams{
		Input: openai.EmbeddingNewParamsInputUnion{
			OfString: openai.String(text),
		},
		Model: openai.EmbeddingModel(e.model),
	})
	if err != nil {
		return nil, err
	}
	if len(res.Data) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}

	embeddings := make([]float32, len(res.Data[0].Embedding))
	for i, v := range res.Data[0].Embedding {
		embeddings[i] = float32(v)
	}
	return embeddings, nil
}

func (e *OpenAIEmbedder) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	res, err := e.client.Embeddings.New(ctx, openai.EmbeddingNewParams{
		Input: openai.EmbeddingNewParamsInputUnion{
			OfArrayOfStrings: texts,
		},
		Model: openai.EmbeddingModel(e.model),
	})
	if err != nil {
		return nil, err
	}

	results := make([][]float32, len(res.Data))
	for i, data := range res.Data {
		results[i] = make([]float32, len(data.Embedding))
		for j, v := range data.Embedding {
			results[i][j] = float32(v)
		}
	}
	return results, nil
}

type GoogleEmbedder struct {
	client *genai.Client
	model  string
}

func NewGoogleEmbedder(ctx context.Context, apiKey, model string) (*GoogleEmbedder, error) {
	if model == "" {
		model = "text-embedding-004"
	}
	client, err := genai.NewClient(ctx, googleopt.WithAPIKey(apiKey))
	if err != nil {
		return nil, err
	}
	return &GoogleEmbedder{
		client: client,
		model:  model,
	}, nil
}

func (e *GoogleEmbedder) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	em := e.client.EmbeddingModel(e.model)
	res, err := em.EmbedContent(ctx, genai.Text(text))
	if err != nil {
		return nil, err
	}
	return res.Embedding.Values, nil
}

func (e *GoogleEmbedder) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	em := e.client.EmbeddingModel(e.model)
	batch := em.NewBatch()
	for _, t := range texts {
		batch.AddContent(genai.Text(t))
	}
	res, err := em.BatchEmbedContents(ctx, batch)
	if err != nil {
		return nil, err
	}

	results := make([][]float32, len(res.Embeddings))
	for i, e := range res.Embeddings {
		results[i] = e.Values
	}
	return results, nil
}
