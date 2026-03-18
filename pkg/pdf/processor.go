package pdf

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/generative-ai-go/genai"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/risefit/knowledge-graph/pkg/schema"
)

type Processor struct {
	client    *genai.Client
	modelName string
}

func NewProcessor(client *genai.Client, modelName string, chunkSize, chunkOverlap int) *Processor {
	if modelName == "" {
		modelName = "gemini-2.5-flash"
	}
	return &Processor{
		client:    client,
		modelName: modelName,
	}
}

func (p *Processor) LoadAndSplit(ctx context.Context, pdfPath string) ([]schema.Document, error) {
	fmt.Printf("Splitting PDF by pages and parsing with Gemini: %s...\n", pdfPath)

	// 1. Create temporary directory for single-page PDFs
	tempDir, err := os.MkdirTemp("", "pdf-pages-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir) // Ensure we clean up

	// 2. Extract pages using pdfcpu
	err = api.SplitFile(pdfPath, tempDir, 1, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to split pdf: %w", err)
	}

	// 3. Find all generated page files
	files, err := os.ReadDir(tempDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read temp dir: %w", err)
	}

	// Extract filename without path as document title
	docTitle := filepath.Base(pdfPath)

	var allDocs []schema.Document

	// Iterate through each page
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".pdf") {
			continue
		}

		pagePath := filepath.Join(tempDir, f.Name())

		// Extract page number from filename (e.g., "manual_1.pdf" -> 1)
		// pdfcpu usually names them like <filename>_<page#>.pdf
		pageNoStr := strings.TrimSuffix(strings.TrimPrefix(f.Name(), strings.TrimSuffix(docTitle, ".pdf")+"_"), ".pdf")
		pageNo, _ := strconv.Atoi(pageNoStr)

		fmt.Printf("  Parsing page %d...\n", pageNo)

		pageContent, err := p.extractPageWithGemini(ctx, pagePath)
		if err != nil {
			fmt.Printf("  Warning: failed to parse page %d: %v\n", pageNo, err)
			continue
		}

		now := time.Now().Format(time.RFC3339)
		allDocs = append(allDocs, schema.Document{
			PageContent: pageContent,
			Metadata: map[string]any{
				"document_title": docTitle,
				"page_number":    pageNo,
				"created_at":     now,
				"updated_at":     now,
			},
		})
	}

	return allDocs, nil
}

func (p *Processor) extractPageWithGemini(ctx context.Context, pagePath string) (string, error) {
	data, err := os.ReadFile(pagePath)
	if err != nil {
		return "", err
	}

	model := p.client.GenerativeModel(p.modelName)
	prompt := []genai.Part{
		genai.Blob{
			MIMEType: "application/pdf",
			Data:     data,
		},
		genai.Text("Extract all content from this PDF page. For tables, format them as Markdown tables. For graphs or images, provide a detailed textual description. Output in clean Markdown format."),
	}

	resp, err := model.GenerateContent(ctx, prompt...)
	if err != nil {
		return "", err
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("gemini returned no content for page")
	}

	var builder strings.Builder
	for _, part := range resp.Candidates[0].Content.Parts {
		if text, ok := part.(genai.Text); ok {
			builder.WriteString(string(text))
		}
	}

	return builder.String(), nil
}
