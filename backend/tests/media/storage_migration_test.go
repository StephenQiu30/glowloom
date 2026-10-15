package media_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

type legacyVerificationObjects struct {
	ref  domain.ObjectRef
	data []byte
	info mediaapp.ObjectInfo
	err  error
}

func (s legacyVerificationObjects) Get(_ context.Context, ref domain.ObjectRef) (io.ReadCloser, error) {
	if ref != s.ref {
		return nil, errors.New("wrong profile")
	}
	return io.NopCloser(bytes.NewReader(s.data)), s.err
}
func (s legacyVerificationObjects) Stat(_ context.Context, ref domain.ObjectRef) (mediaapp.ObjectInfo, error) {
	if ref != s.ref {
		return mediaapp.ObjectInfo{}, errors.New("wrong profile")
	}
	return s.info, s.err
}
func TestStorageLegacyVerifierChecksActualBytesBeforeBackfill(t *testing.T) {
	data := []byte("legacy immutable bytes")
	a := sourceDocumentAsset(data)
	ref := domain.ObjectRef{ProfileID: "minio-main", Bucket: "storage-test", Key: a.ObjectKey}
	candidate := mediaapp.LegacyObjectCandidate{Ref: ref, ByteSize: &a.ByteSize, SHA256: a.SHA256, MIMEType: &a.MimeType}
	objects := legacyVerificationObjects{ref: ref, data: data, info: mediaapp.ObjectInfo{Size: a.ByteSize, ContentType: a.MimeType, SHA256: *a.SHA256}}
	got, err := mediaapp.NewLegacyByteVerifier(objects).VerifyLegacyObject(t.Context(), candidate)
	if err != nil || got.Size != a.ByteSize || got.SHA256 != *a.SHA256 {
		t.Fatal("original bytes not verified", err)
	}
	objects.data = bytes.Repeat([]byte("x"), len(data))
	if _, err := mediaapp.NewLegacyByteVerifier(objects).VerifyLegacyObject(t.Context(), candidate); !errors.Is(err, mediaapp.ErrObjectMismatch) {
		t.Fatal("forged sha metadata became migration proof", err)
	}
	objects.err = errors.New("provider forbidden")
	if _, err := mediaapp.NewLegacyByteVerifier(objects).VerifyLegacyObject(t.Context(), candidate); !errors.Is(err, objects.err) {
		t.Fatal("provider failure became absence proof", err)
	}
}
