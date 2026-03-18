package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"

	"github.com/qdrant/go-client/qdrant"
	"github.com/risefit/knowledge-graph/pkg/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	if cfg.QdrantURL == "" || cfg.QdrantCollectionName == "" {
		log.Fatal("QDRANT_URL and QDRANT_COLLECTION_NAME are required in .env")
	}

	ctx := context.Background()

	u, err := url.Parse(cfg.QdrantURL)
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
		APIKey: cfg.QdrantAPIKey,
		UseTLS: u.Scheme == "https",
	})
	if err != nil {
		log.Fatalf("Failed to create Qdrant client: %v", err)
	}
	defer client.Close()

	// 1. Create Content Collection
	fmt.Printf("Creating content collection: %s...\n", cfg.QdrantCollectionName)
	err = client.CreateCollection(ctx, &qdrant.CreateCollection{
		CollectionName: cfg.QdrantCollectionName,
		VectorsConfig: qdrant.NewVectorsConfigMap(map[string]*qdrant.VectorParams{
			"dense-vector": {
				Size:     1536,
				Distance: qdrant.Distance_Cosine,
			},
		}),
	})
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			fmt.Printf("  Collection %s already exists, skipping.\n", cfg.QdrantCollectionName)
		} else {
			log.Fatalf("Failed to create content collection: %v", err)
		}
	} else {
		fmt.Printf("  Successfully created %s\n", cfg.QdrantCollectionName)
	}

	// 2. Create Registry Collection
	if cfg.QdrantRegistryCollection != "" {
		fmt.Printf("Creating registry collection: %s...\n", cfg.QdrantRegistryCollection)
		err = client.CreateCollection(ctx, &qdrant.CreateCollection{
			CollectionName: cfg.QdrantRegistryCollection,
			VectorsConfig: qdrant.NewVectorsConfigMap(map[string]*qdrant.VectorParams{
				"none": {
					Size:     1,
					Distance: qdrant.Distance_Cosine,
				},
			}),
		})
		if err != nil {
			if strings.Contains(err.Error(), "already exists") {
				fmt.Printf("  Collection %s already exists, skipping.\n", cfg.QdrantRegistryCollection)
			} else {
				log.Fatalf("Failed to create registry collection: %v", err)
			}
		} else {
			fmt.Printf("  Successfully created %s\n", cfg.QdrantRegistryCollection)
		}
	}

	fmt.Println("Qdrant initialization complete!")
}
