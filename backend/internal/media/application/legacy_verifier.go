package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// LegacyVerificationObjects supplies actual provider metadata and exact bytes.
type LegacyVerificationObjects interface {
	ObjectReader
	Stat(context.Context, domain.ObjectRef) (ObjectInfo, error)
}

// LegacyByteVerifier proves bounded byte facts rather than trusting SHA metadata.
type LegacyByteVerifier struct{ objects LegacyVerificationObjects }

// NewLegacyByteVerifier injects the explicit physical object reader.
func NewLegacyByteVerifier(objects LegacyVerificationObjects) *LegacyByteVerifier {
	return &LegacyByteVerifier{objects: objects}
}

// VerifyLegacyObject computes the original's digest before any location binding.
func (s *LegacyByteVerifier) VerifyLegacyObject(ctx context.Context, c LegacyObjectCandidate) (ObjectInfo, error) {
	if s == nil || s.objects == nil {
		return ObjectInfo{}, ErrUnavailable
	}
	if c.Ref.Validate() != nil {
		return ObjectInfo{}, ErrObjectMismatch
	}
	info, err := s.objects.Stat(ctx, c.Ref)
	if err != nil {
		return ObjectInfo{}, err
	}
	if info.Size < 0 || info.Size > 2<<30 || info.ContentType == "" || c.ByteSize != nil && *c.ByteSize != info.Size || c.MIMEType != nil && *c.MIMEType != info.ContentType {
		return ObjectInfo{}, ErrObjectMismatch
	}
	reader, err := s.objects.Get(ctx, c.Ref)
	if err != nil {
		if reader != nil {
			_ = reader.Close()
		}
		return ObjectInfo{}, err
	}
	if reader == nil {
		return ObjectInfo{}, ErrUnavailable
	}
	digest := sha256.New()
	size, readErr := io.Copy(digest, io.LimitReader(uploadContextReader{ctx: ctx, reader: reader}, info.Size+1))
	closeErr := reader.Close()
	if err := errors.Join(readErr, closeErr, ctx.Err()); err != nil {
		return ObjectInfo{}, err
	}
	actual := hex.EncodeToString(digest.Sum(nil))
	if size != info.Size || c.SHA256 != nil && *c.SHA256 != actual || info.SHA256 != "" && info.SHA256 != actual {
		return ObjectInfo{}, ErrObjectMismatch
	}
	info.SHA256 = actual
	return info, nil
}
