package pdf

import (
	"context"
	"fmt"
	"os"

	"github.com/tmc/langchaingo/documentloaders"
	"github.com/tmc/langchaingo/schema"
	"github.com/tmc/langchaingo/textsplitter"
)

type Processor struct {
	chunkSize    int
	chunkOverlap int
}

func NewProcessor(chunkSize, chunkOverlap int) *Processor {
	return &Processor{
		chunkSize:    chunkSize,
		chunkOverlap: chunkOverlap,
	}
}

func (p *Processor) LoadAndSplit(ctx context.Context, pdfPath string) ([]schema.Document, error) {
	file, err := os.Open(pdfPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open pdf: %w", err)
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	loader := documentloaders.NewPDF(file, fileInfo.Size())
	docs, err := loader.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load pdf: %w", err)
	}

	splitter := textsplitter.NewRecursiveCharacter(
		textsplitter.WithChunkSize(p.chunkSize),
		textsplitter.WithChunkOverlap(p.chunkOverlap),
	)

	splittedDocs, err := textsplitter.SplitDocuments(splitter, docs)
	if err != nil {
		return nil, fmt.Errorf("failed to split documents: %w", err)
	}

	return splittedDocs, nil
}
