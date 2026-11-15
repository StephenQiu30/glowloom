package microservices_test

import (
	"testing"

	"github.com/google/uuid"

	workspacesource "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

func TestCreationProjectPreparationUsesActualHeadGenerationsAndStableOriginalResources(t *testing.T) {
	account, session, scope, org, key := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	identity := workspacesource.SessionAuthorities{Identity: workspacesource.Identity{AccountID: account, SessionID: session, CredentialEpoch: 1}, Account: writefence.Authority{Issuer: writefence.Identity, Kind: writefence.Account, ID: account, Epoch: 2}, Session: writefence.Authority{Issuer: writefence.Identity, Kind: writefence.Session, ID: session, Epoch: 3}}
	local := workspacesource.ProjectCreationScope{ScopeID: scope, OrganizationID: org, Revision: 17, Authority: writefence.Authority{Issuer: writefence.Creation, Kind: writefence.Scope, ID: scope, Epoch: 4}}
	style := "personal-style"
	input := workspacesource.CreateProjectInput{Name: "  original project  ", Description: "original", AspectRatio: "9:16", StyleKey: &style, RequestID: uuid.NewString()}
	spec, err := workspacesource.BuildProjectPreparation(identity, local, key, input)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Command.Authorities[0].Epoch != 2 || spec.Command.Authorities[1].Epoch != 3 || spec.Command.Authorities[2].Epoch != 4 || spec.ScopeRevision != 17 {
		t.Fatal("credential epoch/resource revision replaced actual source heads")
	}
	if spec.Project.Name != "original project" || spec.Project.StyleType != "realistic" || spec.Project.Status != "copying" || spec.Project.OrgID != org || spec.Project.OwnerID != account || spec.Project.CanvasID == uuid.Nil || len(spec.Command.Authorities) != 3 {
		t.Fatalf("original project rules: %+v", spec.Project)
	}
	retry := input
	retry.RequestID = uuid.NewString()
	second, err := workspacesource.BuildProjectPreparation(identity, local, key, retry)
	if err != nil {
		t.Fatal(err)
	}
	if second.PreparationID != spec.PreparationID || second.Project.ID != spec.Project.ID || second.Project.CanvasID != spec.Project.CanvasID || second.Command.ID != spec.Command.ID || second.InputSHA256 != spec.InputSHA256 {
		t.Fatal("audit request replaced original logical request/resource identities")
	}
	independent := input
	independent.IndependentCanvas = true
	other, err := workspacesource.BuildProjectPreparation(identity, local, key, independent)
	if err != nil || other.InputSHA256 == spec.InputSHA256 || other.Command.Action != "creation.independent_canvas.create" {
		t.Fatalf("public route fingerprint/action lost: %v", err)
	}
	style = "mutated caller memory"
	if *spec.Project.StyleKey != "personal-style" {
		t.Fatal("accepted specification aliases caller's mutable style selection")
	}
}
