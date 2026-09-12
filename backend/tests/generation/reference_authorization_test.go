package generation_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	owner "github.com/StephenQiu30/lanverse/backend/internal/platform/ownercollection"
)

func TestInitialReferenceAuthorizationFreezesHumanIntent(t *testing.T) {
	input := referenceAuthorizationInput()
	authorization, err := domain.BuildInitialReferenceGenerationAuthorization(input)
	if err != nil {
		t.Fatal(err)
	}
	if authorization.Kind != "initial_generation" || authorization.ReasonCode != "initial_generation" || authorization.BaseGenerationTargetRef != nil || authorization.BaseCandidateSetRef != nil || len(authorization.ContentHash) != 64 {
		t.Fatalf("invalid initial authorization: %#v", authorization)
	}
	raw, err := json.Marshal(authorization)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := domain.DecodeReferenceGenerationAuthorization(raw)
	if err != nil || !reflect.DeepEqual(decoded, authorization) {
		t.Fatalf("authorization roundtrip: %v", err)
	}
	for name, invalid := range map[string]string{
		"hash drift":                    strings.Replace(string(raw), `"requested_candidate_bundle_count":2`, `"requested_candidate_bundle_count":3`, 1),
		"provider field":                strings.Replace(string(raw), `{`, `{"provider":"openai",`, 1),
		"retry masquerading as initial": strings.Replace(string(raw), `"reason_code":"initial_generation"`, `"reason_code":"retry_before_dispatch"`, 1),
		"duplicate key":                 strings.Replace(string(raw), `{`, `{"kind":"initial_generation",`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := domain.DecodeReferenceGenerationAuthorization([]byte(invalid)); err == nil {
				t.Fatal("invalid authorization accepted")
			}
		})
	}
	input.RequestedCandidateBundleCount = 3
	changed, err := domain.BuildInitialReferenceGenerationAuthorization(input)
	if err != nil || changed.ContentHash == authorization.ContentHash {
		t.Fatalf("bundle request is not frozen: %v", err)
	}
}

func TestInitialReferenceAuthorizationRejectsInvalidScope(t *testing.T) {
	for name, mutate := range map[string]func(*domain.InitialReferenceGenerationAuthorizationInput){
		"cross project": func(v *domain.InitialReferenceGenerationAuthorizationInput) {
			v.ReferencePlanTargetRef.ProjectID = uuid.NewString()
		},
		"wrong owner": func(v *domain.InitialReferenceGenerationAuthorizationInput) {
			v.ApprovedReferencePlanVersionRef.OwnerKind = "storyboard"
		},
		"unbounded bundle":      func(v *domain.InitialReferenceGenerationAuthorizationInput) { v.RequestedCandidateBundleCount = 5 },
		"missing user action":   func(v *domain.InitialReferenceGenerationAuthorizationInput) { v.HumanActionRef = "" },
		"expired token version": func(v *domain.InitialReferenceGenerationAuthorizationInput) { v.MembershipTokenVersion = 0 },
		"mutable hash": func(v *domain.InitialReferenceGenerationAuthorizationInput) {
			v.ReferencePlanTargetRef.OwnerContentHash = "latest"
		},
	} {
		t.Run(name, func(t *testing.T) {
			v := referenceAuthorizationInput()
			mutate(&v)
			if _, err := domain.BuildInitialReferenceGenerationAuthorization(v); err == nil {
				t.Fatal("invalid authorization accepted")
			}
		})
	}
}

func referenceAuthorizationInput() domain.InitialReferenceGenerationAuthorizationInput {
	workspace, project := uuid.NewString(), uuid.NewString()
	plan := owner.VersionRef{WorkspaceID: workspace, ProjectID: project, OwnerKind: "production/reference", VersionFamily: "reference_plan_set", OwnerLogicalID: uuid.NewString(), OwnerVersionID: uuid.NewString(), OwnerRevision: 1, OwnerContentHash: strings.Repeat("a", 64)}
	target := plan
	key, _ := json.Marshal([]any{"character_identity_anchor", []string{"asset", "asset_identity_state_set", uuid.NewString(), ""}})
	target.OwnerLogicalID = string(key)
	target.OwnerVersionID = uuid.NewString()
	target.OwnerContentHash = strings.Repeat("b", 64)
	return domain.InitialReferenceGenerationAuthorizationInput{ApprovedReferencePlanVersionRef: plan, ReferencePlanTargetRef: target, RequestedCandidateBundleCount: 2, HumanActionRef: uuid.NewString(), MembershipTokenVersion: 1, AuthorizedBy: uuid.NewString(), AuthorizedAt: time.Date(2026, 9, 12, 8, 0, 0, 0, time.UTC)}
}
