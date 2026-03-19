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
	"github.com/risefit/knowledge-graph/pkg/config"
	"github.com/risefit/knowledge-graph/pkg/pdf"
	"github.com/risefit/knowledge-graph/pkg/schema"
	"github.com/risefit/knowledge-graph/pkg/vectorstore"
	"google.golang.org/api/iterator"
	googleopt "google.golang.org/api/option"
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

	// Initialize Google GenAI Client for PDF Parsing
	if cfg.GoogleAPIKey == "" {
		log.Fatal("GOOGLE_API_KEY is required for PDF parsing")
	}
	genaiClient, err := genai.NewClient(ctx, googleopt.WithAPIKey(cfg.GoogleAPIKey))
	if err != nil {
		log.Fatalf("Error creating genai client: %v", err)
	}
	defer genaiClient.Close()

	// Initialize GCS Client
	var gcsOpts []googleopt.ClientOption
	if cfg.GoogleServiceAccountPath != "" {
		gcsOpts = append(gcsOpts, googleopt.WithCredentialsFile(cfg.GoogleServiceAccountPath))
	}
	// Note: We don't use WithAPIKey for GCS because 'list' operations are usually blocked for API Keys.
	// If GoogleServiceAccountPath is empty, storage.NewClient will automatically use Application Default Credentials.
	gcsClient, err := storage.NewClient(ctx, gcsOpts...)
	if err != nil {
		log.Fatalf("Error creating gcs client: %v", err)
	}
	defer gcsClient.Close()

	pdfPath := ""
	if len(os.Args) > 1 {
		pdfPath = os.Args[1]
	}

	// Default to the root of the GCS bucket if no path is provided
	if pdfPath == "" {
		if cfg.GCSBucketName != "" {
			pdfPath = fmt.Sprintf("gs://%s/", cfg.GCSBucketName)
			fmt.Printf("No path provided. Defaulting to GCS bucket root: %s\n", pdfPath)
		} else {
			fmt.Println("Usage: ./ingest [gs://bucket/path/ | ./local/path/ | filename.pdf]")
			fmt.Println("Error: No ingestion path provided and GCS_BUCKET_NAME_KG is not set.")
			return
		}
	}

	// Resolve PDF paths (recursive if directory/bucket)
	pdfPaths, err := resolvePaths(ctx, gcsClient, pdfPath)
	if err != nil {
		log.Fatalf("Error resolving paths: %v", err)
	}

	if len(pdfPaths) == 0 {
		log.Printf("No PDF files found at %s", pdfPath)
		return
	}

	fmt.Printf("Found %d PDF files to process at %s\n", len(pdfPaths), pdfPath)

	processor := pdf.NewProcessor(genaiClient, gcsClient, cfg.PDFParsingModel)

	for i, path := range pdfPaths {
		fmt.Printf("[%d/%d] Processing: %s\n", i+1, len(pdfPaths), path)
		docID := fmt.Sprintf("%x", md5.Sum([]byte(path)))

		if cfg.QdrantRegistryCollection != "" {
			record, err := store.GetDocumentRecord(ctx, docID)
			if err == nil && record != nil && record.Status == "completed" {
				fmt.Printf("  Already processed (status: completed), skipping.\n")
				continue
			}
		}

		now := time.Now().Format(time.RFC3339)
		docTitle := filepath.Base(path)

		docs, err := processor.LoadAndSplit(ctx, path)
		if err != nil {
			log.Printf("  Error processing %s: %v", path, err)
			if cfg.QdrantRegistryCollection != "" {
				store.UpsertDocumentRecord(ctx, schema.DocumentRecord{
					ID:            docID,
					DocumentTitle: docTitle,
					Path:          path,
					Status:        "failed",
					UpdatedAt:     now,
				})
			}
			continue
		}

		if len(docs) == 0 {
			log.Printf("  No content extracted from %s", path)
			continue
		}

		err = store.AddDocuments(ctx, docs)
		if err != nil {
			log.Printf("  Error adding %s to store: %v", path, err)
			continue
		}
		// Final Registry Update
		category := "uncategorized"
		intent := "general_guidance"
		target := "general"
		evidence := "textbook"

		if len(docs) > 0 {
			if v, ok := docs[0].Metadata["category"].(string); ok {
				category = v
			}
			if v, ok := docs[0].Metadata["intent"].(string); ok {
				intent = v
			}
			if v, ok := docs[0].Metadata["target_audience"].(string); ok {
				target = v
			}
			if v, ok := docs[0].Metadata["evidence_level"].(string); ok {
				evidence = v
			}
		}

		err = store.UpsertDocumentRecord(ctx, schema.DocumentRecord{
			ID:             docID,
			DocumentTitle:  docTitle,
			Path:           path,
			Category:       category,
			Intent:         intent,
			TargetAudience: target,
			EvidenceLevel:  evidence,
			Status:         "completed",
			PageCount:      len(docs),
			CreatedAt:      now,
			UpdatedAt:      now,
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

		bucket := gcsClient.Bucket(bucketName)

		// 1. Try treating it as a single object first
		if prefix != "" && !strings.HasSuffix(prefix, "/") {
			_, err := bucket.Object(prefix).Attrs(ctx)
			if err == nil {
				// It's a single file
				return []string{rootPath}, nil
			}
			fmt.Printf("  Debug: Metadata check for '%s' failed: %v\n", prefix, err)
		}

		// 2. Otherwise, treat as a prefix (folder)
		fmt.Printf("  Debug: Listing objects with prefix '%s' in bucket '%s'...\n", prefix, bucketName)
		it := bucket.Objects(ctx, &storage.Query{Prefix: prefix})
		for {
			attrs, err := it.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("failed to list objects in bucket %s: %w", bucketName, err)
			}
			if strings.HasSuffix(strings.ToLower(attrs.Name), ".pdf") {
				paths = append(paths, fmt.Sprintf("gs://%s/%s", bucketName, attrs.Name))
			}
		}

		if len(paths) == 0 {
			// Diagnostic: List first few items to help debug
			fmt.Printf("  Debug: No .pdf matches found for prefix '%s'. Listing first 3 items in bucket:\n", prefix)
			it := bucket.Objects(ctx, &storage.Query{})
			for i := 0; i < 3; i++ {
				attrs, err := it.Next()
				if err != nil {
					break
				}
				fmt.Printf("    - %s\n", attrs.Name)
			}
		}

		return paths, nil
	}

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
