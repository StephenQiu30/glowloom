package application

import (
	"bytes"
	"context"
	"errors"
	"image"
	_ "image/jpeg" // Register the actual image decoders used to verify legacy thumbnails.
	_ "image/png"  // Register the canonical thumbnail decoder.
	"io"

	"github.com/google/uuid"
	_ "golang.org/x/image/webp" // Register the existing supported WebP thumbnail decoder.

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

// ReferenceFact is current, actually verified owning evidence for new entity
// bindings. RenditionSHA256 is its own bytes, never the original asset's SHA.
type ReferenceFact struct {
	AssetID         uuid.UUID
	Revision        int64
	Kind            domain.Kind
	SHA256          string
	ByteSize        int64
	RenditionID     *uuid.UUID
	RenditionSHA256 *string
}

func (q *ReferenceFactQuery) verifyImageRendition(ctx context.Context, r domain.Rendition) (string, error) {
	const limit = int64(8 << 20)
	if r.ByteSize != nil && (*r.ByteSize < 1 || *r.ByteSize > limit) {
		return "", ErrUnavailable
	}
	reader, err := q.objects.Get(ctx, r.ObjectKey)
	if err != nil || reader == nil {
		return "", errors.Join(ErrUnavailable, err)
	}
	var content bytes.Buffer
	size, sha, readErr := copyObjectBytes(ctx, io.LimitReader(reader, limit+1), &content, r.ByteSize, nil)
	closeErr := reader.Close()
	if readErr != nil || closeErr != nil || size > limit {
		return "", errors.Join(ErrUnavailable, readErr, closeErr)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(content.Bytes()))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 1024 || cfg.Height > 1024 || r.Width != nil && int(*r.Width) != cfg.Width || r.Height != nil && int(*r.Height) != cfg.Height {
		return "", ErrUnavailable
	}
	decoded, _, err := image.Decode(bytes.NewReader(content.Bytes()))
	if err != nil || decoded.Bounds().Dx() != cfg.Width || decoded.Bounds().Dy() != cfg.Height {
		return "", ErrUnavailable
	}
	return sha, nil
}

// ReferenceFactReader holds current project/library/original eligibility locks.
// Its owning transaction must remain open during the bounded object reads.
type ReferenceFactReader interface {
	ReadReferenceFactSource(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID, domain.Kind) (LibraryMediaFile, error)
}

// ReferenceFactQuery verifies independent original and image rendition bytes.
type ReferenceFactQuery struct {
	reader  ReferenceFactReader
	objects DocumentObjectGetter
}

// NewReferenceFactQuery injects a transaction-bound owner and private object I/O.
func NewReferenceFactQuery(reader ReferenceFactReader, objects DocumentObjectGetter) *ReferenceFactQuery {
	return &ReferenceFactQuery{reader: reader, objects: objects}
}

func (q *ReferenceFactQuery) verify(ctx context.Context, key string, size *int64, sha *string, limit int64) (int64, string, error) {
	reader, err := q.objects.Get(ctx, key)
	if err != nil || reader == nil {
		return 0, "", errors.Join(ErrUnavailable, err)
	}
	bound := size
	if size != nil && (*size < 1 || *size > limit) {
		_ = reader.Close()
		return 0, "", ErrUnavailable
	}
	if bound == nil {
		bound = &limit
	}
	// When size is unknown, LimitReader establishes a strict decoded byte bound
	// without pretending that the limit is a persisted rendition size.
	input := io.Reader(reader)
	if size == nil {
		input = io.LimitReader(reader, limit+1)
		bound = nil
	}
	n, digest, verifyErr := copyObjectBytes(ctx, input, io.Discard, bound, sha)
	if n > limit {
		verifyErr = ErrObjectMismatch
	}
	return n, digest, errors.Join(verifyErr, reader.Close())
}

// Reference supports actual image/audio binding facts and rejects hidden library
// items. Callers freeze the returned safe proof through their own owning write.
func (q *ReferenceFactQuery) Reference(ctx context.Context, actor identityapp.Principal, project, id uuid.UUID, kind domain.Kind) (ReferenceFact, error) {
	if q == nil || q.reader == nil || q.objects == nil {
		return ReferenceFact{}, ErrUnavailable
	}
	if project == uuid.Nil || id == uuid.Nil || kind != domain.KindImage && kind != domain.KindAudio {
		return ReferenceFact{}, ErrInvalidQuery
	}
	file, err := q.reader.ReadReferenceFactSource(ctx, actor, project, id, kind)
	if err != nil {
		return ReferenceFact{}, err
	}
	a := file.Asset
	if a.ID != id || a.ProjectID != project || a.Personal != nil || a.Kind != kind || !a.CanReference() || a.ContainsRealPerson || a.ConsentRecordID != nil || a.SHA256 == nil || !copySHA(a.SHA256) || a.ByteSize < 1 || a.ByteSize > libraryOriginalLimit(a.Kind) {
		return ReferenceFact{}, ErrUnavailable
	}
	if _, _, err := q.verify(ctx, a.ObjectKey, &a.ByteSize, a.SHA256, libraryOriginalLimit(a.Kind)); err != nil {
		return ReferenceFact{}, errors.Join(ErrUnavailable, err)
	}
	fact := ReferenceFact{AssetID: a.ID, Revision: a.Revision, Kind: a.Kind, SHA256: *a.SHA256, ByteSize: a.ByteSize}
	if kind == domain.KindAudio {
		return fact, nil
	}
	var selected *domain.Rendition
	for i := range file.Renditions {
		r := &file.Renditions[i]
		if r.Kind == domain.RenditionThumb256 {
			selected = r
			break
		}
		if r.Kind == domain.RenditionThumb640 {
			selected = r
		}
	}
	if selected == nil || selected.Validate() != nil || selected.MediaAssetID != a.ID || selected.IsDelete {
		return ReferenceFact{}, ErrUnavailable
	}
	digest, err := q.verifyImageRendition(ctx, *selected)
	if err != nil {
		return ReferenceFact{}, errors.Join(ErrUnavailable, err)
	}
	id = selected.ID
	fact.RenditionID, fact.RenditionSHA256 = &id, &digest
	return fact, nil
}
