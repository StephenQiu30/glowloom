package agent_test

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/agent/grant"
)

func TestVisionReviewInvocationRejectsRehashedDrift(t *testing.T) {
	for name, mutate := range map[string]func(*contract.VisionReviewInvocation){
		"nil identity":          func(v *contract.VisionReviewInvocation) { v.InvocationID = "00000000-0000-0000-0000-000000000000" },
		"noncanonical identity": func(v *contract.VisionReviewInvocation) { v.AttemptID = "00000000000000000000000000000690" },
		"bundle": func(v *contract.VisionReviewInvocation) {
			v.Payload.Scope.BundleInputID = "00000000-0000-4000-8000-000000000099"
		},
		"nil manifest": func(v *contract.VisionReviewInvocation) {
			v.Payload.Shard.ManifestID = "00000000-0000-0000-0000-000000000000"
		},
		"shard":    func(v *contract.VisionReviewInvocation) { v.Payload.Shard.ShardKey = "vision_bundle:other" },
		"variant":  func(v *contract.VisionReviewInvocation) { v.Payload.Variant.StageKey = "compile_reference_brief" },
		"release":  func(v *contract.VisionReviewInvocation) { v.StageRelease.StageReleaseHash = strings.Repeat("e", 64) },
		"calls":    func(v *contract.VisionReviewInvocation) { v.Budget.MaxModelCalls = 2 },
		"deadline": func(v *contract.VisionReviewInvocation) { v.Budget.MaxExecutionSeconds = 121 },
		"output":   func(v *contract.VisionReviewInvocation) { v.Budget.MaxOutputBytes = 131073 },
		"content": func(v *contract.VisionReviewInvocation) {
			v.Payload.StageInput.Subject.InputHash = strings.Repeat("f", 64)
		},
		"attachments": func(v *contract.VisionReviewInvocation) { v.Payload.StageInput.Attachments = nil },
	} {
		t.Run(name, func(t *testing.T) {
			value := validVisionReviewInvocation(t)
			mutate(&value)
			value.InputHash, _ = value.ComputeInputHash()
			if value.Validate() == nil {
				t.Fatal("rehashed invalid invocation accepted")
			}
		})
	}
}

func TestVisionReviewResultRejectsRehashedIdentityAndSemanticDrift(t *testing.T) {
	fixture := readVisionReviewWireFixture(t)
	invocation := validVisionReviewInvocation(t)
	for name, mutate := range map[string]func(*contract.VisionReviewAttemptResult){
		"attempt":         func(v *contract.VisionReviewAttemptResult) { v.AttemptID = "00000000-0000-4000-8000-000000000099" },
		"input hash":      func(v *contract.VisionReviewAttemptResult) { v.InputHash = strings.Repeat("e", 64) },
		"release":         func(v *contract.VisionReviewAttemptResult) { v.StageRelease.StageReleaseHash = strings.Repeat("e", 64) },
		"claim":           func(v *contract.VisionReviewAttemptResult) { v.ClaimVersion++ },
		"authorization":   func(v *contract.VisionReviewAttemptResult) { v.DispatchAuthorizationHash = strings.Repeat("e", 64) },
		"runtime":         func(v *contract.VisionReviewAttemptResult) { v.Executor.RuntimeClass = "text" },
		"automatic retry": func(v *contract.VisionReviewAttemptResult) { v.Status = "outcome_unknown" },
		"subject": func(v *contract.VisionReviewAttemptResult) {
			candidate, _, err := contract.DecodeVisionReviewCandidate(v.Candidate)
			if err != nil {
				t.Fatal(err)
			}
			candidate.Subject.InputHash = strings.Repeat("e", 64)
			v.Candidate = mustJSON(t, candidate)
			hash, err := contract.ProductionCanonicalHash(v.Candidate)
			if err != nil {
				t.Fatal(err)
			}
			v.OutputHash = &hash
		},
	} {
		t.Run(name, func(t *testing.T) {
			result, err := contract.DecodeVisionReviewAttemptResult(fixture.AcceptedResult)
			if err != nil {
				t.Fatal(err)
			}
			mutate(&result)
			result.ResultHash, err = result.ComputeResultHash()
			if err != nil {
				t.Fatal(err)
			}
			if result.ValidateFor(invocation, 1, fixture.Authorization.Hash) == nil {
				t.Fatal("invalid rehashed result accepted")
			}
		})
	}
}

func TestVisionReviewWireRejectsMissingUnknownAndDuplicateFields(t *testing.T) {
	fixture := readVisionReviewWireFixture(t)
	for _, name := range []string{"unknown", "missing zero", "null zero", "duplicate"} {
		t.Run(name, func(t *testing.T) {
			var value map[string]any
			if err := json.Unmarshal(fixture.Invocation, &value); err != nil {
				t.Fatal(err)
			}
			control := value["control"].(map[string]any)
			switch name {
			case "unknown":
				value["provider"] = "unexpected"
			case "missing zero":
				delete(control, "release_fence")
			case "null zero":
				control["release_fence"] = nil
			}
			raw := mustJSON(t, value)
			if name == "duplicate" {
				raw = append([]byte(`{"input_hash":"`+value["input_hash"].(string)+`",`), raw[1:]...)
			}
			if _, err := contract.DecodeVisionReviewInvocation(raw); err == nil {
				t.Fatal("noncanonical invocation accepted")
			}
		})
	}
	for _, status := range []string{"rejected", "outcome_unknown"} {
		t.Run(status, func(t *testing.T) {
			result, err := contract.DecodeVisionReviewAttemptResult(fixture.AcceptedResult)
			if err != nil {
				t.Fatal(err)
			}
			result.Status = status
			result.Candidate = nil
			result.OutputHash = nil
			retry := "never"
			if status == "outcome_unknown" {
				retry = "same_release"
			}
			result.Error = &contract.SceneAnalysisResultError{Code: "execution_failed", SafeSummary: "执行未取得可信结果", RetryClass: retry}
			result.ResultHash, err = result.ComputeResultHash()
			if err != nil {
				t.Fatal(err)
			}
			if result.ValidateFor(validVisionReviewInvocation(t), 1, fixture.Authorization.Hash) != nil {
				t.Fatal("valid failure rejected")
			}
			var value map[string]any
			if err = json.Unmarshal(mustJSON(t, result), &value); err != nil {
				t.Fatal(err)
			}
			delete(value, "output_hash")
			if _, err = contract.DecodeVisionReviewAttemptResult(mustJSON(t, value)); err == nil {
				t.Fatal("missing explicit nullable field accepted")
			}
		})
	}
}

