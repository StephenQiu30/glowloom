package media_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

func normalizedUploadProof() (mediaapp.UploadRequest, domain.MediaAsset, mediaapp.UploadNormalization) {
	sourceSHA, canonicalSHA := strings.Repeat("a", 64), strings.Repeat("b", 64)
	width, height, duration := int32(64), int32(48), int32(600)
	codec := "h264"
	request := mediaapp.UploadRequest{ProjectID: uuid.New(), Key: uuid.New(), RequestID: uuid.New(), SHA256: sourceSHA, FileName: "镜头.webm", ByteSize: 1500}
	asset := domain.MediaAsset{Kind: domain.KindVideo, SHA256: &canonicalSHA, FileName: "镜头.mp4", ByteSize: 1900, MimeType: "video/mp4",
		ObjectKey: "projects/normalized.mp4", Codec: &codec, Width: &width, Height: &height, DurationMS: &duration}
	proof := mediaapp.UploadNormalization{Version: 1, Method: "webm_vp8_vp9_to_mp4_h264",
		Source:    mediaapp.UploadSourceFacts{SHA256: sourceSHA, FileName: request.FileName, ByteSize: request.ByteSize, MIMEType: "video/webm", Codec: "vp9"},
		Canonical: mediaapp.UploadCanonicalFacts{SHA256: canonicalSHA, ByteSize: asset.ByteSize, MIMEType: asset.MimeType, Codec: codec, Width: width, Height: height, DurationMS: duration}}
	return request, asset, proof
}

func TestUploadNormalizationProofRejectsChangedInputAndCanonicalFacts(t *testing.T) {
	request, asset, proof := normalizedUploadProof()
	if err := proof.Validate(request, asset); err != nil {
		t.Fatalf("valid typed conversion proof: %v", err)
	}
	for _, tc := range []struct {
		name string
		edit func(*mediaapp.UploadNormalization)
	}{
		{"version", func(p *mediaapp.UploadNormalization) { p.Version = 2 }},
		{"method", func(p *mediaapp.UploadNormalization) { p.Method = "rename" }},
		{"source sha", func(p *mediaapp.UploadNormalization) { p.Source.SHA256 = strings.Repeat("c", 64) }},
		{"source name", func(p *mediaapp.UploadNormalization) { p.Source.FileName = "替换.webm" }},
		{"source bytes", func(p *mediaapp.UploadNormalization) { p.Source.ByteSize++ }},
		{"source mime", func(p *mediaapp.UploadNormalization) { p.Source.MIMEType = "video/mp4" }},
		{"source codec", func(p *mediaapp.UploadNormalization) { p.Source.Codec = "av1" }},
		{"canonical sha", func(p *mediaapp.UploadNormalization) { p.Canonical.SHA256 = strings.Repeat("c", 64) }},
		{"canonical byte count", func(p *mediaapp.UploadNormalization) { p.Canonical.ByteSize++ }},
		{"canonical mime", func(p *mediaapp.UploadNormalization) { p.Canonical.MIMEType = "video/webm" }},
		{"canonical codec", func(p *mediaapp.UploadNormalization) { p.Canonical.Codec = "vp9" }},
		{"canonical width", func(p *mediaapp.UploadNormalization) { p.Canonical.Width++ }},
		{"canonical height", func(p *mediaapp.UploadNormalization) { p.Canonical.Height++ }},
		{"canonical duration", func(p *mediaapp.UploadNormalization) { p.Canonical.DurationMS++ }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := proof
			tc.edit(&changed)
			if err := changed.Validate(request, asset); !errors.Is(err, mediaapp.ErrInvalidUpload) {
				t.Fatalf("forged proof accepted: %v", err)
			}
		})
	}
	for _, tc := range []struct {
		name string
		edit func(*domain.MediaAsset)
	}{
		{"canonical filename", func(a *domain.MediaAsset) { a.FileName = "镜头.webm" }},
		{"canonical object", func(a *domain.MediaAsset) { a.ObjectKey = "projects/normalized.webm" }},
		{"asset kind", func(a *domain.MediaAsset) { a.Kind = domain.KindAudio }},
		{"missing output sha", func(a *domain.MediaAsset) { a.SHA256 = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := asset
			tc.edit(&changed)
			if err := proof.Validate(request, changed); !errors.Is(err, mediaapp.ErrInvalidUpload) {
				t.Fatalf("false canonical asset accepted: %v", err)
			}
		})
	}
}
