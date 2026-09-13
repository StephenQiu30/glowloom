package workflow_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/access/authentication"
	agentapp "github.com/StephenQiu30/lanverse/backend/internal/agent/application"
	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	generationhttp "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/httpapi"
	genapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	gen "github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	flow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
	generationtestgorm "github.com/StephenQiu30/lanverse/backend/tests/generation/adapter/gormdb"
	"github.com/google/uuid"
)

type referenceBundleReplyLoss struct {
	owner *genapp.ReferenceCandidateBundleService
	lost  atomic.Bool
}

type bundleScopeAuthenticator struct{ actor genapp.Actor }

func (auth bundleScopeAuthenticator) Authenticate(*http.Request) (authentication.Claims, error) {
	return authentication.Claims{UserID: auth.actor.UserID, TokenVersion: auth.actor.TokenVersion}, nil
}

func (loss *referenceBundleReplyLoss) Materialize(ctx context.Context, actor genapp.Actor, command genapp.ReferenceCandidateBundleCommand) (gen.ReferenceCandidateBundle, error) {
	value, err := loss.owner.Materialize(ctx, actor, command)
	if err == nil && loss.lost.CompareAndSwap(false, true) {
		return gen.ReferenceCandidateBundle{}, errors.New("lost candidate Bundle commit response")
	}
	return value, err
}

