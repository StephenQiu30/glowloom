package workflow

import (
	"context"
	"errors"

	"github.com/StephenQiu30/lanverse/backend/internal/media/adapter/document"

	"github.com/StephenQiu30/lanverse/backend/internal/media/adapter/gltf"

	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstorage"
)

// FFUploadProber exposes malformed input as a stable upload validation error.
type FFUploadProber struct{ model *gltf.Validator }

// NewUploadProber injects the offline GLB validator alongside real AV probing.
func NewUploadProber(model *gltf.Validator) FFUploadProber { return FFUploadProber{model: model} }

// Probe verifies real streams and maps invalid input to the upload boundary error.
func (p FFUploadProber) Probe(ctx context.Context, file *application.Downloaded) (application.ProbeResult, error) {
	if file != nil && (file.MIMEType == domain.MIMEText || file.MIMEType == domain.MIMEDOCX) {
		return document.Probe(ctx, file)
	}
	if file != nil && file.MIMEType == "model/gltf-binary" {
		if p.model == nil {
			return application.ProbeResult{}, application.ErrUnavailable
		}
		if file.File == nil {
			return application.ProbeResult{}, application.ErrInvalidUpload
		}
		if err := p.model.Validate(ctx, file.File, file.Size); err != nil {
			if errors.Is(err, gltf.ErrDecoderUnavailable) {
				return application.ProbeResult{}, application.ErrUnavailable
			}
			if errors.Is(err, gltf.ErrInvalidModel) {
				return application.ProbeResult{}, application.ErrUnsupportedUpload
			}
			return application.ProbeResult{}, err
		}
		codec := "glb2"
		return application.ProbeResult{Kind: domain.KindModel, Extension: "glb", Codec: &codec}, nil
	}
	result, err := probeFile(ctx, file, true)
	if errors.Is(err, ErrUnsupportedMedia) {
		return result, application.ErrUnsupportedUpload
	}
	return result, err
}

// FFUploadRenderer adds a real PNG waveform for local audio uploads without
// changing generated output's existing rendition contract.
type FFUploadRenderer struct{}

// Render creates the existing image/video previews or a decoded audio waveform.
func (FFUploadRenderer) Render(ctx context.Context, file *application.Downloaded, probe application.ProbeResult, aspect string) ([]application.RenditionFile, error) {
	if probe.Kind == domain.KindModel || probe.Kind == domain.KindDocument {
		return nil, ctx.Err()
	}
	if probe.Kind != domain.KindAudio {
		return (FFRenderer{}).Render(ctx, file, probe, aspect)
	}
	if file == nil || file.File == nil {
		return nil, application.ErrInvalidUpload
	}
	result, err := renderOne(ctx, file.File.Name(), renderSpec{kind: domain.RenditionWaveform, ext: "png", mimeType: "image/png",
		args: []string{"-filter_complex", "showwavespic=s=1280x256:colors=0x67e8f9", "-frames:v", "1", "-c:v", "png"}})
	if err != nil {
		return nil, err
	}
	return []application.RenditionFile{result}, nil
}

// NewUploadObjects reuses the immutable private bucket adapter for local uploads.
func NewUploadObjects(client *objectstorage.Client) application.UploadObjects {
	return objectStore{client: client}
}
func (s objectStore) Remove(ctx context.Context, key string) error { return s.client.Remove(ctx, key) }
