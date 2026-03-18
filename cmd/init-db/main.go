package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"net/url"

	"github.com/joho/godotenv"
	"github.com/qdrant/go-client/qdrant"
)

func main() {
	// Load .env file
	_ = godotenv.Load()

	qdrantURLStr := os.Getenv("QDRANT_URL")
	qdrantAPIKey := os.Getenv("QDRANT_API_KEY")
	contentCollection := os.Getenv("QDRANT_COLLECTION_NAME")
	registryCollection := os.Getenv("QDRANT_REGISTRY_COLLECTION")

	if qdrantURLStr == "" || contentCollection == "" {
		log.Fatal("QDRANT_URL and QDRANT_COLLECTION_NAME are required in .env")
	}

	ctx := context.Background()

	// Parse URL for gRPC client
	u, err := url.Parse(qdrantURLStr)
	if err != nil {
		log.Fatalf("Failed to parse QDRANT_URL: %v", err)
	}

	host := u.Hostname()
	portStr := u.Port()
	if portStr == "" {
		portStr = "6334"
	}
	port, _ := strconv.Atoi(portStr)

	client, err := qdrant.NewClient(&qdrant.Config{
		Host:   host,
		Port:   port,
		APIKey: qdrantAPIKey,
		UseTLS: u.Scheme == "https",
	})
	if err != nil {
		log.Fatalf("Failed to create Qdrant client: %v", err)
	}
	defer client.Close()

	// 1. Create Content Collection
	fmt.Printf("Creating content collection: %s...\n", contentCollection)
	err = client.CreateCollection(ctx, &qdrant.CreateCollection{
		CollectionName: contentCollection,
		VectorsConfig: qdrant.NewVectorsConfigMap(map[string]*qdrant.VectorParams{
			"dense-vector": {
				Size:     1536, // Matches OpenAI and Google 004
				Distance: qdrant.Distance_Cosine,
			},
		}),
	})
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			fmt.Printf("  Collection %s already exists, skipping.\n", contentCollection)
		} else {
			log.Fatalf("Failed to create content collection: %v", err)
		}
	} else {
		fmt.Printf("  Successfully created %s\n", contentCollection)
	}

	// 2. Create Registry Collection (Metadata-only style)
	if registryCollection != "" {
		fmt.Printf("Creating registry collection: %s...\n", registryCollection)
		err = client.CreateCollection(ctx, &qdrant.CreateCollection{
			CollectionName: registryCollection,
			VectorsConfig: qdrant.NewVectorsConfigMap(map[string]*qdrant.VectorParams{
				"none": {
					Size:     1, // Minimal dummy vector for metadata-only storage
					Distance: qdrant.Distance_Cosine,
				},
			}),
		})
		if err != nil {
			if strings.Contains(err.Error(), "already exists") {
				fmt.Printf("  Collection %s already exists, skipping.\n", registryCollection)
			} else {
				log.Fatalf("Failed to create registry collection: %v", err)
			}
		} else {
			fmt.Printf("  Successfully created %s\n", registryCollection)
		}
	}

	fmt.Println("Qdrant initialization complete!")
}
