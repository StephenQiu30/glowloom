package app

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	bibleapp "github.com/StephenQiu30/lanverse/backend/internal/bible/application"
	bibledomain "github.com/StephenQiu30/lanverse/backend/internal/bible/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/lanverse/backend/internal/media/domain"
	pgworkspace "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
)

func bibleCopyMediaBinding(b bibleapp.ProjectCopyBinding) mediaapp.ProjectCopyBinding {
	return mediaapp.ProjectCopyBinding{JobID: b.JobID, OrgID: b.OrgID, SourceProjectID: b.SourceProjectID, TargetProjectID: b.TargetProjectID}
}

func bibleCopyMediaFact(f bibledomain.MediaFact) mediaapp.ReferenceFact {
	r := mediaapp.ReferenceFact{AssetID: f.AssetID, Revision: f.Revision, Kind: mediadomain.Kind(f.Kind), SHA256: f.SHA256, ByteSize: f.ByteSize}
	if f.Kind == string(mediadomain.KindImage) {
		r.RenditionID, r.RenditionSHA256 = &f.RenditionID, &f.RenditionSHA256
	}
	return r
}

func mediaCopyBibleFact(f mediaapp.ReferenceFact) bibledomain.MediaFact {
	r := bibledomain.MediaFact{AssetID: f.AssetID, Revision: f.Revision, Kind: string(f.Kind), SHA256: f.SHA256, ByteSize: f.ByteSize}
	if f.RenditionID != nil {
		r.RenditionID = *f.RenditionID
	}
	if f.RenditionSHA256 != nil {
		r.RenditionSHA256 = *f.RenditionSHA256
	}
	return r
}

type projectCopyBibleMedia struct {
	query    *mediaapp.ProjectCopyReferenceQuery
	snapshot mediaapp.ProjectCopySnapshot
}

func (m projectCopyBibleMedia) FreezeReferences(ctx context.Context, actor identityapp.Principal, b bibleapp.ProjectCopyBinding, facts []bibledomain.MediaFact, assets map[uuid.UUID]uuid.UUID) ([]bibleapp.ReferenceMapping, error) {
	input := make([]mediaapp.ReferenceFact, len(facts))
	for i, f := range facts {
		if err := f.Validate(f.Kind); err != nil {
			return nil, err
		}
		input[i] = bibleCopyMediaFact(f)
	}
	mapped, err := m.query.FreezeReferences(ctx, actor, bibleCopyMediaBinding(b), m.snapshot, input)
	if err != nil {
		return nil, errors.Join(bibleapp.ErrUnavailable, err)
	}
	result := make([]bibleapp.ReferenceMapping, len(mapped))
	for i, ref := range mapped {
		if assets[ref.Source.AssetID] != ref.Target.AssetID {
			return nil, bibleapp.ErrUnavailable
		}
		result[i] = bibleapp.ReferenceMapping{Source: mediaCopyBibleFact(ref.Source), Target: mediaCopyBibleFact(ref.Target)}
	}
	return result, nil
}

// Each SQL read uses the exact current workspace worker, then releases its
// transaction before the reference query performs bounded private byte I/O.
type projectCopyBibleMediaReader struct {
	database  *gorm.DB
	authority workspaceapp.ProjectCopyAuthority
}

func (r projectCopyBibleMediaReader) read(ctx context.Context, actor identityapp.Principal, b mediaapp.ProjectCopyBinding, s mediaapp.ProjectCopySnapshot, f mediaapp.ReferenceFact, transferred bool) (mediaapp.ProjectCopyReferenceFiles, error) {
	var result mediaapp.ProjectCopyReferenceFiles
	err := r.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		binding := workspaceapp.ProjectCopyBinding{JobID: b.JobID, OrgID: b.OrgID, SourceProjectID: b.SourceProjectID, TargetProjectID: b.TargetProjectID}
		if err := pgworkspace.NewProjectCopyAccessStore(tx, r.authority).Authorize(ctx, actor, binding, true); err != nil {
			return err
		}
		var err error
		owner := pgmedia.NewProjectCopyStore(tx)
		if transferred {
			result, err = owner.ReadTransferredCopyReference(ctx, actor, b, s, f)
		} else {
			result, err = owner.ReadCopyReference(ctx, actor, b, s, f)
		}
		return err
	})
	return result, err
}
func (r projectCopyBibleMediaReader) ReadCopyReference(ctx context.Context, actor identityapp.Principal, b mediaapp.ProjectCopyBinding, s mediaapp.ProjectCopySnapshot, f mediaapp.ReferenceFact) (mediaapp.ProjectCopyReferenceFiles, error) {
	return r.read(ctx, actor, b, s, f, false)
}
func (r projectCopyBibleMediaReader) ReadTransferredCopyReference(ctx context.Context, actor identityapp.Principal, b mediaapp.ProjectCopyBinding, s mediaapp.ProjectCopySnapshot, f mediaapp.ReferenceFact) (mediaapp.ProjectCopyReferenceFiles, error) {
	return r.read(ctx, actor, b, s, f, true)
}

type projectCopyBibleTransferredMedia struct {
	query    *mediaapp.ProjectCopyReferenceQuery
	snapshot mediaapp.ProjectCopySnapshot
}

func (m projectCopyBibleTransferredMedia) VerifyTransferred(ctx context.Context, actor identityapp.Principal, b bibleapp.ProjectCopyBinding, mappings []bibleapp.ReferenceMapping) error {
	input := make([]mediaapp.ProjectCopyReferenceMapping, len(mappings))
	for i, mapping := range mappings {
		input[i] = mediaapp.ProjectCopyReferenceMapping{Source: bibleCopyMediaFact(mapping.Source), Target: bibleCopyMediaFact(mapping.Target)}
	}
	return m.query.VerifyTransferred(ctx, actor, bibleCopyMediaBinding(b), m.snapshot, input)
}
