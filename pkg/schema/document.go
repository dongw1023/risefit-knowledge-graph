package schema

// Document represents a chunk of text with metadata.
type Document struct {
	PageContent string
	Metadata    map[string]any
	Score       float32
}
