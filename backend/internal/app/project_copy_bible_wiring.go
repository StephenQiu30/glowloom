package app

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pgbible "github.com/StephenQiu30/lanverse/backend/internal/bible/adapter/postgres"
	bibleapp "github.com/StephenQiu30/lanverse/backend/internal/bible/application"
	bibledomain "github.com/StephenQiu30/lanverse/backend/internal/bible/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstorage"
	pgworkspace "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	workspacedomain "github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

type projectCopyBibleAccess struct {
	store *pgworkspace.ProjectCopyAccessStore
}

func (a projectCopyBibleAccess) Authorize(ctx context.Context, actor identityapp.Principal, b bibleapp.ProjectCopyBinding, target bool) error {
	return a.store.Authorize(ctx, actor, workspaceapp.ProjectCopyBinding{JobID: b.JobID, OrgID: b.OrgID, SourceProjectID: b.SourceProjectID, TargetProjectID: b.TargetProjectID}, target)
}
func projectCopyBibleBinding(b workspaceapp.ProjectCopyBinding) bibleapp.ProjectCopyBinding {
	return bibleapp.ProjectCopyBinding{JobID: b.JobID, OrgID: b.OrgID, SourceProjectID: b.SourceProjectID, TargetProjectID: b.TargetProjectID}
}

func workspaceBibleCopyCounts(c bibleapp.CopyCounts) workspacedomain.ProjectCopyBibleCounts {
	return workspacedomain.ProjectCopyBibleCounts{Characters: c.Characters, CharacterVersions: c.CharacterVersions, CharacterConfirmations: c.CharacterConfirmations, Locations: c.Locations, LocationVersions: c.LocationVersions, LocationConfirmations: c.LocationConfirmations, Props: c.Props, PropVersions: c.PropVersions, PropConfirmations: c.PropConfirmations, Looks: c.Looks, LookVersions: c.LookVersions, References: c.References, Voices: c.Voices, Redirects: c.Redirects, Splits: c.Splits}
}

func bibleOwnerCopyCounts(c workspacedomain.ProjectCopyBibleCounts) bibleapp.CopyCounts {
	return bibleapp.CopyCounts{Characters: c.Characters, CharacterVersions: c.CharacterVersions, CharacterConfirmations: c.CharacterConfirmations, Locations: c.Locations, LocationVersions: c.LocationVersions, LocationConfirmations: c.LocationConfirmations, Props: c.Props, PropVersions: c.PropVersions, PropConfirmations: c.PropConfirmations, Looks: c.Looks, LookVersions: c.LookVersions, References: c.References, Voices: c.Voices, Redirects: c.Redirects, Splits: c.Splits}
}

