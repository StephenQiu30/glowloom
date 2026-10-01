package codex

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"image"
	_ "image/jpeg" // Register only the image formats accepted by this protocol.
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"strings"

	_ "golang.org/x/image/webp" // Register the accepted WebP image decoder.

	"github.com/StephenQiu30/lanverse/backend/internal/operation/application"
)

const maxImageBytes = 32 * 1024 * 1024

func readImage(dir, path string) (application.ProviderReceiptOutput, []byte, error) {
	if !filepath.IsAbs(path) || len(path) > 4096 || strings.TrimSpace(path) != path {
		return application.ProviderReceiptOutput{}, nil, errProtocol
	}
	relative, err := filepath.Rel(dir, path)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return application.ProviderReceiptOutput{}, nil, errProtocol
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return application.ProviderReceiptOutput{}, nil, errProtocol
	}
	defer func() { _ = root.Close() }()
	// os.Root rejects symlinks that escape the task directory and prevents the
	// check/open race of EvalSymlinks followed by an unrestricted os.Open.
	file, err := root.Open(relative)
	if err != nil {
		return application.ProviderReceiptOutput{}, nil, errProtocol
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > maxImageBytes {
		return application.ProviderReceiptOutput{}, nil, errProtocol
	}
	data, err := io.ReadAll(io.LimitReader(file, maxImageBytes+1))
	if err != nil || len(data) > maxImageBytes || int64(len(data)) != info.Size() {
		return application.ProviderReceiptOutput{}, nil, errProtocol
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width < 1 || config.Height < 1 || config.Width > 8192 || config.Height > 8192 || int64(config.Width)*int64(config.Height) > 40_000_000 {
		return application.ProviderReceiptOutput{}, nil, errProtocol
	}
	mime := map[string]string{"png": "image/png", "jpeg": "image/jpeg", "webp": "image/webp"}[format]
	if mime == "" {
		return application.ProviderReceiptOutput{}, nil, errProtocol
	}
	// Decode the complete file after bounding dimensions. A plausible header is
	// insufficient evidence that a usable image was generated.
	_, decodedFormat, err := image.Decode(bytes.NewReader(data))
	if err != nil || decodedFormat != format {
		return application.ProviderReceiptOutput{}, nil, errProtocol
	}
	digest := sha256.Sum256(data)
	return application.ProviderReceiptOutput{Sequence: 1, SizeBytes: int64(len(data)), MIMEType: mime, SHA256: hex.EncodeToString(digest[:])}, data, nil
}
