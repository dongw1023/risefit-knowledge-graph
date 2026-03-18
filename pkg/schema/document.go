package schema

// Document represents a chunk of text with metadata.
type Document struct {
	PageContent string
	Metadata    map[string]any
	Score       float32
}

// DocumentRecord represents document-level metadata for the registry.
type DocumentRecord struct {
	ID             string         `json:"id"`
	DocumentTitle  string         `json:"document_title"`
	Path           string         `json:"path"`
	Category       string         `json:"category"`        // Core Topic
	Intent         string         `json:"intent"`          // Trigger Intent
	TargetAudience string         `json:"target_audience"` // Target Audience
	EvidenceLevel  string         `json:"evidence_level"`  // Evidence Level
	Status         string         `json:"status"`          // e.g., "processing", "completed", "failed"
	PageCount      int            `json:"page_count"`
	CreatedAt      string         `json:"created_at"`
	UpdatedAt      string         `json:"updated_at"`
	Metadata       map[string]any `json:"metadata"`
}
