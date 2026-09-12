package generation_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
)

func TestInitialReferenceExecutionAuthorizationFreezesSelectedConfiguration(t *testing.T) {
	input := domain.InitialReferenceExecutionAuthorizationInput{
		WorkspaceID: uuid.NewString(), ProjectID: uuid.NewString(),
		GenerationTargetRef:                      domain.GenerationRevisionRef{ID: uuid.NewString(), Revision: 1, ContentHash: strings.Repeat("a", 64)},
		SelectedProjectProviderBindingVersionRef: domain.GenerationRevisionRef{ID: uuid.NewString(), Revision: 3, ContentHash: strings.Repeat("b", 64)},
		HumanActionRef:                           uuid.NewString(), AuthorizedBy: uuid.NewString(), MembershipTokenVersion: 1,
		AuthorizedAt: time.Date(2026, 9, 12, 9, 0, 0, 123456789, time.FixedZone("local", 8*3600)),
	}
	authorization, err := domain.BuildInitialReferenceExecutionAuthorization(input)
	if err != nil {
		t.Fatal(err)
	}
	if authorization.Kind != "initial_execution" || authorization.PreviousExecutionRef != nil || authorization.UnresolvedCallAcknowledgements == nil || len(authorization.UnresolvedCallAcknowledgements) != 0 || authorization.AuthorizedAt.Nanosecond()%1000 != 0 || authorization.AuthorizedAt.Location() != time.UTC {
		t.Fatal("invalid initial execution authorization")
	}
	raw, err := json.Marshal(authorization)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := domain.DecodeReferenceExecutionAuthorization(raw)
	if err != nil || !reflect.DeepEqual(decoded, authorization) {
		t.Fatalf("authorization roundtrip: %v", err)
	}
	for name, changed := range map[string]string{
		"switch":                   strings.Replace(string(raw), `"kind":"initial_execution"`, `"kind":"switch_provider"`, 1),
		"retry":                    strings.Replace(string(raw), `"reason_code":"initial_execution"`, `"reason_code":"retry_before_dispatch"`, 1),
		"acknowledgements":         strings.Replace(string(raw), `"unresolved_call_acknowledgements":[]`, `"unresolved_call_acknowledgements":["unknown-call"]`, 1),
		"missing_acknowledgements": strings.Replace(string(raw), `"unresolved_call_acknowledgements":[]`, `"unresolved_call_acknowledgements":null`, 1),
		"secret":                   strings.Replace(string(raw), `{`, `{"api_key":"forbidden",`, 1),
		"duplicate_key":            strings.Replace(string(raw), `{`, `{"kind":"initial_execution",`, 1),
		"binding_hash":             strings.Replace(string(raw), strings.Repeat("b", 64), strings.Repeat("c", 64), 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := domain.DecodeReferenceExecutionAuthorization([]byte(changed)); err == nil {
				t.Fatal("invalid authorization accepted")
			}
		})
	}
	changed := input
	changed.SelectedProjectProviderBindingVersionRef.ID = uuid.NewString()
	different, err := domain.BuildInitialReferenceExecutionAuthorization(changed)
	if err != nil || different.ContentHash == authorization.ContentHash {
		t.Fatal("binding identity was not frozen")
	}
	for _, mutate := range []func(*domain.InitialReferenceExecutionAuthorizationInput){
		func(v *domain.InitialReferenceExecutionAuthorizationInput) { v.WorkspaceID = "current" },
		func(v *domain.InitialReferenceExecutionAuthorizationInput) { v.GenerationTargetRef.Revision = 0 },
		func(v *domain.InitialReferenceExecutionAuthorizationInput) {
			v.SelectedProjectProviderBindingVersionRef.ContentHash = "latest"
		},
		func(v *domain.InitialReferenceExecutionAuthorizationInput) { v.MembershipTokenVersion = 0 },
		func(v *domain.InitialReferenceExecutionAuthorizationInput) { v.HumanActionRef = "" },
	} {
		changed = input
		mutate(&changed)
		if _, err := domain.BuildInitialReferenceExecutionAuthorization(changed); err == nil {
			t.Fatal("invalid authorization input accepted")
		}
	}
}
