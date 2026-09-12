package generation_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/google/uuid"
)

func TestInitialReferenceExecutionSnapshotStrictIdentity(t *testing.T) {
	ref := func() domain.GenerationRevisionRef {
		return domain.GenerationRevisionRef{ID: uuid.NewString(), Revision: 1, ContentHash: strings.Repeat("a", 64)}
	}
	contract := domain.GenerationContractRef{ContractID: "reference-compiler", ContentHash: strings.Repeat("b", 64)}
	policy, policyErr := domain.DefaultReferenceGenerationLimits().Ref()
	if policyErr != nil {
		t.Fatal(policyErr)
	}
	input := domain.InitialReferenceExecutionInput{ID: uuid.NewString(), WorkspaceID: uuid.NewString(), ProjectID: uuid.NewString(), CreatedBy: uuid.NewString(), CreatedAt: time.Now(), ReadSet: domain.ReferenceExecutionReadSet{TargetRef: ref(), TargetReadSetRoot: strings.Repeat("c", 64), BindingRef: ref(), ConnectionRef: ref(), ProfileRef: ref(), CredentialRef: domain.ReferenceCredentialRef{ID: uuid.NewString(), Revision: 1, Fingerprint: strings.Repeat("d", 64)}, RegistryReleaseHash: strings.Repeat("e", 64), AdapterRef: contract, CompilerRef: contract, CapabilityHash: strings.Repeat("f", 64), ManifestHash: strings.Repeat("a", 64), OperationalPolicyRef: policy, AuthorizationRef: domain.GenerationActionRef{ID: uuid.NewString(), ContentHash: strings.Repeat("b", 64)}}}
	input.MembershipTokenVersion = 1
	value, err := domain.BuildInitialReferenceExecution(input)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := domain.DecodeReferenceExecution(raw)
	if err != nil || !reflect.DeepEqual(value, decoded) {
		t.Fatalf("snapshot roundtrip: %v", err)
	}
	for _, bad := range []string{strings.Replace(string(raw), `"revision":1`, `"revision":2`, 1), strings.Replace(string(raw), `"expected_head_revision":0`, `"expected_head_revision":1`, 1), strings.Replace(string(raw), `"target_read_set_root":"`+strings.Repeat("c", 64), `"target_read_set_root":"`+strings.Repeat("d", 64), 1), strings.TrimSuffix(string(raw), "}") + `,"prompt":"forbidden"}`, strings.TrimSuffix(string(raw), "}") + `,"execution_id":"duplicate"}`} {
		if _, err := domain.DecodeReferenceExecution([]byte(bad)); err == nil {
			t.Fatal("corrupt snapshot accepted")
		}
	}
	input.ReadSet.ExpectedHeadRevision = 1
	if _, err := domain.BuildInitialReferenceExecution(input); err == nil {
		t.Fatal("initial execution bypassed Head contract")
	}
	input.ReadSet.ExpectedHeadRevision = 0
	input.MembershipTokenVersion = 0
	if _, err := domain.BuildInitialReferenceExecution(input); err == nil {
		t.Fatal("execution omitted its original preparation actor epoch")
	}
}
