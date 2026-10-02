package application

import (
	"context"
	"errors"
	"math"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

// ProjectCopyReferenceMapping preserves source history and names the media
// owner's actual target identities. Rendition digests remain expected facts
// until VerifyTransferred verifies the independent private objects.
type ProjectCopyReferenceMapping struct {
	Source ReferenceFact
	Target ReferenceFact
}

// ProjectCopyReferenceFiles is private manifest-bound SQL evidence.
type ProjectCopyReferenceFiles struct {
	Source            LibraryMediaFile
	Target            LibraryMediaFile
	RetainedHistory   *RetainedHistoryProof
	TargetRenditionID *uuid.UUID
}

// ProjectCopyReferenceReader is bound by the owning workspace admission or
// worker fence. It returns actual manifest identities, never caller object keys.
type ProjectCopyReferenceReader interface {
	ReadCopyReference(context.Context, identityapp.Principal, ProjectCopyBinding, ProjectCopySnapshot, ReferenceFact) (ProjectCopyReferenceFiles, error)
	ReadTransferredCopyReference(context.Context, identityapp.Principal, ProjectCopyBinding, ProjectCopySnapshot, ReferenceFact) (ProjectCopyReferenceFiles, error)
}

// ProjectCopyReferenceQuery separates SQL-only admission from actual byte proof.
type ProjectCopyReferenceQuery struct {
	repo    ProjectCopyReferenceReader
	objects DocumentObjectGetter
}

// NewProjectCopyReferenceQuery injects the authority-bound SQL reader and byte I/O.
// SQL-only FreezeReferences does not require or access the object getter.
func NewProjectCopyReferenceQuery(repo ProjectCopyReferenceReader, objects DocumentObjectGetter) *ProjectCopyReferenceQuery {
	return &ProjectCopyReferenceQuery{repo: repo, objects: objects}
}

func validReferenceFact(f ReferenceFact) bool {
	if f.AssetID == uuid.Nil || f.Revision < 1 || f.Revision > math.MaxInt32 || f.ByteSize < 1 || !copySHA(&f.SHA256) || f.Kind != domain.KindImage && f.Kind != domain.KindAudio {
		return false
	}
	if f.Kind == domain.KindAudio {
		return f.RenditionID == nil && f.RenditionSHA256 == nil
	}
	return f.RenditionID != nil && *f.RenditionID != uuid.Nil && f.RenditionSHA256 != nil && copySHA(f.RenditionSHA256)
}

// ValidateCopyReferenceFact closes trusted historical binding evidence before
// SQL-only admission. It does not prove that any private object was read.
func ValidateCopyReferenceFact(f ReferenceFact) error {
	if !validReferenceFact(f) {
		return ErrProjectCopyMediaUnavailable
	}
	return nil
}

func referenceMapping(binding ProjectCopyBinding, fact ReferenceFact, files ProjectCopyReferenceFiles) (ProjectCopyReferenceMapping, error) {
	source, target := files.Source.Asset, files.Target.Asset
	if !validReferenceFact(fact) || source.Validate() != nil || source.Personal != nil || source.ID != fact.AssetID || source.ProjectID != binding.SourceProjectID || source.Kind != fact.Kind || source.Status != domain.StatusReady || source.ModerationStatus != domain.ModerationPassed || source.ContainsRealPerson || source.ConsentRecordID != nil || source.SHA256 == nil || *source.SHA256 != fact.SHA256 || source.ByteSize != fact.ByteSize || source.ByteSize > libraryOriginalLimit(source.Kind) || !validRetainedHistory(source, files.RetainedHistory) {
		return ProjectCopyReferenceMapping{}, ErrProjectCopyMediaUnavailable
	}
	// Only an exact owning retained manifest can relate an older immutable
	// content fact to the later soft-delete revision. Source history is unchanged.
	if source.Revision != fact.Revision && (!source.IsDelete || source.Revision <= fact.Revision || files.RetainedHistory == nil) {
		return ProjectCopyReferenceMapping{}, ErrProjectCopyMediaUnavailable
	}
	if !target.CanReference() || target.ContainsRealPerson || target.ConsentRecordID != nil || target.ProjectID != binding.TargetProjectID || target.ID == source.ID || target.Revision != 1 || target.Kind != fact.Kind || target.ByteSize != fact.ByteSize || target.SHA256 == nil || *target.SHA256 != fact.SHA256 {
		return ProjectCopyReferenceMapping{}, ErrProjectCopyMediaUnavailable
	}
	want := ReferenceFact{AssetID: target.ID, Revision: 1, Kind: target.Kind, SHA256: fact.SHA256, ByteSize: fact.ByteSize}
	if fact.Kind == domain.KindImage {
		var found bool
		for _, r := range files.Source.Renditions {
			if r.ID == *fact.RenditionID && r.MediaAssetID == source.ID && !r.IsDelete && r.Validate() == nil && (r.Kind == domain.RenditionThumb256 || r.Kind == domain.RenditionThumb640) {
				found = true
			}
		}
		if !found || files.TargetRenditionID == nil || *files.TargetRenditionID == uuid.Nil {
			return ProjectCopyReferenceMapping{}, ErrProjectCopyMediaUnavailable
		}
		id, digest := *files.TargetRenditionID, *fact.RenditionSHA256
		want.RenditionID, want.RenditionSHA256 = &id, &digest
	}
	return ProjectCopyReferenceMapping{Source: fact, Target: want}, nil
}

// FreezeReferences performs no remote I/O. Expected image digest verification
// is deferred until the existing media object transfer has actually completed.
func (q *ProjectCopyReferenceQuery) FreezeReferences(ctx context.Context, actor identityapp.Principal, binding ProjectCopyBinding, snapshot ProjectCopySnapshot, facts []ReferenceFact) ([]ProjectCopyReferenceMapping, error) {
	if q == nil || q.repo == nil || binding.Validate() != nil || binding.OrgID != actor.OrgID || len(facts) > 4096 {
		return nil, ErrProjectCopyMediaUnavailable
	}
	result := make([]ProjectCopyReferenceMapping, 0, len(facts))
	for _, fact := range facts {
		if !validReferenceFact(fact) {
			return nil, ErrProjectCopyMediaUnavailable
		}
		files, err := q.repo.ReadCopyReference(ctx, actor, binding, snapshot, fact)
		if err != nil {
			return nil, err
		}
		mapping, err := referenceMapping(binding, fact, files)
		if err != nil {
			return nil, err
		}
		result = append(result, mapping)
	}
	return result, nil
}

func sameReferenceFact(a, b ReferenceFact) bool {
	if a.AssetID != b.AssetID || a.Revision != b.Revision || a.Kind != b.Kind || a.SHA256 != b.SHA256 || a.ByteSize != b.ByteSize || (a.RenditionID == nil) != (b.RenditionID == nil) || (a.RenditionSHA256 == nil) != (b.RenditionSHA256 == nil) {
		return false
	}
	return (a.RenditionID == nil || *a.RenditionID == *b.RenditionID) && (a.RenditionSHA256 == nil || *a.RenditionSHA256 == *b.RenditionSHA256)
}

func verifyReferenceFile(ctx context.Context, objects DocumentObjectGetter, file LibraryMediaFile, fact ReferenceFact) error {
	q := &ReferenceFactQuery{objects: objects}
	a := file.Asset
	if _, _, err := q.verify(ctx, a.ObjectKey, &fact.ByteSize, &fact.SHA256, libraryOriginalLimit(fact.Kind)); err != nil {
		return errors.Join(ErrObjectMismatch, err)
	}
	if fact.Kind == domain.KindAudio {
		return nil
	}
	for _, r := range file.Renditions {
		if r.ID != *fact.RenditionID {
			continue
		}
		digest, err := q.verifyImageRendition(ctx, r)
		if err != nil || digest != *fact.RenditionSHA256 {
			return errors.Join(ErrObjectMismatch, err)
		}
		return nil
	}
	return ErrObjectMismatch
}

// VerifyTransferred requires registered owning receipts, exact SQL facts, and
// actual source/target bytes. The caller runs it before publishing its project.
func (q *ProjectCopyReferenceQuery) VerifyTransferred(ctx context.Context, actor identityapp.Principal, binding ProjectCopyBinding, snapshot ProjectCopySnapshot, mappings []ProjectCopyReferenceMapping) error {
	if q == nil || q.repo == nil || q.objects == nil || binding.Validate() != nil || binding.OrgID != actor.OrgID || len(mappings) > 4096 {
		return ErrProjectCopyMediaUnavailable
	}
	for _, mapping := range mappings {
		if !validReferenceFact(mapping.Source) || !validReferenceFact(mapping.Target) {
			return ErrObjectMismatch
		}
		files, err := q.repo.ReadTransferredCopyReference(ctx, actor, binding, snapshot, mapping.Source)
		if err != nil {
			return err
		}
		expected, err := referenceMapping(binding, mapping.Source, files)
		if err != nil || !sameReferenceFact(mapping.Target, expected.Target) {
			return errors.Join(ErrObjectMismatch, err)
		}
		if err := verifyReferenceFile(ctx, q.objects, files.Source, mapping.Source); err != nil {
			return err
		}
		if err := verifyReferenceFile(ctx, q.objects, files.Target, mapping.Target); err != nil {
			return err
		}
	}
	return nil
}
