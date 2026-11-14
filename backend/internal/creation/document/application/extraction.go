// Package application defines Creation's file-only document extraction contract.
package application

import (
	"context"
	"errors"
	"os"

	"github.com/StephenQiu30/glowloom-server/backend/internal/script/domain"
)

// MaxBytes bounds the original TXT or DOCX, not the canonical rich JSON.
const MaxBytes int64 = 20 << 20

// ErrExtractionFailed rejects originals that cannot be faithfully extracted.
var ErrExtractionFailed = errors.New("creation document extraction failed")

// File describes a caller-owned original. Extraction never closes or removes it.
type File struct {
	File     *os.File
	Size     int64
	SHA256   string
	MIMEType string
}

// Result preserves editable content, original encoding and provable origins.
type Result struct {
	Document domain.RichDocument
	Encoding string
	Mapping  []domain.SourceOriginMapping
	Warnings []domain.SourceExtractionWarning
}

// Extractor borrows an original without changing its ownership or file cursor.
type Extractor interface {
	Extract(context.Context, *File) (Result, error)
}