func workspaceBibleCopySnapshot(s bibleapp.ProjectCopySnapshot) workspacedomain.ProjectCopyBibleSnapshot {
	r := workspacedomain.ProjectCopyBibleSnapshot{ID: s.ID, ManifestSHA256: s.ManifestSHA256, ContentSHA256: s.ContentSHA256, Counts: workspaceBibleCopyCounts(s.Counts), Identities: make([]workspacedomain.ProjectCopyBibleMapping, len(s.Identities)), Versions: make([]workspacedomain.ProjectCopyBibleMapping, len(s.Versions))}
	for i, m := range s.Identities {
		r.Identities[i] = workspacedomain.ProjectCopyBibleMapping{Kind: string(m.Kind), SourceID: m.SourceID, TargetID: m.TargetID}
	}
	for i, m := range s.Versions {
		r.Versions[i] = workspacedomain.ProjectCopyBibleMapping{Kind: string(m.Kind), SourceID: m.SourceID, TargetID: m.TargetID}
	}
	return r
}
func bibleOwnerCopySnapshot(s workspacedomain.ProjectCopyBibleSnapshot) bibleapp.ProjectCopySnapshot {
	r := bibleapp.ProjectCopySnapshot{ID: s.ID, ManifestSHA256: s.ManifestSHA256, ContentSHA256: s.ContentSHA256, Counts: bibleOwnerCopyCounts(s.Counts), Identities: make([]bibleapp.IdentityMapping, len(s.Identities)), Versions: make([]bibleapp.VersionMapping, len(s.Versions))}
	for i, m := range s.Identities {
		r.Identities[i] = bibleapp.IdentityMapping{Kind: bibledomain.Kind(m.Kind), SourceID: m.SourceID, TargetID: m.TargetID}
	}
	for i, m := range s.Versions {
		r.Versions[i] = bibleapp.VersionMapping{Kind: bibledomain.Kind(m.Kind), SourceID: m.SourceID, TargetID: m.TargetID}
	}
	return r
}
func workspaceBibleCopyReceipt(r bibleapp.ProjectCopyReceipt) workspacedomain.ProjectCopyBibleReceipt {
	return workspacedomain.ProjectCopyBibleReceipt{ManifestSHA256: r.ManifestSHA256, ContentSHA256: r.ContentSHA256, Counts: workspaceBibleCopyCounts(r.Counts)}
}
func projectCopyBibleError(err error) error {
	if err == nil {
		return nil
	}
	var transfer *bibleapp.ProjectCopyTransferError
	if errors.As(err, &transfer) {
		return &workspaceapp.ProjectCopyBibleTransferError{Code: transfer.Code, NeedsReconciliation: transfer.NeedsReconciliation, Cause: err}
	}
	if errors.Is(err, bibleapp.ErrUnavailable) {
		return errors.Join(workspaceapp.ErrProjectDependencyUnavailable, err)
	}
	return err
}
func provideBibleProjectCopyStore(database *gorm.DB, authority workspaceapp.ProjectCopyAuthority, mediaSnapshot mediaapp.ProjectCopySnapshot) *pgbible.ProjectCopyStore {
	return pgbible.NewProjectCopyStore(database, func(tx *gorm.DB) bibleapp.ProjectCopyAccess {
		return projectCopyBibleAccess{store: pgworkspace.NewProjectCopyAccessStore(tx, authority)}
	}, func(tx *gorm.DB) bibleapp.ProjectCopyMedia {
		return projectCopyBibleMedia{query: mediaapp.NewProjectCopyReferenceQuery(pgmedia.NewProjectCopyStore(tx), nil), snapshot: mediaSnapshot}
	}, func(tx *gorm.DB) bibleapp.ProjectCopyScopes {
		return projectCopyBibleScopes{store: provideScriptProjectCopyStore(tx, authority)}
	})
}

type projectCopyBibleOwner struct{ store *pgbible.ProjectCopyStore }

