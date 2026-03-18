package pdf

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/google/generative-ai-go/genai"
	"github.com/pdfcpu/pdfcpu/pkg/api"
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

	// Handle GCS paths
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

	fmt.Printf("Splitting PDF by pages and parsing with Gemini: %s...\n", localPath)

	// 1. Create temporary directory for single-page PDFs
	tempDir, err := os.MkdirTemp("", "pdf-pages-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir) // Ensure we clean up

	// 2. Extract pages using pdfcpu
	err = api.SplitFile(localPath, tempDir, 1, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to split pdf: %w", err)
	}

	// 3. Find all generated page files
	files, err := os.ReadDir(tempDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read temp dir: %w", err)
	}

	// Extract filename and initial tags from path
	docTitle := filepath.Base(pdfPath)
	category, _ := p.extractTags(pdfPath)

	var allDocs []schema.Document
	var smartCategory, smartIntent, smartTarget, smartEvidence string

	// Iterate through each page
	for i, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".pdf") {
			continue
		}

		pagePath := filepath.Join(tempDir, f.Name())

		// Extract page number
		pageNoStr := strings.TrimSuffix(strings.TrimPrefix(f.Name(), strings.TrimSuffix(docTitle, ".pdf")+"_"), ".pdf")
		pageNo, _ := strconv.Atoi(pageNoStr)

		fmt.Printf("  Parsing page %d...\n", pageNo)

		pageContent, err := p.extractPageWithGemini(ctx, pagePath)
		if err != nil {
			fmt.Printf("  Warning: failed to parse page %d: %v\n", pageNo, err)
			continue
		}

		// --- SMART TAGGING: Use first page content to categorize into 4 dimensions ---
		if i == 0 || (smartCategory == "" && pageContent != "") {
			fmt.Printf("  Analyzing document dimensions...\n")
			smartCategory, smartIntent, smartTarget, smartEvidence = p.smartCategorize(ctx, pageContent)
			if smartCategory != "" {
				category = smartCategory
			}
			fmt.Printf("  Dimensions: category=%s, intent=%s, target=%s, evidence=%s\n", category, smartIntent, smartTarget, smartEvidence)
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

func (p *Processor) smartCategorize(ctx context.Context, firstPageContent string) (string, string, string, string) {
	model := p.client.GenerativeModel(p.modelName)

	prompt := fmt.Sprintf(`Analyze the following text from the first page of a fitness/nutrition document. 
Assign exactly one tag for each of the four dimensions below.

Dimension 1: Category (Core Topic)
- hypertrophy (増肌机制)
- fat_loss (减脂与能量消耗)
- nutrition (营养与补剂)
- biomechanics (生物力学与动作)
- recovery (恢复与睡眠)

Dimension 2: Intent (Trigger Intent)
- programming (用于生成或调整训练计划)
- myth_busting (用于反驳用户的伪科学言论)
- form_correction (用于纠正动作姿态)
- general_guidance (用于日常知识问答)

Dimension 3: Target Audience
- general (所有人)
- female (女性专属)
- special_population (老人、高血压、孕妇等)

Dimension 4: Evidence Level
- position_stand (官方立场声明, 最高优先级)
- meta_analysis (元分析综述, 高级优先级)
- textbook (教材基础理论, 兜底解释)

Return the result strictly in this format: CATEGORY|INTENT|TARGET|EVIDENCE
Example: hypertrophy|programming|general|meta_analysis

Text:
%s`, firstPageContent)

	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return "", "", "", ""
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", "", "", ""
	}

	result := ""
	if part, ok := resp.Candidates[0].Content.Parts[0].(genai.Text); ok {
		result = string(part)
	}

	// Parse "CATEGORY|INTENT|TARGET|EVIDENCE"
	parts := strings.Split(strings.TrimSpace(result), "|")

	cat := "uncategorized"
	intent := "general_guidance"
	target := "general"
	evidence := "textbook"

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
		// gs://bucket/category/topic/file.pdf -> category/topic/file.pdf
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

	// Split the directory into parts
	parts := strings.Split(filepath.ToSlash(dir), "/")

	category := "uncategorized"
	topic := "general"

	if len(parts) >= 1 {
		category = parts[0]
	}
	if len(parts) >= 2 {
		topic = parts[1]
	}

	return category, topic
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

func (p *Processor) downloadFromGCS(ctx context.Context, gcsPath string) (string, error) {
	if p.gcsClient == nil {
		return "", fmt.Errorf("gcs client not initialized")
	}

	// gs://bucket/path/to/file
	path := strings.TrimPrefix(gcsPath, "gs://")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid gcs path: %s", gcsPath)
	}
	bucketName := parts[0]
	objectName := parts[1]

	bucket := p.gcsClient.Bucket(bucketName)
	obj := bucket.Object(objectName)

	r, err := obj.NewReader(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to create reader for gcs object: %w", err)
	}
	defer r.Close()

	// Create a temp file
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
