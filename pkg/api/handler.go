package api

import (
	"encoding/json"
	"net/http"

	"github.com/risefit/knowledge-graph/pkg/vectorstore"
)

type SearchRequest struct {
	Query      string `json:"query"`
	NumResults int    `json:"num_results"`
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
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req SearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Query == "" {
		http.Error(w, "Query is required", http.StatusBadRequest)
		return
	}

	if req.NumResults <= 0 {
		req.NumResults = 5 // Default
	}

	docs, err := h.store.SimilaritySearch(r.Context(), req.Query, req.NumResults)
	if err != nil {
		http.Error(w, "Search failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

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
