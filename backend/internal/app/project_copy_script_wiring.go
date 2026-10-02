package app

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstorage"
	scriptobjects "github.com/StephenQiu30/lanverse/backend/internal/script/adapter/objects"
	pgscript "github.com/StephenQiu30/lanverse/backend/internal/script/adapter/postgres"
	scriptapp "github.com/StephenQiu30/lanverse/backend/internal/script/application"
	pgworkspace "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	workspacedomain "github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

type projectCopyScriptAccess struct {
	store *pgworkspace.ProjectCopyAccessStore
}

func (a projectCopyScriptAccess) Authorize(ctx context.Context, actor identityapp.Principal, b scriptapp.ProjectCopyBinding, target bool) error {
	return a.store.Authorize(ctx, actor, workspaceapp.ProjectCopyBinding{JobID: b.JobID, OrgID: b.OrgID, SourceProjectID: b.SourceProjectID, TargetProjectID: b.TargetProjectID}, target)
}

func projectCopyScriptBinding(b workspaceapp.ProjectCopyBinding) scriptapp.ProjectCopyBinding {
	return scriptapp.ProjectCopyBinding{JobID: b.JobID, OrgID: b.OrgID, SourceProjectID: b.SourceProjectID, TargetProjectID: b.TargetProjectID}
}

func projectCopyScriptCounts(c scriptapp.ScriptCopyCounts) workspacedomain.ProjectCopyScriptCounts {
	return workspacedomain.ProjectCopyScriptCounts{Sources: c.Sources, Versions: c.Versions, VersionSources: c.VersionSources, ProjectStates: c.ProjectStates, VersionHeads: c.VersionHeads, SplitSets: c.SplitSets, SplitConfirmations: c.SplitConfirmations, Episodes: c.Episodes, Structures: c.Structures, Scenes: c.Scenes, DialogueLines: c.DialogueLines, ActionLines: c.ActionLines, Objects: c.Objects}
}

func scriptProjectCopySnapshot(s workspacedomain.ProjectCopyScriptSnapshot) scriptapp.ProjectCopySnapshot {
	c := s.Counts
	return scriptapp.ProjectCopySnapshot{ID: s.ID, ManifestSHA256: s.ManifestSHA256, ContentSHA256: s.ContentSHA256, Counts: scriptapp.ScriptCopyCounts{Sources: c.Sources, Versions: c.Versions, VersionSources: c.VersionSources, ProjectStates: c.ProjectStates, VersionHeads: c.VersionHeads, SplitSets: c.SplitSets, SplitConfirmations: c.SplitConfirmations, Episodes: c.Episodes, Structures: c.Structures, Scenes: c.Scenes, DialogueLines: c.DialogueLines, ActionLines: c.ActionLines, Objects: c.Objects}}
}

func projectCopyScriptReceipt(r scriptapp.ProjectCopyReceipt) workspacedomain.ProjectCopyScriptReceipt {
	return workspacedomain.ProjectCopyScriptReceipt{ManifestSHA256: r.ManifestSHA256, ContentSHA256: r.ContentSHA256, Counts: projectCopyScriptCounts(r.Counts)}
}

func projectCopyScriptError(err error) error {
	if err == nil {
		return nil
	}
	var transfer *scriptapp.ProjectCopyTransferError
	if errors.As(err, &transfer) {
		return &workspaceapp.ProjectCopyScriptTransferError{Code: transfer.Code, NeedsReconciliation: transfer.NeedsReconciliation, Cause: err}
	}
	if errors.Is(err, scriptapp.ErrUnavailable) || errors.Is(err, scriptapp.ErrContextUnavailable) {
		return errors.Join(workspaceapp.ErrProjectDependencyUnavailable, err)
	}
	return err
}

func provideScriptProjectCopyStore(database *gorm.DB, authority workspaceapp.ProjectCopyAuthority) *pgscript.ProjectCopyStore {
	return pgscript.NewProjectCopyStoreWithCharacters(database, func(tx *gorm.DB) scriptapp.ProjectCopyAccess {
		return projectCopyScriptAccess{store: pgworkspace.NewProjectCopyAccessStore(tx, authority)}
	}, func(tx *gorm.DB) scriptapp.ProjectCopyCharacters { return provideProjectCopyCharacters(tx, authority) })
}