type visionReviewWireFixture struct {
	Invocation       json.RawMessage `json:"invocation"`
	AcceptedResult   json.RawMessage `json:"accepted_result"`
	StageInstanceKey string          `json:"stage_instance_key"`
	TestSecret       string          `json:"test_secret"`
	Authorization    struct {
		Value        string                                            `json:"-"`
		Claims       contract.SceneAnalysisDispatchAuthorizationClaims `json:"claims"`
		SignatureHex string                                            `json:"signature_hex"`
		Hash         string                                            `json:"hash"`
		ClaimVersion int64                                             `json:"claim_version"`
		ExpiresAt    time.Time                                         `json:"expires_at"`
	} `json:"authorization"`
}

func readVisionReviewWireFixture(t *testing.T) visionReviewWireFixture {
	t.Helper()
	raw, err := os.ReadFile("testdata/vision_review_wire.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture visionReviewWireFixture
	if err = json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(fixture.Authorization.Claims)
	if err != nil {
		t.Fatal(err)
	}
	signature, err := hex.DecodeString(fixture.Authorization.SignatureHex)
	if err != nil {
		t.Fatal(err)
	}
	fixture.Authorization.Value = base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(signature)
	return fixture
}

func TestVisionReviewWireSharedGoldenAndSignedAuthorization(t *testing.T) {
	fixture := readVisionReviewWireFixture(t)
	value, err := contract.DecodeVisionReviewInvocation(fixture.Invocation)
	if err != nil {
		t.Fatal(err)
	}
	wantHash, err := contract.ProductionCanonicalHash(mustJSON(t, validVisionReviewInvocation(t)))
	if err != nil {
		t.Fatal(err)
	}
	gotHash, err := contract.ProductionCanonicalHash(fixture.Invocation)
	if err != nil || gotHash != wantHash || value.StageInstanceKey() != fixture.StageInstanceKey {
		t.Fatal("cross-language invocation drift")
	}
	now := time.Unix(100, 0)
	signer, err := grant.NewSigner(fixture.TestSecret, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	auth, err := signer.IssueVisionReviewDispatchAuthorization(value, 1)
	if err != nil || auth.Value != fixture.Authorization.Value || auth.Hash != fixture.Authorization.Hash || auth.ClaimVersion != fixture.Authorization.ClaimVersion || !auth.ExpiresAt.Equal(fixture.Authorization.ExpiresAt) {
		t.Fatalf("shared signature differs: %v", err)
	}
	if err = signer.VerifyVisionReviewDispatchAuthorization(auth.Value, value, 1); err != nil {
		t.Fatal(err)
	}
	result, err := contract.DecodeVisionReviewAttemptResult(fixture.AcceptedResult)
	if err != nil || result.ValidateFor(value, 1, auth.Hash) != nil {
		t.Fatalf("cross-language result rejected: %v", err)
	}
	for _, mutate := range []func(*contract.VisionReviewInvocation){
		func(v *contract.VisionReviewInvocation) { v.AttemptID = "00000000-0000-4000-8000-000000000099" },
		func(v *contract.VisionReviewInvocation) {
			v.StageRelease.AgentImageDigest = "sha256:" + strings.Repeat("f", 64)
		},
		func(v *contract.VisionReviewInvocation) { v.Control.ReleaseFence++ },
		func(v *contract.VisionReviewInvocation) { v.Budget.MaxExecutionSeconds-- },
	} {
		changed := validVisionReviewInvocation(t)
		mutate(&changed)
		changed.InputHash, _ = changed.ComputeInputHash()
		if changed.Validate() != nil {
			t.Fatal("mutation must be a valid, distinct invocation")
		}
		if signer.VerifyVisionReviewDispatchAuthorization(auth.Value, changed, 1) == nil || result.ValidateFor(changed, 1, auth.Hash) == nil {
			t.Fatal("authorization/result reused across invocation")
		}
	}
	if signer.VerifyVisionReviewDispatchAuthorization(auth.Value, value, 2) == nil {
		t.Fatal("claim version drift accepted")
	}
	if signer.VerifyVisionReviewDispatchAuthorization(auth.Value+"x", value, 1) == nil {
		t.Fatal("tampered signature accepted")
	}
	now = time.Unix(99, 0)
	if signer.VerifyVisionReviewDispatchAuthorization(auth.Value, value, 1) == nil {
		t.Fatal("authorization exceeding maximum TTL accepted")
	}
	if _, err = signer.IssueVisionReviewDispatchAuthorization(value, 0); err == nil {
		t.Fatal("zero claim accepted")
	}
	now = time.Unix(400, 0)
	if signer.VerifyVisionReviewDispatchAuthorization(auth.Value, value, 1) == nil {
		t.Fatal("expired authorization accepted")
	}
}
