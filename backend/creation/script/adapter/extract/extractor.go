package extract

import (
	"context"
	"errors"

	"github.com/StephenQiu30/glowloom-server/backend/creation/document/adapter/native"
	documentapp "github.com/StephenQiu30/glowloom-server/backend/creation/document/application"
	"github.com/StephenQiu30/glowloom-server/backend/creation/script/application"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/media/application"
)

// Extractor adapts caller-owned upload files to Creation's native extractor.
type Extractor struct{}

// NewExtractor creates the native original-document boundary adapter.
func NewExtractor() *Extractor { return &Extractor{} }

// Extract only maps file/result contracts and preserves recognizable failures.
func (e *Extractor) Extract(ctx context.Context, file *mediaapp.Downloaded) (application.ExtractedDocument, error) {
	var source *documentapp.File
	if file != nil {
		source = &documentapp.File{File: file.File, Size: file.Size, SHA256: file.SHA256, MIMEType: file.MIMEType}
	}
	result, err := native.NewExtractor().Extract(ctx, source)
	if err != nil {
		if errors.Is(err, documentapp.ErrExtractionFailed) {
			return application.ExtractedDocument{}, errors.Join(application.ErrExtractionFailed, err)
		}
		return application.ExtractedDocument{}, err
	}
	return application.ExtractedDocument{
		Document: result.Document,
		Encoding: result.Encoding,
		Mapping:  result.Mapping,
		Warnings: result.Warnings,
	}, nil
}