func assertReferenceCandidateBundleOwner(t *testing.T, ctx context.Context, db *generationtestgorm.Database, service *genapp.ReferenceCandidateBundleService, actor genapp.Actor, execution gen.ReferenceExecution, state agentapp.VisionReviewExecutionState, runID string) {
	t.Helper()
	command := genapp.ReferenceCandidateBundleCommand{WorkspaceID: execution.WorkspaceID, ProjectID: execution.ProjectID, ExecutionRef: gen.GenerationRevisionRef{ID: execution.ID, Revision: 1, ContentHash: execution.ContentHash}, VisionReviewRef: gen.GenerationRevisionRef{ID: state.Candidate.ID, Revision: state.Candidate.Revision, ContentHash: state.Candidate.CandidateRevisionHash}}
	value, err := service.Materialize(ctx, actor, command)
	if err != nil {
		t.Fatal(err)
	}
	query, err := service.Get(ctx, actor, execution.ProjectID, value.ID)
	if err != nil || !reflect.DeepEqual(query, value) {
		t.Fatalf("candidate Bundle exact read: %v", err)
	}
	review, _, err := contract.DecodeVisionReviewCandidate(state.Result.Candidate)
	if err != nil || review.Checks[0].Status != "fail" || review.Checks[1].Status != "not_assessable" || review.Checks[3].Status != "warn" {
		t.Fatal("review findings were lost or treated as visual approval")
	}
	mux := http.NewServeMux()
	generationhttp.NewReferenceCandidateBundleHandler(service, bundleScopeAuthenticator{actor: actor}).Register(mux)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest("GET", "/api/projects/"+execution.ProjectID+"/reference-candidate-bundles/"+value.ID, nil))
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("persisted Bundle HTTP: %d", response.Code)
	}
	var envelope struct {
		Data gen.ReferenceCandidateBundle `json:"data"`
	}
	if json.Unmarshal(response.Body.Bytes(), &envelope) != nil || !reflect.DeepEqual(envelope.Data, value) {
		t.Fatal("Bundle query did not return persisted facts")
	}
	for _, private := range []string{"object_key", "staging/", "authorization", "prompt", "selection_ready"} {
		if strings.Contains(response.Body.String(), private) {
			t.Fatalf("Bundle query leaked %s", private)
		}
	}
	var node model.NodeRunProjection
	if err := db.Where("workflow_run_id = ? AND definition_key = ?", runID, "generation.reference_candidate_bundle").First(&node).Error; err != nil {
		t.Fatal(err)
	}
	output, _, _, err := flow.ParseNodeOutput(json.RawMessage(node.Output))
	if err != nil || node.Status != "SUCCEEDED" || len(output.Bindings) != 1 || output.Bindings[0].ReferenceID != value.ID || output.Bindings[0].ContentHash != value.ContentHash {
		t.Fatal("Workflow did not publish exact Bundle reference")
	}
	var count int64
	if err := db.Model(&model.GenerationReferenceCandidateBundle{}).Where("vision_candidate_id = ?", state.Candidate.ID).Count(&count).Error; err != nil || count != 1 {
		t.Fatal("Bundle lost-response recovery duplicated facts")
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := service.Materialize(ctx, actor, command)
			if err == nil && !reflect.DeepEqual(got, value) {
				err = errors.New("concurrent Bundle identity changed")
			}
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	var run model.WorkflowRun
	if err := db.First(&run, "id = ?", runID).Error; err != nil {
		t.Fatal(err)
	}
	originalRunStatus := run.Status
	for _, status := range []string{"FAILED", "NEEDS_ATTENTION", "WAITING_HUMAN", "CANCELLED"} {
		if err := db.Model(&model.WorkflowRun{}).Where("id = ?", runID).UpdateColumn("status", status).Error; err != nil {
			t.Fatal(err)
		}
		_, err := service.Materialize(ctx, actor, command)
		if restoreErr := db.Model(&model.WorkflowRun{}).Where("id = ?", runID).UpdateColumn("status", originalRunStatus).Error; restoreErr != nil {
			t.Fatal(restoreErr)
		}
		if (status == "CANCELLED") != (err != nil) {
			t.Fatalf("completed review recovery under %s: %v", status, err)
		}
	}
	for _, fault := range []string{"actor", "execution", "review_hash", "project", "workspace"} {
		bad, who := command, actor
		switch fault {
		case "actor":
			who.TokenVersion++
		case "execution":
			bad.ExecutionRef.ID = uuid.NewString()
		case "review_hash":
			bad.VisionReviewRef.ContentHash = strings.Repeat("e", 64)
		case "project":
			bad.ProjectID = uuid.NewString()
		case "workspace":
			bad.WorkspaceID = uuid.NewString()
		}
		if _, err := service.Materialize(ctx, who, bad); err == nil {
			t.Fatalf("accepted Bundle %s", fault)
		}
	}
	var control model.SceneAnalysisControlHead
	if err := db.First(&control, "release_id = ?", state.Record.ReleaseID).Error; err != nil {
		t.Fatal(err)
	}
	originalStatus := control.Status
	if err := db.Model(&control).UpdateColumn("status", "revoked").Error; err != nil {
		t.Fatal(err)
	}
	_, readErr := service.Get(ctx, actor, execution.ProjectID, value.ID)
	_, writeErr := service.Materialize(ctx, actor, command)
	if err := db.Model(&control).UpdateColumn("status", originalStatus).Error; err != nil {
		t.Fatal(err)
	}
	if readErr == nil || writeErr == nil {
		t.Fatal("Bundle ignored revoked Control")
	}
	var row model.GenerationReferenceCandidateBundle
	if err := db.First(&row, "id = ?", value.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&row).Update("content_hash", strings.Repeat("f", 64)).Error; err == nil {
		t.Fatal("Bundle model allowed mutation")
	}
	if err := db.Model(&model.GenerationReferenceCandidateBundle{}).Where("id = ?", value.ID).UpdateColumn("content_hash", strings.Repeat("f", 64)).Error; err != nil {
		t.Fatal(err)
	}
	_, readErr = service.Get(ctx, actor, execution.ProjectID, value.ID)
	_, writeErr = service.Materialize(ctx, actor, command)
	if err := db.Model(&model.GenerationReferenceCandidateBundle{}).Where("id = ?", value.ID).UpdateColumn("content_hash", value.ContentHash).Error; err != nil {
		t.Fatal(err)
	}
	if readErr == nil || writeErr == nil {
		t.Fatal("Bundle accepted corrupted persisted hash")
	}
}
