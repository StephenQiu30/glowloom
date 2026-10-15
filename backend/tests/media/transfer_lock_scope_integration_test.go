package media_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

type transferProjectLockTrace struct {
	mediaapp.ScopeAccess
	transfer mediaapp.TransferScopeAccess
	writes   *[]bool
}

func (a transferProjectLockTrace) TransferActor(ctx context.Context, actor identityapp.Principal, scope domain.LibraryScope, write bool) (identityapp.Principal, error) {
	if scope.Kind == domain.LibraryProject {
		*a.writes = append(*a.writes, write)
	}
	return a.transfer.TransferActor(ctx, actor, scope, write)
}

func tracedTransferScopes(base pgmedia.ScopeAccessFactory, writes *[]bool) pgmedia.ScopeAccessFactory {
	return func(tx *gorm.DB) mediaapp.ScopeAccess {
		access := base(tx)
		return transferProjectLockTrace{ScopeAccess: access, transfer: access.(mediaapp.TransferScopeAccess), writes: writes}
	}
}

// Trace the actual production scope factory while all synthetic text, receipt
// and job rows stay in an outer rollback. No object or Temporal I/O is needed.
func TestMediaTransferScopeLocksStartAtRequiredStrength(t *testing.T) {
	database := mediaStoreDB(t)
	tx := database.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer func() {
		if err := tx.Rollback().Error; err != nil {
			t.Error("roll back synthetic transfer lock fixtures", err)
		}
	}()
	actor, project := mediaStoreProject(t, tx)
	personal := domain.LibraryScope{Kind: domain.LibraryPersonal}
	text := "Synthetic lock-order fixture"
	created, err := mediaLibraryStore(tx, libraryTestAccess, time.Now).ApplyLibraryCommand(t.Context(), actor, mediaapp.LibraryCommand{Scope: personal, Key: uuid.New(), Action: "create_text", Metadata: &mediaapp.LibraryMetadata{PlainText: &text, Title: "Lock fixture", Category: "other", Tags: []string{}}})
	if err != nil {
		t.Fatal(err)
	}
	input := mediaapp.TransferInput{Source: personal, Target: domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}, Items: []mediaapp.LibraryItemRevision{{ID: created.Items[0].ID, Revision: created.Items[0].Revision}}, ExpectedSourceRevision: created.Revision, ExpectedProjectRevision: 1, Key: uuid.New()}
	job, err := mediaTransferStore(tx, libraryTestAccess, transferTestGuards, time.Now).CreateTransfer(t.Context(), actor, input)
	if err != nil {
		t.Fatal(err)
	}
	var writes []bool
	requests := pgmedia.NewScopedTransferStore(tx, app.NewMediaLibraryProjectAccess, transferTestGuards, time.Now, tracedTransferScopes(app.NewMediaScopeAccess, &writes))
	workers := pgmedia.NewScopedTransferStore(tx, app.NewMediaLibraryWorkerProjectAccess, transferTestGuards, time.Now, tracedTransferScopes(app.NewMediaWorkerScopeAccess, &writes))
	checkFirst := func(name string, write bool) {
		t.Helper()
		if len(writes) == 0 || writes[0] != write {
			t.Fatal("scope lock strength must be chosen before reading a transfer job", name, writes, write)
		}
		writes = nil
	}
	if _, err := requests.GetTransfer(t.Context(), actor, job.ID); err != nil {
		t.Fatal(err)
	}
	checkFirst("query", false)
	record := transferEventRecord(t, transferRecoveryFixture{db: tx, actor: actor}, input.Key)
	var envelope struct{ Data mediaapp.TransferDelivery }
	if err := json.Unmarshal(record.Value, &envelope); err != nil {
		t.Fatal(err)
	}
	if valid, err := workers.VerifyTransferDelivery(t.Context(), envelope.Data); err != nil || !valid {
		t.Fatal("actual permanent transfer delivery", valid, err)
	}
	checkFirst("delivery", true)
	lease, err := workers.ClaimTransfer(t.Context(), transferExecution(job))
	if err != nil {
		t.Fatal(err)
	}
	checkFirst("claim", true)
	if err := workers.HeartbeatTransfer(t.Context(), lease); err != nil {
		t.Fatal(err)
	}
	checkFirst("lease", true)
	current, err := requests.GetTransfer(t.Context(), actor, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	checkFirst("running query", false)
	if _, err := requests.ControlTransfer(t.Context(), actor, job.ID, uuid.New(), current.Revision, "cancel"); err != nil {
		t.Fatal(err)
	}
	checkFirst("control", true)
}
