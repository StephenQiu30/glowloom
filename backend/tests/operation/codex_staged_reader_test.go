package operation_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"image/png"
	"os"
	"strings"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/media/adapter/staged"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/application"
)

func TestM1StagedIngestReaderRejectsUnverifiedImage(t *testing.T) {
	for _, behavior := range []string{"complete", "false_mime", "false_hash", "truncated", "large_dimensions", "wrong_receipt", "cancelled"} {
		t.Run(behavior, func(t *testing.T) {
			var encoded bytes.Buffer
			width := 2
			if behavior == "large_dimensions" {
				width = 8193
			}
			if err := png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, width, 2))); err != nil {
				t.Fatal(err)
			}
			data := encoded.Bytes()
			if behavior == "truncated" {
				data = data[:len(data)-16]
			}
			digest := sha256.Sum256(data)
			receipt := codexTestReceipt(codexTestInput().Identity)
			receipt.Outputs[0] = application.ProviderReceiptOutput{Sequence: 1, SizeBytes: int64(len(data)), MIMEType: "image/png", SHA256: hex.EncodeToString(digest[:])}
			if behavior == "false_mime" {
				receipt.Outputs[0].MIMEType = "image/jpeg"
			}
			if behavior == "false_hash" {
				receipt.Outputs[0].SHA256 = strings.Repeat("a", 64)
			}
			source := stagedReaderFixture{output: receipt.Outputs[0], data: data}
			if behavior == "wrong_receipt" {
				source.output.SHA256 = strings.Repeat("b", 64)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if behavior == "cancelled" {
				cancel()
			}
			got, err := staged.NewReader(source).Download(ctx, receipt)
			if behavior != "complete" {
				if err == nil || got != nil {
					t.Fatal("unverified image created a media temporary file")
				}
				return
			}
			if err != nil || got == nil || got.File == nil {
				t.Fatalf("valid private image rejected: %v", err)
			}
			name := got.File.Name()
			info, err := got.File.Stat()
			if err != nil || info.Mode().Perm() != 0600 {
				t.Fatal("media temporary file is not private")
			}
			if err := got.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(name); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("media temporary file outlived its owner")
			}
		})
	}
}

func TestM1StagedIngestReaderPreservesReceiptRejection(t *testing.T) {
	r := staged.NewReader(stagedReaderFixture{verifyErr: application.ErrProviderCallConflict})
	err := r.ValidateReceipt(t.Context(), codexTestReceipt(codexTestInput().Identity))
	if !errors.Is(err, mediaapp.ErrInvalidIngest) || !errors.Is(err, application.ErrProviderCallConflict) {
		t.Fatal("receipt rejection lost its media classification or error chain")
	}
}

type stagedReaderFixture struct {
	output    application.ProviderReceiptOutput
	data      []byte
	verifyErr error
}

func (f stagedReaderFixture) VerifyReceipt(context.Context, application.ProviderReceipt) error {
	return f.verifyErr
}
func (f stagedReaderFixture) ReadImage(context.Context, application.ProviderReceipt) (application.ProviderReceiptOutput, []byte, error) {
	return f.output, f.data, nil
}
