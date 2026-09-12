package generation_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	generationhttp "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/httpapi"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	"github.com/google/uuid"
)

type generationAuthorizationCommand func(context.Context, application.Actor, application.AuthorizeInitialReferenceGenerationCommand) (domain.ReferenceGenerationAuthorization, error)

func (f generationAuthorizationCommand) AuthorizeInitial(ctx context.Context, actor application.Actor, command application.AuthorizeInitialReferenceGenerationCommand) (domain.ReferenceGenerationAuthorization, error) {
	return f(ctx, actor, command)
}

type generationTargetCommand func(context.Context, application.Actor, application.BuildReferenceGenerationTargetCommand) (application.ReferenceGenerationTarget, error)

func (f generationTargetCommand) BuildInitial(ctx context.Context, actor application.Actor, command application.BuildReferenceGenerationTargetCommand) (application.ReferenceGenerationTarget, error) {
	return f(ctx, actor, command)
}

type executionAuthorizationCommand func(context.Context, application.Actor, application.AuthorizeInitialReferenceExecutionCommand) (domain.ReferenceExecutionAuthorization, error)

func (f executionAuthorizationCommand) AuthorizeInitial(ctx context.Context, actor application.Actor, command application.AuthorizeInitialReferenceExecutionCommand) (domain.ReferenceExecutionAuthorization, error) {
	return f(ctx, actor, command)
}

type executionPreparationCommand func(context.Context, application.Actor, application.PrepareInitialReferenceExecutionCommand) (domain.ReferenceExecution, error)

func (f executionPreparationCommand) PrepareInitial(ctx context.Context, actor application.Actor, command application.PrepareInitialReferenceExecutionCommand) (domain.ReferenceExecution, error) {
	return f(ctx, actor, command)
}

