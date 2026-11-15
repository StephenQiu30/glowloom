package microservices_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/executionwork"
	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/mediawork"
	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/pgsource"
	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"
	executionrpc "github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/grpcprojectwork"
	executionpg "github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/postgres"
	mediarpc "github.com/StephenQiu30/glowloom-server/backend/media/adapter/grpcprojectwork"
	mediapg "github.com/StephenQiu30/glowloom-server/backend/media/adapter/postgres"
)

func closureActualParticipants(t *testing.T, f *sourcePolicyFixture) (source.ProjectWorkReader, source.ProjectWorkReader) {
	t.Helper()
	executionTLS, executionClientTLS, _ := ownerPeerTLS(t, "creation")
	mediaTLS, mediaClientTLS, _ := ownerPeerTLS(t, "creation")
	executionToken, mediaToken := strings.Repeat("execution-work-", 4), strings.Repeat("media-work-", 4)
	executionServer, err := executionrpc.NewServer(executionpg.NewProjectWorkStore(f.execution), executionToken, executionTLS)
	if err != nil {
		t.Fatal(err)
	}
	mediaServer, err := mediarpc.NewServer(mediapg.NewProjectWorkReader(f.media), mediaToken, "creation", mediaTLS)
	if err != nil {
		t.Fatal(err)
	}
	executionClient, err := executionwork.New(serveProjectPreparation(t, executionServer), executionToken, executionClientTLS)
	if err != nil {
		t.Fatal(err)
	}
	mediaClient, err := mediawork.New(serveProjectPreparation(t, mediaServer), mediaToken, mediaClientTLS)
	if err != nil {
		t.Fatal(err)
	}
	return executionClient, mediaClient
}

// Source policy/head bootstrap remains explicit fixture. Every business probe,
// peer transport and role-restricted final source transaction below is actual.
func TestCreationProjectClosureActualRemoteMissingResponsibilityKeepsGateUnknown(t *testing.T) {
	f, store, spec := projectClosurePGFixture(t)
	id := source.Identity{AccountID: spec.ActorID, SessionID: spec.SessionID, CredentialEpoch: 1}
	if _, err := store.RegisterSourceCommand(t.Context(), id, source.AdmissionRequest{Command: f.request.Command, Target: "execution", ScopeID: spec.ScopeID, ProjectID: spec.ProjectID}); err != nil {
		t.Fatal(err)
	}
	exec, media := closureActualParticipants(t, f)
	if _, err := closurePGUseCase(t, f, store, exec, media).Close(t.Context(), spec); !errors.Is(err, source.ErrProjectClosureUnknown) {
		t.Fatalf("missing remote original admission became idle: %v", err)
	}
	state, err := store.ReadProjectClosure(t.Context(), spec.ID)
	if err != nil || !state.GateClosed || state.AuthorityStarted || state.Cycle != 1 {
		t.Fatalf("original unknown responsibility changed: %+v %v", state, err)
	}
	assertClosureProjectEpoch(t, f, 1)
	if _, err := closurePGUseCase(t, f, pgsource.New(f.creation), exec, media).Resolve(t.Context(), spec.ID); !errors.Is(err, source.ErrProjectClosureUnknown) {
		t.Fatal("rebuilt use case lost original unknown", err)
	}
}
func TestCreationProjectClosureActualRemoteIdleCommitsOriginalFourWriterClose(t *testing.T) {
	f, store, spec := projectClosurePGFixture(t)
	exec, media := closureActualParticipants(t, f)
	project, err := closurePGUseCase(t, f, store, exec, media).Close(t.Context(), spec)
	if err != nil || project.Status != "archived" || project.Revision != 2 {
		t.Fatalf("actual owner probes did not produce unique close: %+v %v", project, err)
	}
	assertClosureProjectEpoch(t, f, 2)
	state, err := store.ReadProjectClosure(t.Context(), spec.ID)
	if err != nil || state.Result == nil || state.Result.ID != project.ID {
		t.Fatal("original source decision not durable", err)
	}
}
