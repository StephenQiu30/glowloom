package identity_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	canvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/postgres"
	workspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
	domain "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/domain"
)

// Child snapshots are controlled; these tests exercise coordinator persistence and authority.
type secondaryMedia struct {
	workspace.ProjectCopyMediaOwner
}

func (secondaryMedia) Freeze(context.Context, identity.Principal, media.ProjectCopyBinding, time.Time) (media.ProjectCopySnapshot, error) {
	return media.ProjectCopySnapshot{ID: uuid.New(), ManifestSHA256: strings.Repeat("a", 64), AssetMapping: map[uuid.UUID]uuid.UUID{}}, nil
}

type secondaryCanvas struct {
	workspace.ProjectCopyCanvasOwner
}

func (secondaryCanvas) Freeze(context.Context, identity.Principal, canvas.ProjectCopyBinding, map[uuid.UUID]uuid.UUID) (canvas.ProjectCopySnapshot, error) {
	return canvas.ProjectCopySnapshot{ID: uuid.New(), ManifestSHA256: strings.Repeat("b", 64)}, nil
}

type secondaryBudget struct{}

func (secondaryBudget) CreateCopyTargetBudget(context.Context, identity.Principal, uuid.UUID) error {
	return nil
}
func (secondaryBudget) VerifyCopyTargetBudget(context.Context, identity.Principal, uuid.UUID) error {
	return nil
}

func TestSecondaryCopyAdmissionPersistsSelectedScope(t *testing.T) {
	verifySecondaryRollback(t)
	tx, actor, team, scope, project, requests := projectConsumerFixture(t)
	installSecondaryScope(t, tx)
	personal := actor.ScopeID
	actor.ScopeID = scope
	// Include an owned preset so its cloned ownership cannot silently become NULL.
	preset := uuid.New()
	if err := tx.Exec(`INSERT INTO workspace.style_preset(id,org_id,owner_scope_id,project_id,name,style_type,prompt_fragment)VALUES(?,?,?,?,'test preset','realistic','test')`, preset, actor.OrgID, scope, project).Error; err != nil {
		t.Fatal(err)
	}
	store := pgworkspace.NewScopedProjectCopyStore(tx, requests, secondaryWorkers, func(*gorm.DB) workspace.ProjectWorkGuard { return secondaryWork{} }, func(*gorm.DB) workspace.ProjectCopyOwners {
		return workspace.ProjectCopyOwners{Media: secondaryMedia{}, Canvas: secondaryCanvas{}, Budget: secondaryBudget{}}
	})
	input := workspace.ProjectCopyInput{SourceProjectID: project, ExpectedRevision: 1, TargetName: "Copy target", IdempotencyKey: uuid.New(), RequestID: uuid.NewString()}
	saved, err := store.Create(t.Context(), actor, input, time.Now())
	if err != nil {
		t.Fatalf("collaborator admission: %v", err)
	}
	var mapping struct{ ProjectScope, JobScope, PlacementScope, PresetScope uuid.UUID }
	if err := tx.Raw(`SELECT p.owner_scope_id AS project_scope,j.owner_scope_id AS job_scope,m.owner_scope_id AS placement_scope,s.owner_scope_id AS preset_scope FROM workspace.project p JOIN workspace.project_copy_job j ON j.target_project_id=p.id JOIN workspace.project_folder_placement m ON m.project_id=p.id JOIN workspace.style_preset s ON s.project_id=p.id WHERE p.id=?`, saved.TargetProjectID).Scan(&mapping).Error; err != nil {
		t.Fatal(err)
	}
	if mapping.ProjectScope != scope || mapping.JobScope != scope || mapping.PlacementScope != scope || mapping.PresetScope != scope {
		t.Fatalf("copy inherited physical/unknown ownership: %+v", mapping)
	}
	if replay, err := store.Create(t.Context(), actor, input, time.Now()); err != nil || replay.ID != saved.ID {
		t.Fatalf("same-scope replay: %v", err)
	}
	other := actor
	other.ScopeID = personal
	if _, err := store.Create(t.Context(), other, input, time.Now()); !errors.Is(err, workspace.ErrIdempotencyConflict) {
		t.Fatalf("cross-scope replay: %v", err)
	}
	authority := workspace.ProjectCopyAuthority{Binding: workspace.ProjectCopyBinding{JobID: saved.ID, OrgID: actor.OrgID, SourceProjectID: project, TargetProjectID: saved.TargetProjectID}, ActorID: actor.ID, SourceRevision: 1, Phase: "freeze"}
	access := pgworkspace.NewScopedProjectCopyAccessStore(tx, authority, requests, secondaryWorkers)
	if err := access.Authorize(t.Context(), actor, authority.Binding, true); err != nil {
		t.Fatal(err)
	}
	if err := access.Authorize(t.Context(), other, authority.Binding, true); !errors.Is(err, workspace.ErrProjectNotFound) {
		t.Fatalf("child owner wrong-scope binding: %v", err)
	}
	if err := tx.Exec(`UPDATE workspace.team SET status='archived' WHERE id=?`, team).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := store.Find(t.Context(), actor, saved.ID); err != nil {
		t.Fatalf("archived copy read: %v", err)
	}
	if _, err := store.Create(t.Context(), actor, input, time.Now()); err == nil {
		t.Fatal("archived copy replay write")
	}
	if err := tx.Exec(`UPDATE workspace.team SET status='active' WHERE id=?`, team).Error; err != nil {
		t.Fatal(err)
	}
	worker, err := store.WorkerActor(t.Context(), actor.OrgID, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	nonce := uuid.New()
	if _, err := store.Claim(t.Context(), worker, saved.ID, nonce, false, time.Now()); err != nil {
		t.Fatal(err)
	}
	authority.Phase = "transfer"
	authority.WorkerID = uuid.New()
	stale := pgworkspace.NewScopedProjectCopyAccessStore(tx, authority, requests, secondaryWorkers)
	if err := stale.Authorize(t.Context(), worker, authority.Binding, true); !errors.Is(err, domain.ErrProjectCopyWorkerConflict) {
		t.Fatalf("child stale fence: %v", err)
	}
	if err := tx.Exec(`UPDATE workspace.team_member SET status='removed' WHERE team_id=? AND user_id=?`, team, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(t.Context(), actor, input, time.Now()); err == nil {
		t.Fatal("revoked replay")
	}
}
