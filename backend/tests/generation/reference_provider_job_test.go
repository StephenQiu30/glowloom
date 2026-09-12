package generation_test

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	"github.com/google/uuid"
)

func TestReferenceProviderJobDeterministicCompleteCallSet(t *testing.T) {
	execution := domain.GenerationRevisionRef{ID: uuid.NewString(), Revision: 1, ContentHash: strings.Repeat("a", 64)}
	inputs := []domain.ReferenceProviderCallInput{}
	for bundle := 0; bundle < 2; bundle++ {
		for _, slot := range []string{"front", "profile", "back"} {
			inputs = append(inputs, domain.ReferenceProviderCallInput{BundleIndex: bundle, SlotKey: slot, CompiledRequestHash: strings.Repeat("b", 64)})
		}
	}
	job, calls, err := domain.BuildReferenceProviderJob(execution, inputs)
	if err != nil || len(calls) != 6 {
		t.Fatalf("build complete set: %v", err)
	}
	for i, call := range calls {
		raw, _ := json.Marshal([]any{execution, call.BundleIndex, call.SlotKey, call.CompiledRequestHash})
		key, _ := canonical.Hash(raw)
		if call.CallKey != key || job.CallKeys[i] != key {
			t.Fatal("call key lost canonical tuple identity")
		}
		encoded, _ := json.Marshal(call)
		decoded, err := domain.DecodeReferenceProviderCall(encoded)
		if err != nil || !reflect.DeepEqual(decoded, call) {
			t.Fatalf("call roundtrip: %v", err)
		}
	}
	rawKeys, _ := json.Marshal(job.CallKeys)
	root, _ := canonical.Hash(rawKeys)
	if root != job.CallSetRoot {
		t.Fatal("job root does not cover the complete set")
	}
	reordered := slices.Clone(inputs)
	slices.Reverse(reordered)
	same, sameCalls, err := domain.BuildReferenceProviderJob(execution, reordered)
	if err != nil || !reflect.DeepEqual(same, job) || !reflect.DeepEqual(sameCalls, calls) {
		t.Fatal("input order changed job identity")
	}
	raw, _ := json.Marshal(job)
	decoded, err := domain.DecodeReferenceProviderJob(raw)
	if err != nil || !reflect.DeepEqual(decoded, job) {
		t.Fatalf("job roundtrip: %v", err)
	}
	for _, bad := range []string{strings.TrimSuffix(string(raw), "}") + `,"prompt":"not allowed"}`, strings.TrimSuffix(string(raw), "}") + `,"call_keys":[]}`, strings.Replace(string(raw), job.CallSetRoot, strings.Repeat("f", 64), 1)} {
		if _, err := domain.DecodeReferenceProviderJob([]byte(bad)); err == nil {
			t.Fatal("corrupt job accepted")
		}
	}
	callRaw, _ := json.Marshal(calls[0])
	for _, bad := range []string{strings.Replace(string(callRaw), calls[0].CompiledRequestHash, strings.Repeat("c", 64), 1), strings.TrimSuffix(string(callRaw), "}") + `,"call_key":"duplicate"}`, strings.TrimSuffix(string(callRaw), "}") + `,"body":"forbidden"}`} {
		if _, err := domain.DecodeReferenceProviderCall([]byte(bad)); err == nil {
			t.Fatal("corrupt call accepted")
		}
	}
	for _, change := range []string{"execution_id", "execution_hash", "request_hash", "slot"} {
		ref := execution
		changed := slices.Clone(inputs)
		switch change {
		case "execution_id":
			ref.ID = uuid.NewString()
		case "execution_hash":
			ref.ContentHash = strings.Repeat("c", 64)
		case "request_hash":
			changed[0].CompiledRequestHash = strings.Repeat("c", 64)
		case "slot":
			changed[0].SlotKey = "detail"
			changed[3].SlotKey = "detail"
		}
		other, _, err := domain.BuildReferenceProviderJob(ref, changed)
		if err != nil || other.CallSetRoot == job.CallSetRoot || other.ContentHash == job.ContentHash {
			t.Fatalf("%s not bound: %v", change, err)
		}
	}
	for _, fault := range []string{"empty", "duplicate", "missing", "bundle_gap", "negative_bundle", "large_bundle", "invalid_slot", "invalid_hash", "too_many_slots", "wrong_revision"} {
		bad := slices.Clone(inputs)
		ref := execution
		switch fault {
		case "empty":
			bad = nil
		case "duplicate":
			bad = append(bad, bad[0])
		case "missing":
			bad = bad[:len(bad)-1]
		case "bundle_gap":
			for i := 3; i < len(bad); i++ {
				bad[i].BundleIndex = 2
			}
		case "negative_bundle":
			bad[0].BundleIndex = -1
		case "large_bundle":
			bad[0].BundleIndex = 4
		case "invalid_slot":
			bad[0].SlotKey = "bad slot"
		case "invalid_hash":
			bad[0].CompiledRequestHash = "bad"
		case "too_many_slots":
			bad = bad[:3]
			for _, slot := range []string{"left", "right"} {
				bad = append(bad, domain.ReferenceProviderCallInput{SlotKey: slot, CompiledRequestHash: strings.Repeat("b", 64)})
			}
		case "wrong_revision":
			ref.Revision = 2
		}
		got, gotCalls, err := domain.BuildReferenceProviderJob(ref, bad)
		if err == nil || !reflect.DeepEqual(got, domain.ReferenceProviderJob{}) || gotCalls != nil {
			t.Fatalf("%s accepted", fault)
		}
	}
}
