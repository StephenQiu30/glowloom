// Package staged reads bounded private provider artifacts into the media boundary.
package staged

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // Register only the image formats accepted by the receipt.
	_ "image/png"
	"io"
	"os"

	_ "golang.org/x/image/webp" // Register the accepted WebP image decoder.

	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	operationapp "github.com/StephenQiu30/lanverse/backend/internal/operation/application"
)

// ReceiptSource verifies durable identities and reads server-derived object keys.
type ReceiptSource interface {
	VerifyReceipt(context.Context, operationapp.ProviderReceipt) error
	ReadImage(context.Context, operationapp.ProviderReceipt) (operationapp.ProviderReceiptOutput, []byte, error)
}

// Reader adapts private receipt bytes to the existing bounded media pipeline.
type Reader struct{ source ReceiptSource }

// NewReader explicitly injects the durable provider receipt boundary.
func NewReader(source ReceiptSource) *Reader { return &Reader{source: source} }

// ValidateReceipt binds replay to the durable call without requiring staging bytes.
func (r *Reader) ValidateReceipt(ctx context.Context, receipt operationapp.ProviderReceipt) error {
	if r == nil || r.source == nil || receipt.Validate() != nil {
		return application.ErrInvalidIngest
	}
	return classifyReceiptError(r.source.VerifyReceipt(ctx, receipt))
}

// Download validates the complete image and owns the private temporary file.
func (r *Reader) Download(ctx context.Context, receipt operationapp.ProviderReceipt) (*application.Downloaded, error) {
	if r == nil || r.source == nil || receipt.Validate() != nil {
		return nil, application.ErrInvalidIngest
	}
	output, data, err := r.source.ReadImage(ctx, receipt)
	if err != nil {
		return nil, classifyReceiptError(err)
	}
	digest := sha256.Sum256(data)
	if output != receipt.Outputs[0] || int64(len(data)) != output.SizeBytes || hex.EncodeToString(digest[:]) != output.SHA256 || !validImage(data, output.MIMEType) {
		return nil, application.ErrInvalidIngest
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	file, err := os.CreateTemp("", "lanverse-staged-image-*.image")
	if err != nil {
		return nil, fmt.Errorf("create staged image file: %w", err)
	}
	downloaded := &application.Downloaded{File: file, Size: output.SizeBytes, MIMEType: output.MIMEType, SHA256: output.SHA256}
	if _, err := file.Write(data); err != nil {
		_ = downloaded.Close()
		return nil, fmt.Errorf("write staged image file: %w", err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		_ = downloaded.Close()
		return nil, fmt.Errorf("rewind staged image file: %w", err)
	}
	return downloaded, nil
}

func classifyReceiptError(err error) error {
	if errors.Is(err, operationapp.ErrInvalidProviderCall) || errors.Is(err, operationapp.ErrProviderCallConflict) {
		return errors.Join(application.ErrInvalidIngest, err)
	}
	return err
}

func validImage(data []byte, mime string) bool {
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width < 1 || config.Height < 1 || config.Width > 8192 || config.Height > 8192 || int64(config.Width)*int64(config.Height) > 40_000_000 {
		return false
	}
	if map[string]string{"png": "image/png", "jpeg": "image/jpeg", "webp": "image/webp"}[format] != mime {
		return false
	}
	_, decodedFormat, err := image.Decode(bytes.NewReader(data))
	return err == nil && decodedFormat == format
}
