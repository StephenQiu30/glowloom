// Package application coordinates authorized immutable script use cases.
package application

import (
	"context"
	"errors"

	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
)

// ErrExtractionFailed preserves a per-file failure without accepting replacement text.
var ErrExtractionFailed = errors.New("script source extraction failed")

// ExtractionWarning reports an actual original-document feature not represented in rich text.
type ExtractionWarning = domain.SourceExtractionWarning

// SourceMapping records only provable decoded-text or Word-paragraph origins.
// DOCX mapping never invents byte/character offsets into serialized XML.
type SourceMapping = domain.SourceOriginMapping

// ExtractedDocument preserves editable content, original encoding and explicit warnings.
type ExtractedDocument struct {
	Document domain.RichDocument
	Encoding string
	Mapping  []SourceMapping
	Warnings []ExtractionWarning
}

// SourceExtractor consumes caller-owned originals and never closes their files.
type SourceExtractor interface {
	Extract(context.Context, *mediaapp.Downloaded) (ExtractedDocument, error)
}