func (o projectCopyBibleOwner) ReferencedMediaFacts(ctx context.Context, actor identityapp.Principal, b workspaceapp.ProjectCopyBinding) ([]mediaapp.ReferenceFact, error) {
	facts, err := o.store.ReferencedMediaFacts(ctx, actor, projectCopyBibleBinding(b))
	if err != nil {
		return nil, projectCopyBibleError(err)
	}
	result := make([]mediaapp.ReferenceFact, len(facts))
	for i, f := range facts {
		result[i] = bibleCopyMediaFact(f)
	}
	return result, nil
}
func (o projectCopyBibleOwner) Freeze(ctx context.Context, actor identityapp.Principal, b workspaceapp.ProjectCopyBinding, assets map[uuid.UUID]uuid.UUID, at time.Time) (workspacedomain.ProjectCopyBibleSnapshot, error) {
	s, err := o.store.Freeze(ctx, actor, projectCopyBibleBinding(b), assets, at)
	return workspaceBibleCopySnapshot(s), projectCopyBibleError(err)
}
func (o projectCopyBibleOwner) Register(ctx context.Context, actor identityapp.Principal, b workspaceapp.ProjectCopyBinding, s workspacedomain.ProjectCopyBibleSnapshot) (workspacedomain.ProjectCopyBibleReceipt, error) {
	r, err := o.store.Register(ctx, actor, projectCopyBibleBinding(b), bibleOwnerCopySnapshot(s))
	return workspaceBibleCopyReceipt(r), projectCopyBibleError(err)
}
func (o projectCopyBibleOwner) Verify(ctx context.Context, actor identityapp.Principal, b workspaceapp.ProjectCopyBinding, s workspacedomain.ProjectCopyBibleSnapshot) (workspacedomain.ProjectCopyBibleReceipt, error) {
	r, err := o.store.Verify(ctx, actor, projectCopyBibleBinding(b), bibleOwnerCopySnapshot(s))
	return workspaceBibleCopyReceipt(r), projectCopyBibleError(err)
}
func (o projectCopyBibleOwner) FinishCleanup(ctx context.Context, actor identityapp.Principal, b workspaceapp.ProjectCopyBinding, s workspacedomain.ProjectCopyBibleSnapshot) error {
	return projectCopyBibleError(o.store.Cleanup(ctx, actor, projectCopyBibleBinding(b), bibleOwnerCopySnapshot(s)))
}

type projectCopyBibleTransfer struct {
	owner     *bibleapp.ProjectCopy
	database  *gorm.DB
	authority workspaceapp.ProjectCopyAuthority
	snapshot  mediaapp.ProjectCopySnapshot
}

func (t projectCopyBibleTransfer) Transfer(ctx context.Context, actor identityapp.Principal, b workspaceapp.ProjectCopyBinding, s workspacedomain.ProjectCopyBibleSnapshot) error {
	return projectCopyBibleError(t.owner.Transfer(ctx, actor, projectCopyBibleBinding(b), bibleOwnerCopySnapshot(s)))
}
func (t projectCopyBibleTransfer) Cleanup(ctx context.Context, actor identityapp.Principal, b workspaceapp.ProjectCopyBinding, s workspacedomain.ProjectCopyBibleSnapshot) error {
	return projectCopyBibleError(t.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return provideBibleProjectCopyStore(tx, t.authority, t.snapshot).Cleanup(ctx, actor, projectCopyBibleBinding(b), bibleOwnerCopySnapshot(s))
	}))
}
func provideProjectCopyBibleTransfer(database *gorm.DB, storage *objectstorage.Client) workspaceapp.ProjectCopyBibleTransferFactory {
	return func(job workspacedomain.ProjectCopyJob, worker uuid.UUID, cleanup bool) workspaceapp.ProjectCopyBibleTransfer {
		phase := "bible_transfer"
		if job.Stage == "finalizing" {
			phase = "bible_verify"
		}
		if cleanup {
			phase = "cleanup"
		}
		authority := workspaceapp.ProjectCopyAuthority{Binding: workspaceapp.ProjectCopyBinding{JobID: job.ID, OrgID: job.OrgID, SourceProjectID: job.SourceProjectID, TargetProjectID: job.TargetProjectID}, ActorID: job.ExecutionActor(), WorkerID: worker, SourceRevision: job.SourceRevision, Phase: phase, Bible: job.Manifest.Bible}
		snapshot := mediaapp.ProjectCopySnapshot{ID: job.Manifest.MediaSnapshotID, ManifestSHA256: job.Manifest.MediaSHA256, Assets: job.Manifest.Assets, Renditions: job.Manifest.Renditions}
		query := mediaapp.NewProjectCopyReferenceQuery(projectCopyBibleMediaReader{database: database, authority: authority}, storage)
		return projectCopyBibleTransfer{owner: bibleapp.NewProjectCopy(provideBibleProjectCopyStore(database, authority, snapshot), projectCopyBibleTransferredMedia{query: query, snapshot: snapshot}), database: database, authority: authority, snapshot: snapshot}
	}
}
