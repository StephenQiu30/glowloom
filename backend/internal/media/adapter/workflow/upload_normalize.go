package workflow

import (
	"context"
	"errors"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/processing/adapter/ffmpeg"
	processingapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/processing/application"
)

// FFUploadNormalizer adapts the file-only native normalizer to upload contracts.
// It is not a fallback for remote processing failures.
type FFUploadNormalizer struct{}

// Normalize borrows the upload source and transfers ownership of canonical bytes.
func (FFUploadNormalizer) Normalize(ctx context.Context, source *application.Downloaded) (application.NormalizedUpload, error) {
	var input *processingapp.File
	if source != nil {
		input = &processingapp.File{File: source.File, Size: source.Size, SHA256: source.SHA256, MIMEType: source.MIMEType}
	}
	result, err := (ffmpeg.Normalizer{}).Normalize(ctx, input)
	if err != nil {
		switch {
		case errors.Is(err, processingapp.ErrInvalidInput):
			return application.NormalizedUpload{}, application.ErrInvalidUpload
		case errors.Is(err, processingapp.ErrUnsupported):
			return application.NormalizedUpload{}, application.ErrUnsupportedUpload
		case errors.Is(err, processingapp.ErrTooLarge):
			return application.NormalizedUpload{}, application.ErrUploadTooLarge
		default:
			return application.NormalizedUpload{}, err
		}
	}
	file := result.File
	return application.NormalizedUpload{
		File:        &application.Downloaded{File: file.File, Size: file.Size, SHA256: file.SHA256, MIMEType: file.MIMEType},
		SourceCodec: result.SourceCodec,
	}, nil
}
