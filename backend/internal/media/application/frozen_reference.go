package application

import (
	"context"
	"errors"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

// FrozenReferenceFactReader reads the owning evidence for an existing immutable
// binding. Catalog visibility is distinct from original eligibility. The caller
// must keep its owning transaction open throughout the bounded object reads.
type FrozenReferenceFactReader interface {
	ReadFrozenReferenceFactSource(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID, domain.Kind) (LibraryMediaFile, error)
}

// VerifyFrozenReference checks an existing proof against exact current media
// facts and actual private bytes; it cannot form or update a new binding.
func (q *ReferenceFactQuery) VerifyFrozenReference(ctx context.Context, actor identityapp.Principal, project uuid.UUID, fact ReferenceFact) error {
	if q == nil || q.reader == nil || q.objects == nil {
		return ErrUnavailable
	}
	reader, ok := q.reader.(FrozenReferenceFactReader)
	if !ok {
		return ErrUnavailable
	}
	if project == uuid.Nil || fact.AssetID == uuid.Nil || fact.Kind != domain.KindImage && fact.Kind != domain.KindAudio || fact.Revision < 1 || fact.ByteSize < 1 || !copySHA(&fact.SHA256) {
		return ErrUnavailable
	}
	if fact.Kind == domain.KindImage {
		if fact.RenditionID == nil || *fact.RenditionID == uuid.Nil || fact.RenditionSHA256 == nil || !copySHA(fact.RenditionSHA256) {
			return ErrUnavailable
		}
	} else if fact.RenditionID != nil || fact.RenditionSHA256 != nil {
		return ErrUnavailable
	}
	file, err := reader.ReadFrozenReferenceFactSource(ctx, actor, project, fact.AssetID, fact.Kind)
	if err != nil {
		return err
	}
	a := file.Asset
	if a.ID != fact.AssetID || a.ProjectID != project || a.Personal != nil || a.Kind != fact.Kind || !a.CanReference() || a.ContainsRealPerson || a.ConsentRecordID != nil || a.Revision != fact.Revision || a.ByteSize != fact.ByteSize || a.ByteSize > libraryOriginalLimit(a.Kind) || a.SHA256 == nil || *a.SHA256 != fact.SHA256 {
		return ErrUnavailable
	}
	if _, _, err := q.verify(ctx, a.ObjectKey, &a.ByteSize, a.SHA256, libraryOriginalLimit(a.Kind)); err != nil {
		return errors.Join(ErrUnavailable, err)
	}
	if fact.Kind == domain.KindAudio {
		return nil
	}
	for _, r := range file.Renditions {
		if r.ID != *fact.RenditionID {
			continue
		}
		if r.Validate() != nil || r.MediaAssetID != a.ID || r.IsDelete || r.Kind != domain.RenditionThumb256 && r.Kind != domain.RenditionThumb640 {
			return ErrUnavailable
		}
		digest, err := q.verifyImageRendition(ctx, r)
		if err != nil {
			return errors.Join(ErrUnavailable, err)
		}
		if digest != *fact.RenditionSHA256 {
			return ErrUnavailable
		}
		return nil
	}
	return ErrUnavailable
}
