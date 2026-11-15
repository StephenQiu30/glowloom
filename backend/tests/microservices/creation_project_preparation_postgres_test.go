package microservices_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/pgsource"
	workspacesource "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"
	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"

	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

// This isolates Creation's actual project preparation transaction using explicit
// policy heads. It does not stand in for native registration or Identity proof.
func projectPreparationFixture(t *testing.T) (*sourcePolicyFixture, workspacesource.ProjectPreparationSpec) {
	t.Helper()
	f := newSourcePolicyFixture(t)
	var scope struct {
		PhysicalScopeID uuid.UUID
		Revision        int64
	}
	if err := f.creation.Raw(`SELECT physical_scope_id,revision FROM workspace.creation_scope WHERE id=?`, f.request.ScopeID).Scan(&scope).Error; err != nil {
		t.Fatal(err)
	}
	key := uuid.New()
	id := uuid.NewSHA1(f.request.ActorID, []byte("creation.project.preparation/"+key.String()))
	spec := workspacesource.ProjectPreparationSpec{PreparationID: id, ActorID: f.request.ActorID, SessionID: f.request.SessionID, ScopeID: f.request.ScopeID, ScopeRevision: scope.Revision, IdempotencyKey: key, RequestID: uuid.New(), InputSHA256: strings.Repeat("a", 64), Project: domain.Project{ID: uuid.NewSHA1(id, []byte("creation.project")), CanvasID: uuid.NewSHA1(id, []byte("creation.canvas")), OrgID: scope.PhysicalScopeID, OwnerID: f.request.ActorID, Name: "prepared original", Description: "original request", AspectRatio: "16:9", StyleType: "realistic", Resolution: "1080p", Status: domain.ProjectStatusCopying, Revision: 1}}
	spec.Command = writefence.Command{ID: uuid.NewSHA1(id, []byte("creation.project.create")), Action: "creation.project.create", Authorities: append([]writefence.Authority(nil), f.request.Command.Authorities[:3]...)}
	var err error
	spec.Command.SHA256, err = spec.CommandSHA256()
	if err != nil || spec.Validate() != nil {
		t.Fatalf("original spec: %v %v", err, spec.Validate())
	}
	for _, a := range spec.Command.Authorities {
		control := writefence.Control{Authority: a, CoordinationID: uuid.New()}
		if a.Issuer == writefence.Creation {
			var row struct{ CoordinationID uuid.UUID }
			if err := f.creation.Raw(`SELECT coordination_id FROM infra.authority_head WHERE service_owner=infra.runtime_owner() AND issuer=? AND authority_kind=? AND authority_id=?`, a.Issuer, a.Kind, a.ID).Scan(&row).Error; err != nil {
				t.Fatal(err)
			}
			control.CoordinationID = row.CoordinationID
		}
		if err := pgfence.New(f.creation).Install(t.Context(), control); err != nil {
			t.Fatal(err)
		}
	}
	_, err = pgsource.New(f.creation).RegisterSourceCommand(t.Context(), workspacesource.Identity{AccountID: spec.ActorID, SessionID: spec.SessionID, CredentialEpoch: 1}, workspacesource.AdmissionRequest{Command: spec.Command, Target: writefence.WriterCreation, ScopeID: spec.ScopeID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pgfence.New(f.creation).Admit(t.Context(), spec.Command); err != nil {
		t.Fatal(err)
	}
	return f, spec
}

func TestCreationProjectPreparationNeverProvesChangedHiddenResources(t *testing.T) {
	for _, change := range []string{"project-content", "canvas-deleted"} {
		t.Run(change, func(t *testing.T) {
			f, spec := projectPreparationFixture(t)
			store := pgsource.New(f.creation)
			if _, err := store.PrepareProject(t.Context(), spec); err != nil {
				t.Fatal(err)
			}
			if change == "project-content" {
				if err := f.creation.Exec(`UPDATE workspace.project SET description='uncommitted replacement' WHERE id=?`, spec.Project.ID).Error; err != nil {
					t.Fatal(err)
				}
			} else {
				if err := f.creation.Exec(`UPDATE canvas.document SET is_delete=true WHERE id=?`, spec.Project.CanvasID).Error; err != nil {
					t.Fatal(err)
				}
			}
			if _, err := store.ReadProjectPreparation(t.Context(), spec.BudgetRequest()); !errors.Is(err, workspacesource.ErrProjectPreparationConflict) {
				t.Fatalf("changed hidden source returned prepared proof: %v", err)
			}
		})
	}
}

func TestCreationProjectPreparationAtomicallyKeepsHiddenProjectCanvasAndOriginalTask(t *testing.T) {
	f, spec := projectPreparationFixture(t)
	var store workspacesource.ProjectPreparationRepository = pgsource.New(f.creation)
	state, err := store.PrepareProject(t.Context(), spec)
	if err != nil || state.Spec.PreparationID != spec.PreparationID || state.Activation != nil || state.Budget != nil {
		t.Fatalf("source prepare: %+v %v", state, err)
	}
	var facts struct {
		Status    string
		CanvasID  uuid.UUID
		Work      int64
		Admission string
	}
	if err := f.admin.Raw(`SELECT p.status,c.id AS canvas_id,(SELECT count(*) FROM infra.task_work WHERE kind=? AND business_id=?) AS work,(SELECT state FROM infra.writer_command WHERE service_owner='creation' AND id=?) AS admission FROM workspace.project p JOIN canvas.document c ON c.id=p.initial_canvas_id AND c.project_id=p.id WHERE p.id=?`, workspacesource.ProjectPreparationTask, spec.PreparationID.String(), spec.Command.ID, spec.Project.ID).Scan(&facts).Error; err != nil {
		t.Fatal(err)
	}
	if facts.Status != "copying" || facts.CanvasID != spec.Project.CanvasID || facts.Work != 1 || facts.Admission != "admitted" {
		t.Fatalf("partial initial facts: %+v", facts)
	}
	if _, err := store.PrepareProject(t.Context(), spec); err != nil {
		t.Fatal("exact original retry", err)
	}
	changed := spec
	changed.Project.Name = "different input"
	if _, err := store.PrepareProject(t.Context(), changed); !errors.Is(err, workspacesource.ErrProjectPreparationConflict) {
		t.Fatalf("changed source specification: %v", err)
	}
	if _, err := store.BeginProjectAuthority(t.Context(), spec.BudgetRequest(), nil); !errors.Is(err, workspacesource.ErrProjectPreparationUnknown) {
		t.Fatalf("opened head before wallet ready: %v", err)
	}
	var heads int
	if err := f.admin.Raw(`SELECT count(*) FROM infra.authority_head WHERE issuer='creation' AND authority_kind='project' AND authority_id=?`, spec.Project.ID).Scan(&heads).Error; err != nil || heads != 0 {
		t.Fatalf("premature project authority: %d %v", heads, err)
	}
	if err := f.creation.Exec(`DELETE FROM workspace.project_preparation WHERE preparation_id=?`, spec.PreparationID).Error; err == nil {
		t.Fatal("Creation deleted unknown responsibility")
	}
}
