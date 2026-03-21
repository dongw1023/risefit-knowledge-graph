package api

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/risefit/knowledge-graph/pkg/vectorstore"
)

type SearchRequest struct {
	Query      string          `json:"query"`
	NumResults int             `json:"num_results"`
	Filters    MetadataFilters `json:"filters"`
}

type MetadataFilters struct {
	Category       string `json:"category,omitempty"`
	Intent         string `json:"intent,omitempty"`
	TargetAudience string `json:"target_audience,omitempty"`
	EvidenceLevel  string `json:"evidence_level,omitempty"`
}

type SearchResponse struct {
	Results []SearchResult `json:"results"`
}

type SearchResult struct {
	Content  string         `json:"content"`
	Metadata map[string]any `json:"metadata"`
	Score    float32        `json:"score"`
}

type Handler struct {
	store *vectorstore.Store
}

func NewHandler(store *vectorstore.Store) *Handler {
	return &Handler{store: store}
}

func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		log.Printf("Method not allowed: %s", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req SearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("Invalid request body: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Query == "" {
		log.Printf("Empty query")
		http.Error(w, "Query is required", http.StatusBadRequest)
		return
	}

	log.Printf("Search request: query=%q, num_results=%d, filters=%+v", req.Query, req.NumResults, req.Filters)

	if req.NumResults <= 0 {
		req.NumResults = 5 // Default
	}

	filter := vectorstore.Filter{
		Category:       req.Filters.Category,
		Intent:         req.Filters.Intent,
		TargetAudience: req.Filters.TargetAudience,
		EvidenceLevel:  req.Filters.EvidenceLevel,
	}

	docs, err := h.store.SimilaritySearch(r.Context(), req.Query, req.NumResults, filter)
	if err != nil {
		log.Printf("Search failed: %v", err)
		http.Error(w, "Search failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("Search successful: found %d results", len(docs))

	results := make([]SearchResult, len(docs))
	for i, doc := range docs {
		results[i] = SearchResult{
			Content:  doc.PageContent,
			Metadata: doc.Metadata,
			Score:    doc.Score,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(SearchResponse{Results: results})
}
