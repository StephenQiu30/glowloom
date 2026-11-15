package microservices_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/pgsource"
	workspacesource "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	"github.com/google/uuid"
	"gorm.io/gorm"

	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

func TestCreationProjectPreparationFinalFenceAndOutboxFailureRetainOriginalUnknown(t *testing.T) {
	for _, fault := range []string{"account-revoked", "higher-project-head", "outbox-invalid-json"} {
		t.Run(fault, func(t *testing.T) {
			f, spec := projectPreparationFixture(t)
			newUseCase, _, writers, _ := projectPreparationParticipants(t, f, false, true)
			if _, err := newUseCase().Prepare(t.Context(), spec); err == nil {
				t.Fatal("expected missing Media ACK")
			}
			initial := workspacesource.InitialProjectControl(spec)
			coordinator := writefence.NewCoordinator(pgfence.NewAuthorityStore(f.creation), writers)
			expected := initial.Authority
			expected.Epoch = 0
			ready, err := coordinator.Set(t.Context(), expected, initial.CoordinationID, false)
			if err != nil || !ready.Complete {
				t.Fatalf("actual original writer readiness: %v", err)
			}
			switch fault {
			case "account-revoked":
				authority := spec.Command.Authorities[0]
				authority.Epoch++
				if err := pgfence.New(f.creation).Install(t.Context(), writefence.Control{Authority: authority, CoordinationID: uuid.New(), Blocked: true}); err != nil {
					t.Fatal(err)
				}
			case "higher-project-head":
				closed, err := coordinator.Set(t.Context(), initial.Authority, uuid.New(), true)
				if err != nil || !closed.Complete || closed.Control.Epoch != 2 {
					t.Fatalf("project closed: %+v %v", closed, err)
				}
			case "outbox-invalid-json":
				name := "project-activation-outbox-" + uuid.NewString()
				if err := f.creation.Callback().Raw().Before("gorm:raw").Register(name, func(tx *gorm.DB) {
					if strings.Contains(tx.Statement.SQL.String(), "INSERT INTO infra.outbox") && len(tx.Statement.Vars) == 4 && tx.Statement.Vars[1] == "glowloom.workspace.project_changed.v1" {
						tx.Statement.Vars[3] = "{"
					}
				}); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = f.creation.Callback().Raw().Remove(name) })
			}
			store := pgsource.New(f.creation)
			if _, err := store.ActivatePreparedProject(t.Context(), spec.BudgetRequest(), initial, nil); err == nil {
				t.Fatal("final failed fence/write returned successful activation")
			}
			var facts struct {
				Status                       string
				Receipt, Events, Activations int64
			}
			if err := f.admin.Raw(`SELECT status,(SELECT count(*) FROM workspace.project_creation_receipt WHERE actor_id=? AND idem_key=?) AS receipt,(SELECT count(*) FROM infra.outbox WHERE partition_key=? AND topic IN ('glowloom.workspace.project_changed.v1','glowloom.audit.recorded.v1')) AS events,(SELECT count(*) FROM workspace.project_preparation WHERE preparation_id=? AND activated_at IS NOT NULL) AS activations FROM workspace.project WHERE id=?`, spec.ActorID, spec.IdempotencyKey, spec.Project.ID.String(), spec.PreparationID, spec.Project.ID).Scan(&facts).Error; err != nil {
				t.Fatal(err)
			}
			if facts.Status != "copying" || facts.Receipt != 0 || facts.Events != 0 || facts.Activations != 0 {
				t.Fatalf("failed activation leaked partial local facts: %+v", facts)
			}
			command, err := pgfence.New(f.creation).Load(t.Context(), spec.Command.ID)
			if err != nil || command.State != writefence.Admitted || len(command.Receipt) != 0 {
				t.Fatalf("original unknown responsibility lost: %+v %v", command, err)
			}
			if fault == "higher-project-head" {
				if _, err := newUseCase().Prepare(t.Context(), spec); err == nil {
					t.Fatal("original preparation reopened a higher blocked project head")
				}
				var head struct {
					Epoch   int64
					Blocked bool
				}
				if err := f.creation.Raw(`SELECT epoch,blocked FROM infra.authority_head WHERE service_owner=infra.runtime_owner() AND issuer='creation' AND authority_kind='project' AND authority_id=?`, spec.Project.ID).Scan(&head).Error; err != nil || head.Epoch != 2 || !head.Blocked {
					t.Fatalf("old initialization replaced current head: %+v %v", head, err)
				}
			}
		})
	}
}

func TestCreationProjectPreparationRecoveryRejectsOldTaskLeaseAndChangedReference(t *testing.T) {
	f, spec := projectPreparationFixture(t)
	store := pgsource.New(f.creation)
	if _, err := store.PrepareProject(t.Context(), spec); err != nil {
		t.Fatal(err)
	}
	tasks := taskruntime.NewStore(f.creation).ForBusiness(spec.PreparationID.String())
	job, err := tasks.Claim(t.Context(), []string{workspacesource.ProjectPreparationTask}, "project-old-worker", time.Minute)
	if err != nil || job == nil {
		t.Fatalf("claim original task: %v", err)
	}
	if _, err := store.ReadProjectPreparationForTask(t.Context(), *job); err != nil {
		t.Fatal(err)
	}
	changed := *job
	changed.Reference = []byte(`{"preparation_id":"` + uuid.NewString() + `"}`)
	if _, err := store.ReadProjectPreparationForTask(t.Context(), changed); !errors.Is(err, taskruntime.ErrLeaseLost) {
		t.Fatalf("changed reference accepted under original lease: %v", err)
	}
	if err := f.admin.Exec(`UPDATE infra.task_work SET epoch=epoch+1,owner='project-new-worker' WHERE service_owner='creation' AND kind=? AND business_id=?`, workspacesource.ProjectPreparationTask, spec.PreparationID.String()).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReadProjectPreparationForTask(t.Context(), *job); !errors.Is(err, taskruntime.ErrLeaseLost) {
		t.Fatalf("old lease read original recovery responsibility: %v", err)
	}
	key := spec.BudgetRequest()
	budget := workspacesource.PreparedProjectBudget{Request: key, BudgetID: uuid.NewSHA1(spec.PreparationID, []byte("billing.project_budget")), BudgetRevision: 1, LimitMicros: 9007199254740991, WalletPrepared: true}
	if _, err := store.SaveProjectBudget(t.Context(), key, budget, job); !errors.Is(err, taskruntime.ErrLeaseLost) {
		t.Fatalf("old lease stored a participant ACK: %v", err)
	}
	if err := tasks.Complete(t.Context(), *job); !errors.Is(err, taskruntime.ErrLeaseLost) {
		t.Fatalf("old worker completed task: %v", err)
	}
}