func TestReferencePreparationHTTPBoundary(t *testing.T) {
	actor := application.Actor{UserID: uuid.NewString(), TokenVersion: 9}
	workspace, project, target, plan, brief := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	hash := strings.Repeat("a", 64)
	action := domain.GenerationActionRef{ID: uuid.NewString(), ContentHash: hash}
	ref := domain.GenerationRevisionRef{ID: target, Revision: 1, ContentHash: hash}
	policy := map[string]any{"view_role": "front", "allowed_media_types": []string{"image/png"}, "aspect_ratio": "1:1", "min_width": 1024, "min_height": 1024, "max_bytes": 1048576}
	cases := []struct {
		path, result string
		body         map[string]any
		command      any
	}{
		{"/reference-targets/" + target + "/generation-authorizations", "generation_authorization_ref", map[string]any{"workspace_id": workspace, "plan_version_id": plan, "plan_content_hash": hash, "target_content_hash": hash, "brief_revision_id": brief, "brief_revision_hash": hash, "candidate_bundle_count": 2, "idempotency_key": "initial"}, application.AuthorizeInitialReferenceGenerationCommand{WorkspaceID: workspace, ProjectID: project, PlanVersionID: plan, PlanContentHash: hash, TargetVersionID: target, TargetContentHash: hash, BriefRevisionID: brief, BriefRevisionHash: hash, CandidateBundleCount: 2, IdempotencyKey: "initial"}},
		{"/reference-generation-targets", "generation_target_ref", map[string]any{"workspace_id": workspace, "generation_authorization_ref": action, "brief_revision_id": brief, "brief_revision_hash": hash, "slot_policies": []any{policy}, "idempotency_key": "initial"}, application.BuildReferenceGenerationTargetCommand{WorkspaceID: workspace, ProjectID: project, AuthorizationID: action.ID, AuthorizationHash: hash, BriefRevisionID: brief, BriefRevisionHash: hash, SlotPolicies: []application.ReferenceOutputSlotPolicy{{ViewRole: "front", AllowedMediaTypes: []string{"image/png"}, AspectRatio: "1:1", MinWidth: 1024, MinHeight: 1024, MaxBytes: 1048576}}, IdempotencyKey: "initial"}},
		{"/reference-generation-targets/" + target + "/execution-authorizations", "execution_authorization_ref", map[string]any{"workspace_id": workspace, "target_hash": hash, "selected_provider_binding_ref": ref, "idempotency_key": "initial"}, application.AuthorizeInitialReferenceExecutionCommand{WorkspaceID: workspace, ProjectID: project, TargetRef: ref, SelectedProviderBindingRef: ref, IdempotencyKey: "initial"}},
		{"/reference-generation-targets/" + target + "/executions", "execution_ref", map[string]any{"workspace_id": workspace, "target_hash": hash, "execution_authorization_ref": action, "idempotency_key": "initial"}, application.PrepareInitialReferenceExecutionCommand{WorkspaceID: workspace, ProjectID: project, TargetRef: ref, AuthorizationRef: action, IdempotencyKey: "initial"}},
	}
	for index, item := range cases {
		t.Run(item.result, func(t *testing.T) {
			raw, _ := json.Marshal(item.body)
			for _, scenario := range []struct {
				name, body, suffix string
				authErr, ownerErr  error
				status             int
				invoke             bool
			}{
				{name: "success", body: string(raw), status: 201, invoke: true},
				{name: "auth_first", body: `not-json`, authErr: errors.New("private auth"), status: 401},
				{name: "unknown", body: strings.TrimSuffix(string(raw), "}") + `,"should_dispatch":true}`, status: 422},
				{name: "project_override", body: strings.TrimSuffix(string(raw), "}") + `,"project_id":"` + project + `"}`, status: 422},
				{name: "head_override", body: strings.TrimSuffix(string(raw), "}") + `,"expected_execution_head_revision":1}`, status: 422},
				{name: "free_prompt", body: strings.TrimSuffix(string(raw), "}") + `,"prompt":"free prompt"}`, status: 422},
				{name: "duplicate", body: strings.TrimSuffix(string(raw), "}") + `,"workspace_id":"` + workspace + `"}`, status: 422},
				{name: "query", body: string(raw), suffix: "?retry=true", status: 422},
				{name: "empty_query", body: string(raw), suffix: "?", status: 422},
				{name: "empty", body: `{}`, status: 422},
				{name: "null", body: `null`, status: 422},
				{name: "trailing", body: string(raw) + ` {}`, status: 422},
				{name: "oversized", body: strings.Repeat(" ", 17*1024) + string(raw), status: 422},
				{name: "idempotency", body: string(raw), ownerErr: platformcommand.ErrInputMismatch, status: 409, invoke: true},
				{name: "wrapped_idempotency", body: string(raw), ownerErr: fmt.Errorf("private: %w", platformcommand.ErrInputMismatch), status: 409, invoke: true},
				{name: "forbidden", body: string(raw), ownerErr: &application.Error{Code: "forbidden", Message: "Forbidden", Status: 403}, status: 403, invoke: true},
				{name: "internal", body: string(raw), ownerErr: errors.New("private credential body"), status: 500, invoke: true},
			} {
				t.Run(scenario.name, func(t *testing.T) {
					calls := 0
					check := func(ctx context.Context, gotActor application.Actor, got any, which int) {
						calls++
						if which != index || gotActor != actor || !reflect.DeepEqual(got, item.command) || ctx == nil {
							t.Fatalf("command boundary lost: %d %#v", which, got)
						}
					}
					handler, err := generationhttp.NewReferencePreparationHandler(
						generationAuthorizationCommand(func(c context.Context, a application.Actor, v application.AuthorizeInitialReferenceGenerationCommand) (domain.ReferenceGenerationAuthorization, error) {
							check(c, a, v, 0)
							return domain.ReferenceGenerationAuthorization{InitialReferenceGenerationAuthorizationInput: domain.InitialReferenceGenerationAuthorizationInput{HumanActionRef: action.ID}, ContentHash: hash}, scenario.ownerErr
						}),
						generationTargetCommand(func(c context.Context, a application.Actor, v application.BuildReferenceGenerationTargetCommand) (application.ReferenceGenerationTarget, error) {
							check(c, a, v, 1)
							return application.ReferenceGenerationTarget{ID: ref.ID, Revision: 1, ContentHash: hash, SourcePayload: json.RawMessage(`{"private":"source"}`)}, scenario.ownerErr
						}),
						executionAuthorizationCommand(func(c context.Context, a application.Actor, v application.AuthorizeInitialReferenceExecutionCommand) (domain.ReferenceExecutionAuthorization, error) {
							check(c, a, v, 2)
							return domain.ReferenceExecutionAuthorization{InitialReferenceExecutionAuthorizationInput: domain.InitialReferenceExecutionAuthorizationInput{HumanActionRef: action.ID}, ContentHash: hash}, scenario.ownerErr
						}),
						executionPreparationCommand(func(c context.Context, a application.Actor, v application.PrepareInitialReferenceExecutionCommand) (domain.ReferenceExecution, error) {
							check(c, a, v, 3)
							return domain.ReferenceExecution{InitialReferenceExecutionInput: domain.InitialReferenceExecutionInput{ID: ref.ID}, Revision: 1, ContentHash: hash}, scenario.ownerErr
						}),
						referenceProgressAuthenticator{actor: actor, err: scenario.authErr})
					if err != nil {
						t.Fatal(err)
					}
					mux := http.NewServeMux()
					handler.Register(mux)
					response := httptest.NewRecorder()
					mux.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/projects/"+project+item.path+scenario.suffix, strings.NewReader(scenario.body)))
					if response.Code != scenario.status || response.Header().Get("Cache-Control") != "no-store" || strings.Contains(response.Body.String(), "private") || (calls == 1) != scenario.invoke {
						t.Fatalf("status=%d calls=%d body=%s", response.Code, calls, response.Body.String())
					}
					if scenario.status == 201 {
						var body struct {
							Data map[string]json.RawMessage `json:"data"`
						}
						if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
							t.Fatal(err)
						}
						var expected any = action
						if index == 1 || index == 3 {
							expected = ref
						}
						encoded, _ := json.Marshal(expected)
						if len(body.Data) != 1 || string(body.Data[item.result]) != string(encoded) {
							t.Fatalf("unsafe response: %s", response.Body.String())
						}
					}
				})
			}
		})
	}
	if _, err := generationhttp.NewReferencePreparationHandler(nil, nil, nil, nil, nil); err == nil {
		t.Fatal("missing dependencies accepted")
	}
}
