package workflow_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/access/authentication"
	generationhttp "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/httpapi"
	genapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	gen "github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	generationtestgorm "github.com/StephenQiu30/lanverse/backend/tests/generation/adapter/gormdb"
	"github.com/google/uuid"
)

// This uses the accepted script/Brief facts and real SQL Owners from the existing
// journey, with the production JWT verifier and an isolated synthetic signing key.
func prepareReferenceOverHTTP(t *testing.T, ctx context.Context, database *generationtestgorm.Database, actor genapp.Actor, authorization genapp.AuthorizeInitialReferenceGenerationCommand, build genapp.BuildReferenceGenerationTargetCommand, authorizer *genapp.ReferenceGenerationAuthorizationService, builder *genapp.ReferenceGenerationTargetService, configuration referenceExecutionFixture, first referencePreparationFixture) referencePreparationFixture {
	t.Helper()
	secret := strings.Repeat("reference-http-test-only-", 3)
	issuer := authentication.NewIssuer(secret, "reference-test", "reference-test", time.Hour, time.Now, uuid.NewString)
	validToken, err := issuer.Issue(actor.UserID, actor.TokenVersion)
	if err != nil {
		t.Fatal(err)
	}
	staleToken, err := issuer.Issue(actor.UserID, actor.TokenVersion+1)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := generationhttp.NewReferencePreparationHandler(authorizer, builder, configuration.service, first.service, authentication.NewVerifier(secret, "reference-test", "reference-test", time.Now))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	handler.Register(mux)
	factCounts := func() [7]int64 {
		t.Helper()
		var counts [7]int64
		for index, table := range []any{&model.CommandReceipt{}, &model.GenerationTarget{}, &model.GenerationReferenceTargetHead{}, &model.GenerationReferenceExecution{}, &model.GenerationReferenceExecutionHead{}, &model.GenerationReferenceProviderJob{}, &model.GenerationReferenceProviderCall{}} {
			if err := database.WithContext(ctx).Model(table).Where("workspace_id = ?", authorization.WorkspaceID).Count(&counts[index]).Error; err != nil {
				t.Fatal(err)
			}
		}
		return counts
	}
	post := func(path string, body map[string]any, token string, status int) []byte {
		t.Helper()
		before := factCounts()
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodPost, "/api/projects/"+authorization.ProjectID+path, bytes.NewReader(raw)).WithContext(ctx)
		request.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, request)
		if response.Code != status || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("Reference HTTP %s: status=%d want=%d body=%s", path, response.Code, status, response.Body.String())
		}
		if status != http.StatusCreated && factCounts() != before {
			t.Fatalf("rejected HTTP command left facts: %s", path)
		}
		return response.Body.Bytes()
	}
	command := func(path string, body map[string]any) map[string]json.RawMessage {
		t.Helper()
		post(path, body, "invalid", 401)
		post(path, body, staleToken, 401)
		raw := post(path, body, validToken, 201)
		beforeReplay := factCounts()
		if replay := post(path, body, validToken, 201); !bytes.Equal(raw, replay) || factCounts() != beforeReplay {
			t.Fatalf("HTTP command replay drifted: %s", path)
		}
		var envelope struct {
			Data map[string]json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil || len(envelope.Data) != 1 {
			t.Fatalf("invalid command response: %v", err)
		}
		return envelope.Data
	}
	authorizationPath := "/reference-targets/" + authorization.TargetVersionID + "/generation-authorizations"
	authorizationBody := map[string]any{"workspace_id": authorization.WorkspaceID, "plan_version_id": authorization.PlanVersionID, "plan_content_hash": authorization.PlanContentHash, "target_content_hash": authorization.TargetContentHash, "brief_revision_id": authorization.BriefRevisionID, "brief_revision_hash": authorization.BriefRevisionHash, "candidate_bundle_count": authorization.CandidateBundleCount, "idempotency_key": authorization.IdempotencyKey}
	var authorizationRef gen.GenerationActionRef
	if err := json.Unmarshal(command(authorizationPath, authorizationBody)["generation_authorization_ref"], &authorizationRef); err != nil || !authorizationRef.Valid() {
		t.Fatalf("generation authorization ref: %v", err)
	}
	authorizationBody["candidate_bundle_count"] = authorization.CandidateBundleCount%4 + 1
	post(authorizationPath, authorizationBody, validToken, 409)
	policies := make([]map[string]any, len(build.SlotPolicies))
	for index, policy := range build.SlotPolicies {
		policies[index] = map[string]any{"view_role": policy.ViewRole, "allowed_media_types": policy.AllowedMediaTypes, "aspect_ratio": policy.AspectRatio, "min_width": policy.MinWidth, "min_height": policy.MinHeight, "max_bytes": policy.MaxBytes}
	}
	targetBody := map[string]any{"workspace_id": build.WorkspaceID, "generation_authorization_ref": authorizationRef, "brief_revision_id": build.BriefRevisionID, "brief_revision_hash": build.BriefRevisionHash, "slot_policies": policies, "idempotency_key": build.IdempotencyKey}
	viewRole := policies[0]["view_role"]
	policies[0]["view_role"] = "unsupported_view"
	post("/reference-generation-targets", targetBody, validToken, 422)
	policies[0]["view_role"] = viewRole
	var targetRef gen.GenerationRevisionRef
	if err := json.Unmarshal(command("/reference-generation-targets", targetBody)["generation_target_ref"], &targetRef); err != nil || !targetRef.Valid() {
		t.Fatalf("generation target ref: %v", err)
	}
	targetBody["idempotency_key"] = build.IdempotencyKey + ":duplicate-head"
	post("/reference-generation-targets", targetBody, validToken, 409)
	base := "/reference-generation-targets/" + targetRef.ID
	executionAuthorizationBody := map[string]any{"workspace_id": build.WorkspaceID, "target_hash": targetRef.ContentHash, "selected_provider_binding_ref": configuration.command.SelectedProviderBindingRef, "idempotency_key": "reference-group-authorize:" + targetRef.ID}
	var executionAuthorizationRef gen.GenerationActionRef
	if err := json.Unmarshal(command(base+"/execution-authorizations", executionAuthorizationBody)["execution_authorization_ref"], &executionAuthorizationRef); err != nil || !executionAuthorizationRef.Valid() {
		t.Fatalf("execution authorization ref: %v", err)
	}
	prepare := genapp.PrepareInitialReferenceExecutionCommand{WorkspaceID: build.WorkspaceID, ProjectID: build.ProjectID, TargetRef: targetRef, AuthorizationRef: executionAuthorizationRef, IdempotencyKey: "reference-group-prepare:" + targetRef.ID}
	executionBody := map[string]any{"workspace_id": prepare.WorkspaceID, "target_hash": targetRef.ContentHash, "execution_authorization_ref": executionAuthorizationRef, "idempotency_key": prepare.IdempotencyKey}
	var executionRef gen.GenerationRevisionRef
	if err := json.Unmarshal(command(base+"/executions", executionBody)["execution_ref"], &executionRef); err != nil || !executionRef.Valid() {
		t.Fatalf("execution ref: %v", err)
	}
	executionBody["idempotency_key"] = prepare.IdempotencyKey + ":duplicate-head"
	post(base+"/executions", executionBody, validToken, 409)
	// Exact service replay reads and revalidates the HTTP-created snapshot; it does
	// not prepare a different target or hide missing HTTP persistence.
	prepared, err := first.service.PrepareInitial(ctx, actor, prepare)
	if err != nil || !reflect.DeepEqual(executionRef, gen.GenerationRevisionRef{ID: prepared.ID, Revision: prepared.Revision, ContentHash: prepared.ContentHash}) {
		t.Fatalf("HTTP preparation was not persisted: %v", err)
	}
	return referencePreparationFixture{service: first.service, command: prepare, execution: prepared}
}
