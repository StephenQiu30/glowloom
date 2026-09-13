package workflow_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	agentgorm "github.com/StephenQiu30/lanverse/backend/internal/agent/adapter/gormdb"
	agentapp "github.com/StephenQiu30/lanverse/backend/internal/agent/application"
	generationgorm "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/gormdb"
	generationhttp "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/httpapi"
	genapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	gen "github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	generationtestgorm "github.com/StephenQiu30/lanverse/backend/tests/generation/adapter/gormdb"
	"github.com/google/uuid"
)

func assertReferenceCandidateSetOwner(t *testing.T, ctx context.Context, db *generationtestgorm.Database, bundles *genapp.ReferenceCandidateBundleService, reviews *agentgorm.VisionReviewStore, actor genapp.Actor, execution gen.ReferenceExecution, review agentapp.VisionReviewExecutionState) {
	t.Helper()
	store, err := generationgorm.NewReferenceCandidateSetStore(db, reviews.ReadAcceptedVisionReviewSet)
	if err != nil {
		t.Fatal(err)
	}
	service := genapp.NewReferenceCandidateSetService(store)
	executionRef := gen.GenerationRevisionRef{ID: execution.ID, Revision: execution.Revision, ContentHash: execution.ContentHash}
	bundle, err := bundles.Materialize(ctx, actor, genapp.ReferenceCandidateBundleCommand{WorkspaceID: execution.WorkspaceID, ProjectID: execution.ProjectID, ExecutionRef: executionRef, VisionReviewRef: gen.GenerationRevisionRef{ID: review.Candidate.ID, Revision: review.Candidate.Revision, ContentHash: review.Candidate.CandidateRevisionHash}})
	if err != nil {
		t.Fatal(err)
	}
	progress, err := genapp.NewReferenceExecutionQuery(generationgorm.New(db)).Get(ctx, actor, execution.ProjectID, execution.ID)
	if err != nil {
		t.Fatal(err)
	}
	command := genapp.ReferenceCandidateSetCommand{ProjectID: execution.ProjectID, ExecutionRef: executionRef, ExpectedProgressHash: progress.ContentHash, BundleRefs: []gen.GenerationRevisionRef{{ID: bundle.ID, Revision: 1, ContentHash: bundle.ContentHash}}}
	mux := http.NewServeMux()
	generationhttp.NewReferenceCandidateSetHandler(service, bundleScopeAuthenticator{actor: actor}).Register(mux)
	body, _ := json.Marshal(map[string]any{"execution_hash": execution.ContentHash, "expected_progress_hash": progress.ContentHash, "candidate_bundle_refs": command.BundleRefs})
	path := "/api/projects/" + execution.ProjectID + "/reference-executions/" + execution.ID + "/candidate-sets"
	removeFault, err := generationtestgorm.FailCandidateSetInsert(db)
	if err != nil {
		t.Fatal(err)
	}
	_, rollbackErr := service.Materialize(ctx, actor, command)
	if err := removeFault(); err != nil {
		t.Fatal(err)
	}
	var rolledBackCount int64
	if err := db.Model(&model.GenerationReferenceCandidateSet{}).Where("execution_id = ?", execution.ID).Count(&rolledBackCount).Error; err != nil || rollbackErr == nil || rolledBackCount != 0 {
		t.Fatal("candidate Set insert did not roll back atomically")
	}
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest("POST", path, bytes.NewReader(body)))
	if response.Code != http.StatusCreated || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("candidate Set POST: %d %s", response.Code, response.Body.String())
	}
	var envelope struct {
		Data gen.ReferenceCandidateSet `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	value := envelope.Data
	if value.GenerationCompletionState != "partial_explicit_failure" || len(value.BundleRefs) != 1 || len(value.FailedSlots) == 0 {
		t.Fatalf("lost explicit failure: %+v", value)
	}
	readPath := "/api/projects/" + execution.ProjectID + "/reference-candidate-sets/" + value.ID
	response = httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest("GET", readPath, nil))
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("candidate Set GET: %d", response.Code)
	}
	var loaded struct {
		Data gen.ReferenceCandidateSet `json:"data"`
	}
	if json.Unmarshal(response.Body.Bytes(), &loaded) != nil || !reflect.DeepEqual(loaded.Data, value) {
		t.Fatal("Set query changed persisted facts")
	}
	for _, private := range []string{"object_key", "staging/", "authorization", "prompt", "selection_ready", "rights_approved"} {
		if strings.Contains(response.Body.String(), private) {
			t.Fatalf("Set leaked %s", private)
		}
	}
	for _, invalid := range []struct{ method, path, body string }{
		{"POST", path, strings.TrimSuffix(string(body), "}") + `,"selected":true}`},
		{"POST", path + "?latest=true", string(body)},
		{"POST", path, `{}`},
		{"GET", readPath + "?latest=true", ""},
		{"GET", readPath, `{}`},
	} {
		response = httptest.NewRecorder()
		mux.ServeHTTP(response, httptest.NewRequest(invalid.method, invalid.path, strings.NewReader(invalid.body)))
		if response.Code != http.StatusUnprocessableEntity {
			t.Fatalf("invalid Set HTTP accepted: %d", response.Code)
		}
	}
	// A lost HTTP response is retried with the same frozen input, not a new Set.
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := service.Materialize(ctx, actor, command)
			if err == nil && !reflect.DeepEqual(got, value) {
				err = errors.New("Set retry identity changed")
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
	for _, fault := range []string{"actor", "execution", "progress", "missing_review", "duplicate_review", "review_hash", "project"} {
		bad, who := command, actor
		bad.BundleRefs = append([]gen.GenerationRevisionRef{}, command.BundleRefs...)
		switch fault {
		case "actor":
			who.TokenVersion++
		case "execution":
			bad.ExecutionRef.ContentHash = strings.Repeat("e", 64)
		case "progress":
			bad.ExpectedProgressHash = strings.Repeat("e", 64)
		case "missing_review":
			bad.BundleRefs = []gen.GenerationRevisionRef{}
		case "duplicate_review":
			bad.BundleRefs = append(bad.BundleRefs, bad.BundleRefs[0])
		case "review_hash":
			bad.BundleRefs[0].ContentHash = strings.Repeat("e", 64)
		case "project":
			bad.ProjectID = uuid.NewString()
		}
		if _, err := service.Materialize(ctx, who, bad); err == nil {
			t.Fatalf("Set accepted %s", fault)
		}
	}
	if _, err := service.Get(ctx, actor, uuid.NewString(), value.ID); err == nil {
		t.Fatal("Set crossed project scope")
	}
	var control model.SceneAnalysisControlHead
	if err := db.First(&control, "release_id = ?", review.Record.ReleaseID).Error; err != nil {
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
		t.Fatal("Set ignored revoked Control")
	}
	var row model.GenerationReferenceCandidateSet
	if err := db.First(&row, "id = ?", value.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&row).Update("content_hash", strings.Repeat("f", 64)).Error; err == nil {
		t.Fatal("Set model allowed mutation")
	}
	if err := db.Model(&row).UpdateColumn("content_hash", strings.Repeat("f", 64)).Error; err != nil {
		t.Fatal(err)
	}
	_, readErr = service.Get(ctx, actor, execution.ProjectID, value.ID)
	_, writeErr = service.Materialize(ctx, actor, command)
	if err := db.Model(&row).UpdateColumn("content_hash", value.ContentHash).Error; err != nil {
		t.Fatal(err)
	}
	if readErr == nil || writeErr == nil {
		t.Fatal("Set accepted corrupted persisted hash")
	}
	var count int64
	if err := db.Model(&model.GenerationReferenceCandidateSet{}).Where("execution_id = ?", execution.ID).Count(&count).Error; err != nil || count != 1 {
		t.Fatal("Set retries or rejected commands duplicated facts")
	}
	// Inject a valid unresolved observation into this fixture only. Restoring the
	// committed receipt represents reconciliation, not another provider send.
	var call model.GenerationReferenceProviderCall
	if err := db.Where("execution_id = ?", execution.ID).Order("bundle_index, slot_key").First(&call).Error; err != nil {
		t.Fatal(err)
	}
	original, err := gen.DecodeReferenceCallState(json.RawMessage(call.StateContent))
	if err != nil {
		t.Fatal(err)
	}
	pending, err := gen.NewReferenceCallState(call.CallKey)
	if err != nil {
		t.Fatal(err)
	}
	claimed, _, err := gen.ClaimReferenceCall(pending, *original.Dispatch)
	if err != nil {
		t.Fatal(err)
	}
	unknown, _, err := gen.ExpireReferenceCall(claimed, original.Dispatch.DeadlineAt.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(unknown)
	restore := func() error {
		return db.Model(&call).UpdateColumns(map[string]any{"status": original.Status, "revision": original.Revision, "state_hash": original.ContentHash, "state_content": call.StateContent}).Error
	}
	defer func() {
		if err := restore(); err != nil {
			t.Error(err)
		}
	}()
	if err := db.Model(&model.GenerationReferenceProviderCall{}).Where("call_key = ?", call.CallKey).UpdateColumns(map[string]any{"status": unknown.Status, "revision": unknown.Revision, "state_hash": unknown.ContentHash, "state_content": json.RawMessage(raw)}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.Get(ctx, actor, execution.ProjectID, value.ID); err == nil {
		t.Fatal("Set ignored changed execution progress")
	}
	progress, err = genapp.NewReferenceExecutionQuery(generationgorm.New(db)).Get(ctx, actor, execution.ProjectID, execution.ID)
	if err != nil {
		t.Fatal(err)
	}
	unresolved := command
	unresolved.ExpectedProgressHash, unresolved.BundleRefs = progress.ContentHash, []gen.GenerationRevisionRef{}
	reconciliation, err := service.Materialize(ctx, actor, unresolved)
	if err != nil || reconciliation.GenerationCompletionState != "outcome_unknown" || len(reconciliation.BundleRefs) != 0 || len(reconciliation.FailedSlots) == 0 {
		t.Fatalf("unknown Set: %v", err)
	}
	if _, err := service.Get(ctx, actor, execution.ProjectID, reconciliation.ID); err != nil {
		t.Fatal(err)
	}
	if err := restore(); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Get(ctx, actor, execution.ProjectID, reconciliation.ID); err == nil {
		t.Fatal("reconciled observation left old unknown Set current")
	}
	if _, err := service.Materialize(ctx, actor, unresolved); err == nil {
		t.Fatal("stale unknown input recreated current Set")
	}
	if current, err := service.Get(ctx, actor, execution.ProjectID, value.ID); err != nil || !reflect.DeepEqual(current, value) {
		t.Fatalf("restored Set: %v", err)
	}
}
