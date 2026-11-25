package script

import (
	"context"
	"errors"

	"golang.org/x/sync/semaphore"

	"github.com/StephenQiu30/glowloom-server/backend/internal/creation/document/adapter/native"
	document "github.com/StephenQiu30/glowloom-server/backend/internal/creation/document/application"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/creation/script/application"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

const defaultExtractionConcurrency int64 = 2

type localExtractor struct {
	extractor *native.Extractor
	slots     *semaphore.Weighted
}

func (e *localExtractor) Extract(ctx context.Context, source *media.Downloaded) (app.ExtractedDocument, error) {
	if err := e.slots.Acquire(ctx, 1); err != nil {
		return app.ExtractedDocument{}, err
	}
	defer e.slots.Release(1)

	var file *document.File
	if source != nil {
		file = &document.File{File: source.File, Size: source.Size, SHA256: source.SHA256, MIMEType: source.MIMEType}
	}
	result, err := e.extractor.Extract(ctx, file)
	if err != nil {
		if errors.Is(err, document.ErrExtractionFailed) {
			return app.ExtractedDocument{}, app.ErrExtractionFailed
		}
		return app.ExtractedDocument{}, err
	}
	return app.ExtractedDocument{Document: result.Document, Encoding: result.Encoding, Mapping: result.Mapping, Warnings: result.Warnings}, nil
}

// ProvideSourceExtractor wires the bounded in-process TXT/Markdown/DOCX extractor.
func ProvideSourceExtractor(_ config.Config) (app.SourceExtractor, error) {
	return &localExtractor{
		extractor: native.NewExtractor(),
		slots:     semaphore.NewWeighted(defaultExtractionConcurrency),
	}, nil
}
