package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	EmbeddingProvider        string
	GoogleAPIKey             string
	OpenAIAPIKey             string
	OpenAIEmbeddingModel     string
	QdrantURL                string
	QdrantAPIKey             string
	QdrantCollectionName     string
	QdrantRegistryCollection string
	GCSBucketName            string
	PDFParsingModel          string
	Port                     string
}

func Load() (*Config, error) {
	// Load .env if it exists
	_ = godotenv.Load()

	return &Config{
		EmbeddingProvider:        getEnv("EMBEDDING_PROVIDER", "openai"),
		GoogleAPIKey:             os.Getenv("GOOGLE_API_KEY"),
		OpenAIAPIKey:             os.Getenv("OPENAI_API_KEY"),
		OpenAIEmbeddingModel:     getEnv("OPENAI_EMBEDDING_MODEL", "text-embedding-3-small"),
		QdrantURL:                os.Getenv("QDRANT_URL"),
		QdrantAPIKey:             os.Getenv("QDRANT_API_KEY"),
		QdrantCollectionName:     getEnv("QDRANT_COLLECTION_NAME", "risefit_content_v1"),
		QdrantRegistryCollection: getEnv("QDRANT_REGISTRY_COLLECTION", "risefit_registry"),
		GCSBucketName:            getEnv("GCS_BUCKET_NAME", "risefit-knowledge-graph-data"),
		PDFParsingModel:          getEnv("PDF_PARSING_MODEL", "gemini-2.5-flash"),
		Port:                     getEnv("PORT", "8080"),
	}, nil
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
