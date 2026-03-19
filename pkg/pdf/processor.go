package pdf

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/google/generative-ai-go/genai"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/risefit/knowledge-graph/pkg/schema"
)

type Processor struct {
	client    *genai.Client
	gcsClient *storage.Client
	modelName string
}

func NewProcessor(client *genai.Client, gcsClient *storage.Client, modelName string) *Processor {
	if modelName == "" {
		modelName = "gemini-2.5-flash"
	}
	return &Processor{
		client:    client,
		gcsClient: gcsClient,
		modelName: modelName,
	}
}

func (p *Processor) LoadAndSplit(ctx context.Context, pdfPath string) ([]schema.Document, error) {
	localPath := pdfPath
	var isTemp bool

	if strings.HasPrefix(pdfPath, "gs://") {
		fmt.Printf("Downloading PDF from GCS: %s...\n", pdfPath)
		var err error
		localPath, err = p.downloadFromGCS(ctx, pdfPath)
		if err != nil {
			return nil, fmt.Errorf("failed to download from GCS: %w", err)
		}
		isTemp = true
	}

	if isTemp {
		defer os.Remove(localPath)
	}

	docTitle := filepath.Base(pdfPath)
	fmt.Printf("Processing: %s\n", docTitle)

	// 1. Prepare Temporary Directories
	tempDir, err := os.MkdirTemp("", "pdf-pages-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// 2. Optimized Splitting Logic
	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed

	// Attempt to Optimize/Repair the PDF first.
	// This often fixes the "invalid dict entry: XMP" errors found in scientific papers.
	fmt.Printf("  Cleaning and splitting PDF locally...\n")
	optimizedPath := localPath + ".opt"
	err = api.OptimizeFile(localPath, optimizedPath, conf)
	if err == nil {
		localPath = optimizedPath
		defer os.Remove(optimizedPath)
	} else {
		fmt.Printf("  Note: Optimization skipped (%v), attempting direct split...\n", err)
	}

	// Now split the (potentially cleaned) file
	err = api.SplitFile(localPath, tempDir, 1, conf)
	if err != nil {
		return nil, fmt.Errorf("failed to split PDF locally: %w. Please check if the PDF is password protected or corrupted", err)
	}

	// 3. Process each page file
	files, err := os.ReadDir(tempDir)
	if err != nil {
		return nil, err
	}

	var allDocs []schema.Document
	var smartCategory, smartIntent, smartTarget, smartEvidence string
	category, _ := p.extractTags(pdfPath)
	re := regexp.MustCompile(`_(\d+)\.pdf$`)

	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".pdf") {
			continue
		}

		pagePath := filepath.Join(tempDir, f.Name())
		pageNo := 1
		if match := re.FindStringSubmatch(f.Name()); len(match) > 1 {
			pageNo, _ = strconv.Atoi(match[1])
		}

		fmt.Printf("  Parsing page %d with Gemini...\n", pageNo)
		pageContent, err := p.extractPageWithGemini(ctx, pagePath)
		if err != nil {
			fmt.Printf("  Warning: failed to parse page %d: %v\n", pageNo, err)
			continue
		}

		// Analyze dimensions from the first successful page parse
		if smartCategory == "" && pageContent != "" {
			fmt.Printf("  Determining document type and topic...\n")
			smartCategory, smartIntent, smartTarget, smartEvidence = p.smartCategorize(ctx, pageContent)
			if smartCategory != "" {
				category = smartCategory
			}
			fmt.Printf("  Smart Tags: category=%s, intent=%s, target=%s, evidence=%s\n", category, smartIntent, smartTarget, smartEvidence)
		}

		now := time.Now().Format(time.RFC3339)
		allDocs = append(allDocs, schema.Document{
			PageContent: pageContent,
			Metadata: map[string]any{
				"document_title":  docTitle,
				"page_number":     pageNo,
				"category":        category,
				"intent":          smartIntent,
				"target_audience": smartTarget,
				"evidence_level":  smartEvidence,
				"created_at":      now,
				"updated_at":      now,
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
		genai.Blob{MIMEType: "application/pdf", Data: data},
		genai.Text("Extract all content from this PDF page. For tables, format them as Markdown tables. For graphs or images, provide a detailed textual description. Output in clean Markdown format."),
	}

	resp, err := model.GenerateContent(ctx, prompt...)
	if err != nil {
		return "", err
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("gemini returned no content")
	}

	var builder strings.Builder
	for _, part := range resp.Candidates[0].Content.Parts {
		if text, ok := part.(genai.Text); ok {
			builder.WriteString(string(text))
		}
	}
	return builder.String(), nil
}

func (p *Processor) smartCategorize(ctx context.Context, firstPageContent string) (string, string, string, string) {
	model := p.client.GenerativeModel(p.modelName)
	prompt := fmt.Sprintf(`Analyze the following text from the first page of a fitness/nutrition document. 
Assign exactly one tag for each dimension below.

Dimensions: 
1. Category: hypertrophy, fat_loss, nutrition, biomechanics, recovery
2. Intent: programming, myth_busting, form_correction, general_guidance
3. Target Audience: general, female, special_population
4. Evidence Level: position_stand, meta_analysis, textbook

Return format: CATEGORY|INTENT|TARGET|EVIDENCE
Text:
%s`, firstPageContent)

	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return "", "", "", ""
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", "", "", ""
	}

	var result string
	if part, ok := resp.Candidates[0].Content.Parts[0].(genai.Text); ok {
		result = string(part)
	}

	parts := strings.Split(strings.TrimSpace(result), "|")
	cat, intent, target, evidence := "uncategorized", "general_guidance", "general", "textbook"

	if len(parts) >= 1 {
		cat = strings.ToLower(strings.TrimSpace(parts[0]))
	}
	if len(parts) >= 2 {
		intent = strings.ToLower(strings.TrimSpace(parts[1]))
	}
	if len(parts) >= 3 {
		target = strings.ToLower(strings.TrimSpace(parts[2]))
	}
	if len(parts) >= 4 {
		evidence = strings.ToLower(strings.TrimSpace(parts[3]))
	}

	return cat, intent, target, evidence
}

func (p *Processor) extractTags(path string) (string, string) {
	cleanPath := path
	if strings.HasPrefix(path, "gs://") {
		parts := strings.SplitN(strings.TrimPrefix(path, "gs://"), "/", 2)
		if len(parts) < 2 {
			return "unknown", "unknown"
		}
		cleanPath = parts[1]
	}
	dir := filepath.Dir(cleanPath)
	if dir == "." || dir == "/" {
		return "uncategorized", "general"
	}
	parts := strings.Split(filepath.ToSlash(dir), "/")
	category, topic := "uncategorized", "general"
	if len(parts) >= 1 {
		category = parts[0]
	}
	if len(parts) >= 2 {
		topic = parts[1]
	}
	return category, topic
}

func (p *Processor) downloadFromGCS(ctx context.Context, gcsPath string) (string, error) {
	if p.gcsClient == nil {
		return "", fmt.Errorf("gcs client not initialized")
	}
	path := strings.TrimPrefix(gcsPath, "gs://")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid gcs path: %s", gcsPath)
	}
	bucketName, objectName := parts[0], parts[1]
	bucket := p.gcsClient.Bucket(bucketName)
	obj := bucket.Object(objectName)
	r, err := obj.NewReader(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to create reader for gcs object: %w", err)
	}
	defer r.Close()
	tmpFile, err := os.CreateTemp("", "gcs-pdf-*.pdf")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpFilename := tmpFile.Name()
	if _, err := io.Copy(tmpFile, r); err != nil {
		tmpFile.Close()
		os.Remove(tmpFilename)
		return "", fmt.Errorf("failed to copy gcs object to temp file: %w", err)
	}
	tmpFile.Close()
	return tmpFilename, nil
}