type projectCopyScriptOwner struct{ store *pgscript.ProjectCopyStore }

func (o projectCopyScriptOwner) ReferencedMedia(ctx context.Context, actor identityapp.Principal, b workspaceapp.ProjectCopyBinding) ([]uuid.UUID, error) {
	ids, err := o.store.ReferencedMedia(ctx, actor, projectCopyScriptBinding(b))
	return ids, projectCopyScriptError(err)
}

func (o projectCopyScriptOwner) Freeze(ctx context.Context, actor identityapp.Principal, b workspaceapp.ProjectCopyBinding, assets map[uuid.UUID]uuid.UUID, at time.Time) (workspacedomain.ProjectCopyScriptSnapshot, error) {
	s, err := o.store.Freeze(ctx, actor, projectCopyScriptBinding(b), assets, at)
	return workspacedomain.ProjectCopyScriptSnapshot{ID: s.ID, ManifestSHA256: s.ManifestSHA256, ContentSHA256: s.ContentSHA256, Counts: projectCopyScriptCounts(s.Counts)}, projectCopyScriptError(err)
}

func (o projectCopyScriptOwner) Register(ctx context.Context, actor identityapp.Principal, b workspaceapp.ProjectCopyBinding, s workspacedomain.ProjectCopyScriptSnapshot) (workspacedomain.ProjectCopyScriptReceipt, error) {
	r, err := o.store.Register(ctx, actor, projectCopyScriptBinding(b), scriptProjectCopySnapshot(s))
	return projectCopyScriptReceipt(r), projectCopyScriptError(err)
}

func (o projectCopyScriptOwner) FinishCleanup(ctx context.Context, actor identityapp.Principal, b workspaceapp.ProjectCopyBinding, s workspacedomain.ProjectCopyScriptSnapshot) error {
	return projectCopyScriptError(o.store.FinishCleanup(ctx, actor, projectCopyScriptBinding(b), scriptProjectCopySnapshot(s)))
}

type projectCopyScriptTransfer struct{ owner *scriptapp.ProjectCopy }

func (t projectCopyScriptTransfer) Transfer(ctx context.Context, actor identityapp.Principal, b workspaceapp.ProjectCopyBinding, s workspacedomain.ProjectCopyScriptSnapshot) error {
	r, err := t.owner.Transfer(ctx, actor, projectCopyScriptBinding(b), scriptProjectCopySnapshot(s))
	if err != nil {
		return projectCopyScriptError(err)
	}
	expected := workspacedomain.ProjectCopyScriptReceipt{ManifestSHA256: s.ManifestSHA256, ContentSHA256: s.ContentSHA256, Counts: s.Counts}
	if projectCopyScriptReceipt(r) != expected {
		return &workspaceapp.ProjectCopyScriptTransferError{Code: "script_copy_receipt_invalid", NeedsReconciliation: true, Cause: workspacedomain.ErrInvalidProjectCopy}
	}
	return nil
}

func (t projectCopyScriptTransfer) Cleanup(ctx context.Context, actor identityapp.Principal, b workspaceapp.ProjectCopyBinding, s workspacedomain.ProjectCopyScriptSnapshot) error {
	return projectCopyScriptError(t.owner.Cleanup(ctx, actor, projectCopyScriptBinding(b), scriptProjectCopySnapshot(s)))
}

func provideProjectCopyScriptTransfer(database *gorm.DB, storage *objectstorage.Client) workspaceapp.ProjectCopyScriptTransferFactory {
	return func(job workspacedomain.ProjectCopyJob, worker uuid.UUID, cleanup bool) workspaceapp.ProjectCopyScriptTransfer {
		phase := "transfer"
		if cleanup {
			phase = "cleanup"
		}
		authority := workspaceapp.ProjectCopyAuthority{Binding: workspaceapp.ProjectCopyBinding{JobID: job.ID, OrgID: job.OrgID, SourceProjectID: job.SourceProjectID, TargetProjectID: job.TargetProjectID}, ActorID: job.ExecutionActor(), WorkerID: worker, SourceRevision: job.SourceRevision, Phase: phase, Bible: job.Manifest.Bible}
		return projectCopyScriptTransfer{owner: scriptapp.NewProjectCopy(provideScriptProjectCopyStore(database, authority), scriptobjects.NewStorage(storage))}
	}
}
