package media

import (
	"context"
	"errors"

	"golang.org/x/sync/semaphore"

	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/processing/adapter/ffmpeg"
	processingapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/processing/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

const defaultNormalizationConcurrency int64 = 2

// LocalNormalizer performs bounded in-process WebM-to-H264 MP4 normalization via FFmpeg.
type LocalNormalizer struct {
	normalizer ffmpeg.Normalizer
	slots      *semaphore.Weighted
}

// Normalize borrows source, runs bounded FFmpeg transcoding, and transfers ownership of the result file.
func (n *LocalNormalizer) Normalize(ctx context.Context, source *mediaapp.Downloaded) (mediaapp.NormalizedUpload, error) {
	if err := n.slots.Acquire(ctx, 1); err != nil {
		return mediaapp.NormalizedUpload{}, err
	}
	defer n.slots.Release(1)

	var input *processingapp.File
	if source != nil {
		input = &processingapp.File{File: source.File, Size: source.Size, SHA256: source.SHA256, MIMEType: source.MIMEType}
	}
	result, err := n.normalizer.Normalize(ctx, input)
	if err != nil {
		switch {
		case errors.Is(err, processingapp.ErrTooLarge):
			return mediaapp.NormalizedUpload{}, mediaapp.ErrUploadTooLarge
		case errors.Is(err, processingapp.ErrUnsupported), errors.Is(err, processingapp.ErrInvalidInput):
			return mediaapp.NormalizedUpload{}, mediaapp.ErrInvalidUpload
		default:
			return mediaapp.NormalizedUpload{}, err
		}
	}
	return mediaapp.NormalizedUpload{
		File: &mediaapp.Downloaded{
			File:     result.File.File,
			Size:     result.File.Size,
			SHA256:   result.File.SHA256,
			MIMEType: result.File.MIMEType,
		},
		SourceCodec: result.SourceCodec,
	}, nil
}

// ProvideUploadNormalizer configures the bounded in-process FFmpeg normalizer.
func ProvideUploadNormalizer(_ config.Config) (*LocalNormalizer, error) {
	return &LocalNormalizer{
		normalizer: ffmpeg.Normalizer{},
		slots:      semaphore.NewWeighted(defaultNormalizationConcurrency),
	}, nil
}
