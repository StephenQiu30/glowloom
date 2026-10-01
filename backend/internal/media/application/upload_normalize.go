package application

import (
	"context"
	"encoding/hex"
	"path"
	"strings"

	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

// UploadNormalizer fully decodes an accepted recording into a new canonical file.
// The caller owns the original; the consumer must Close the returned file.
type UploadNormalizer interface {
	Normalize(context.Context, *Downloaded) (NormalizedUpload, error)
}

// NormalizedUpload contains actual bytes and the decoded source video codec.
type NormalizedUpload struct {
	File        *Downloaded
	SourceCodec string
}

// UploadNormalization is a versioned proof linking an original upload receipt
// to its independently hashed and probed canonical asset.
type UploadNormalization struct {
	Version   int                  `json:"version"`
	Method    string               `json:"method"`
	Source    UploadSourceFacts    `json:"source"`
	Canonical UploadCanonicalFacts `json:"canonical"`
}

// UploadSourceFacts retain the original, server-read idempotency facts.
type UploadSourceFacts struct {
	SHA256   string `json:"sha256"`
	FileName string `json:"file_name"`
	ByteSize int64  `json:"byte_size"`
	MIMEType string `json:"mime_type"`
	Codec    string `json:"codec"`
}

// UploadCanonicalFacts describe the actual normalized original, not its proxy.
type UploadCanonicalFacts struct {
	SHA256     string `json:"sha256"`
	ByteSize   int64  `json:"byte_size"`
	MIMEType   string `json:"mime_type"`
	Codec      string `json:"codec"`
	Width      int32  `json:"width"`
	Height     int32  `json:"height"`
	DurationMS int32  `json:"duration_ms"`
}

// Validate binds both sides of the conversion without relaxing ordinary
// uploads' request SHA, byte count or display-name equality.
func (n UploadNormalization) Validate(r UploadRequest, asset domain.MediaAsset) error {
	source, canonical := n.Source, n.Canonical
	if ValidateUploadRequest(r) != nil || n.Version != 1 || n.Method != "webm_vp8_vp9_to_mp4_h264" ||
		source.SHA256 != r.SHA256 || source.FileName != r.FileName || source.ByteSize != r.ByteSize ||
		source.MIMEType != "video/webm" || !strings.EqualFold(path.Ext(source.FileName), ".webm") ||
		(source.Codec != "vp8" && source.Codec != "vp9") || asset.Kind != domain.KindVideo ||
		asset.SHA256 == nil || canonical.SHA256 != *asset.SHA256 || canonical.SHA256 == source.SHA256 ||
		!uploadFactHash(canonical.SHA256) || canonical.ByteSize != asset.ByteSize || canonical.ByteSize < 1 || canonical.ByteSize > MaxUploadVideoBytes ||
		canonical.MIMEType != "video/mp4" || canonical.MIMEType != asset.MimeType || canonical.Codec != "h264" ||
		asset.Codec == nil || canonical.Codec != *asset.Codec || asset.FileName != canonicalUploadName(r.FileName) ||
		path.Ext(asset.ObjectKey) != ".mp4" || asset.Width == nil || canonical.Width != *asset.Width ||
		asset.Height == nil || canonical.Height != *asset.Height || asset.DurationMS == nil || canonical.DurationMS != *asset.DurationMS ||
		canonical.Width < 2 || canonical.Width > 8192 || canonical.Height < 2 || canonical.Height > 8192 ||
		canonical.Width%2 != 0 || canonical.Height%2 != 0 || int64(canonical.Width)*int64(canonical.Height) > 40_000_000 ||
		canonical.DurationMS < 1 || canonical.DurationMS > 60_000 {
		return ErrInvalidUpload
	}
	return nil
}

func canonicalUploadName(name string) string {
	return strings.TrimSuffix(name, path.Ext(name)) + ".mp4"
}

func uploadFactHash(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
