package main

import (
	"context"
	"crypto/md5"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/google/generative-ai-go/genai"
	"github.com/joho/godotenv"
	"github.com/risefit/knowledge-graph/pkg/pdf"
	"github.com/risefit/knowledge-graph/pkg/schema"
	"github.com/risefit/knowledge-graph/pkg/vectorstore"
	"google.golang.org/api/iterator"
	googleopt "google.golang.org/api/option"
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
	registryCollection := os.Getenv("QDRANT_REGISTRY_COLLECTION")

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

	store, err := vectorstore.NewQdrantStore(ctx, embedConfig, qdrantURL, qdrantAPIKey, collectionName, registryCollection)
	if err != nil {
		log.Fatalf("Error initializing vector store: %v", err)
	}

	// Initialize Google GenAI Client for PDF Parsing
	googleAPIKey := os.Getenv("GOOGLE_API_KEY")
	if googleAPIKey == "" {
		log.Fatal("GOOGLE_API_KEY is required for PDF parsing")
	}
	genaiClient, err := genai.NewClient(ctx, googleopt.WithAPIKey(googleAPIKey))
	if err != nil {
		log.Fatalf("Error creating genai client: %v", err)
	}
	defer genaiClient.Close()

	// Initialize GCS Client
	gcsClient, err := storage.NewClient(ctx, googleopt.WithAPIKey(googleAPIKey))
	if err != nil {
		log.Fatalf("Error creating gcs client: %v", err)
	}
	defer gcsClient.Close()

	parsingModel := os.Getenv("PDF_PARSING_MODEL")
	if parsingModel == "" {
		parsingModel = "gemini-2.5-flash"
	}

	pdfPath := "Cover Confirmation.pdf"
	if len(os.Args) > 1 {
		pdfPath = os.Args[1]
	}

	// If GCS_BUCKET_NAME is set and path is not a gs:// URI,
	// try to treat it as an object in that bucket unless it's a local file.
	bucketName := os.Getenv("GCS_BUCKET_NAME")
	if bucketName != "" && !strings.HasPrefix(pdfPath, "gs://") {
		// Check if it's a local file first
		if _, err := os.Stat(pdfPath); os.IsNotExist(err) {
			// If local file doesn't exist, prefix with GCS bucket
			pdfPath = fmt.Sprintf("gs://%s/%s", bucketName, pdfPath)
		}
	}

	// 2. Resolve PDF paths (recursive if directory/bucket)
	pdfPaths, err := resolvePaths(ctx, gcsClient, pdfPath)
	if err != nil {
		log.Fatalf("Error resolving paths: %v", err)
	}

	if len(pdfPaths) == 0 {
		log.Printf("No PDF files found at %s", pdfPath)
		return
	}

	fmt.Printf("Found %d PDF files to process at %s\n", len(pdfPaths), pdfPath)

	// Initialize PDF Processor
	processor := pdf.NewProcessor(genaiClient, gcsClient, parsingModel)

	for i, path := range pdfPaths {
		fmt.Printf("[%d/%d] Processing: %s\n", i+1, len(pdfPaths), path)

		// Create a unique ID for the document based on its path
		docID := fmt.Sprintf("%x", md5.Sum([]byte(path)))

		// Check if already processed
		if registryCollection != "" {
			record, err := store.GetDocumentRecord(ctx, docID)
			if err == nil && record != nil && record.Status == "completed" {
				fmt.Printf("  Already processed (status: completed), skipping.\n")
				continue
			}
		}

		// Update registry status to "processing"
		now := time.Now().Format(time.RFC3339)
		docTitle := filepath.Base(path)

		// Note: we'd ideally extract category/topic here to store in registry too
		// For simplicity, let the processor do its thing first

		docs, err := processor.LoadAndSplit(ctx, path)
		if err != nil {
			log.Printf("  Error processing %s: %v", path, err)
			store.UpsertDocumentRecord(ctx, schema.DocumentRecord{
				ID:            docID,
				DocumentTitle: docTitle,
				Path:          path,
				Status:        "failed",
				UpdatedAt:     now,
			})
			continue
		}

		if len(docs) == 0 {
			log.Printf("  No content extracted from %s", path)
			continue
		}

		// Add to Vector Store
		err = store.AddDocuments(ctx, docs)
		if err != nil {
			log.Printf("  Error adding %s to store: %v", path, err)
			continue
		}

		// Final Registry Update
		category := "uncategorized"
		topic := "general"
		if len(docs) > 0 {
			if cat, ok := docs[0].Metadata["category"].(string); ok {
				category = cat
			}
			if top, ok := docs[0].Metadata["topic"].(string); ok {
				topic = top
			}
		}

		err = store.UpsertDocumentRecord(ctx, schema.DocumentRecord{
			ID:            docID,
			DocumentTitle: docTitle,
			Path:          path,
			Category:      category,
			Topic:         topic,
			Status:        "completed",
			PageCount:     len(docs),
			CreatedAt:     now,
			UpdatedAt:     now,
		})
		if err != nil {
			log.Printf("  Warning: failed to update registry for %s: %v", path, err)
		}

		fmt.Printf("  Successfully stored %d pages\n", len(docs))
	}

	fmt.Println("Batch processing complete!")
}

func resolvePaths(ctx context.Context, gcsClient *storage.Client, rootPath string) ([]string, error) {
	var paths []string

	// Handle GCS
	if strings.HasPrefix(rootPath, "gs://") {
		if gcsClient == nil {
			return nil, fmt.Errorf("gcs client required for gs:// paths")
		}
		path := strings.TrimPrefix(rootPath, "gs://")
		parts := strings.SplitN(path, "/", 2)
		bucketName := parts[0]
		prefix := ""
		if len(parts) > 1 {
			prefix = parts[1]
		}

		it := gcsClient.Bucket(bucketName).Objects(ctx, &storage.Query{Prefix: prefix})
		for {
			attrs, err := it.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return nil, err
			}
			if strings.HasSuffix(strings.ToLower(attrs.Name), ".pdf") {
				paths = append(paths, fmt.Sprintf("gs://%s/%s", bucketName, attrs.Name))
			}
		}
		return paths, nil
	}

	// Handle Local
	info, err := os.Stat(rootPath)
	if err != nil {
		return nil, err
	}

	if !info.IsDir() {
		if strings.HasSuffix(strings.ToLower(rootPath), ".pdf") {
			return []string{rootPath}, nil
		}
		return nil, nil
	}

	err = filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(strings.ToLower(path), ".pdf") {
			paths = append(paths, path)
		}
		return nil
	})

	return paths, err
}
