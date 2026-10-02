package application

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

const (
	// MaxUploadImageBytes is the local image limit in bytes.
	MaxUploadImageBytes int64 = 20 << 20
	// MaxUploadVideoBytes is the local video limit in bytes.
	MaxUploadVideoBytes int64 = 500 << 20
	// MaxUploadAudioBytes is the local audio limit in bytes.
	MaxUploadAudioBytes int64 = 100 << 20
	// MaxUploadModelBytes is the self-contained GLB 2.0 model limit.
	MaxUploadModelBytes int64 = domain.MaxModelBytes
	// MaxUploadDocumentBytes bounds each unmodified script source file.
	MaxUploadDocumentBytes int64 = domain.MaxDocumentBytes
)

// SafeUploadFileName accepts a bounded display name, never a filesystem path.
func SafeUploadFileName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." || len(name) > 255 || !utf8.ValidString(name) || strings.ContainsAny(name, "/\\") {
		return "", ErrInvalidUpload
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return "", ErrInvalidUpload
		}
	}
	return name, nil
}

// ReadUpload bounds the actual content before creating a private temporary file.
// The caller must authorize the project first and must Close the returned file.
func ReadUpload(ctx context.Context, reader io.Reader, name string) (*Downloaded, error) {
	name, err := SafeUploadFileName(name)
	if err != nil || reader == nil {
		return nil, ErrInvalidUpload
	}
	var magic [512]byte
	n, err := io.ReadFull(reader, magic[:])
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	mimeType, extension, limit := uploadType(magic[:n])
	if strings.EqualFold(path.Ext(name), ".txt") && mimeType == "" {
		mimeType, extension, limit = domain.MIMEText, "txt", MaxUploadDocumentBytes
	}
	if strings.EqualFold(path.Ext(name), ".docx") && n >= 4 && bytes.Equal(magic[:4], []byte{'P', 'K', 3, 4}) {
		mimeType, extension, limit = domain.MIMEDOCX, "docx", MaxUploadDocumentBytes
	}
	jsonStart := bytes.TrimSpace(magic[:n])
	if strings.EqualFold(path.Ext(name), ".gltf") && n > 0 && (len(jsonStart) == 0 || jsonStart[0] == '{') {
		mimeType, extension, limit = "model/gltf+json", "gltf", MaxUploadModelBytes
	}
	extensionMatches := strings.EqualFold(path.Ext(name), "."+extension) || extension == "jpg" && strings.EqualFold(path.Ext(name), ".jpeg")
	if mimeType == "" || !extensionMatches {
		return nil, ErrUnsupportedUpload
	}
	file, err := os.CreateTemp("", "lanverse-upload-*-."+extension)
	if err != nil {
		return nil, err
	}
	downloaded := &Downloaded{File: file, MIMEType: mimeType}
	keep := false
	defer func() {
		if !keep {
			_ = downloaded.Close()
		}
	}()
	hash := sha256.New()
	input := io.MultiReader(bytes.NewReader(magic[:n]), reader)
	size, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(uploadContextReader{ctx: ctx, reader: input}, limit+1))
	if err != nil {
		return nil, err
	}
	if size > limit {
		return nil, ErrUploadTooLarge
	}
	if size == 0 {
		return nil, ErrUnsupportedUpload
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	downloaded.Size, downloaded.SHA256 = size, hex.EncodeToString(hash.Sum(nil))
	keep = true
	return downloaded, nil
}

func uploadType(magic []byte) (string, string, int64) {
	if len(magic) >= 12 && string(magic[:4]) == "glTF" {
		return "model/gltf-binary", "glb", MaxUploadModelBytes
	}
	mimeType := http.DetectContentType(magic)
	switch mimeType {
	case "video/webm":
		return mimeType, "webm", MaxUploadVideoBytes
	case "image/jpeg":
		return mimeType, "jpg", MaxUploadImageBytes
	case "image/png":
		return mimeType, "png", MaxUploadImageBytes
	case "image/webp":
		return mimeType, "webp", MaxUploadImageBytes
	case "image/gif":
		return mimeType, "gif", MaxUploadImageBytes
	case "audio/mpeg":
		return mimeType, "mp3", MaxUploadAudioBytes
	case "audio/wave", "audio/x-wav":
		return "audio/wave", "wav", MaxUploadAudioBytes
	}
	// ISO-BMFF's server-read major brand distinguishes M4A and QuickTime;
	// the probe must also verify the corresponding real audio/video stream.
	if len(magic) >= 12 && string(magic[4:8]) == "ftyp" {
		switch string(magic[8:12]) {
		case "M4A ", "M4B ":
			return "audio/mp4", "m4a", MaxUploadAudioBytes
		case "qt  ":
			return "video/quicktime", "mov", MaxUploadVideoBytes
		case "isom", "iso2", "mp41", "mp42", "avc1", "M4V ":
			return "video/mp4", "mp4", MaxUploadVideoBytes
		}
	}
	return "", "", 0
}

type uploadContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r uploadContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
