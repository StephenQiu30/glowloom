package operation_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	operationflow "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/workflow"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/application"
)

const syncCompletedReceiptJSON = `{
	"version": 1,
	"identity": {
		"project_id": "11111111-1111-4111-8111-111111111111",
		"operation_id": "22222222-2222-4222-8222-222222222222",
		"action": "submit",
		"attempt": 1,
		"request_key": "operation/22222222-2222-4222-8222-222222222222",
		"model_profile_version_id": "33333333-3333-4333-8333-333333333333",
		"price_rule_version_id": "44444444-4444-4444-8444-444444444444"
	},
	"manifest_sha256": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	"outputs": [{
		"sequence": 1,
		"size_bytes": 100,
		"mime_type": "image/png",
		"sha256": "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	}]
}`

func syncCompletedCallJSON(receipt string) string {
	return `{"operation_id":"22222222-2222-4222-8222-222222222222",` +
		`"action":"submit","attempt":1,"outcome":"ok","state":"completed",` +
		`"receipt":` + receipt + `}`
}

func TestM1SyncCompletedContract(t *testing.T) {
	for _, usage := range []json.RawMessage{nil, json.RawMessage(`{"output_count":0}`), json.RawMessage(`{"output_count":1}`)} {
		input := decodedSyncCompletedCall(t)
		input.Usage = usage
		if err := input.Validate(); err != nil {
			t.Fatalf("synchronous completion with a bound receipt must be accepted: %v", err)
		}
	}
}

func TestM1ReceiptStrictJSON(t *testing.T) {
	tests := []struct {
		name    string
		receipt string
	}{
		{"duplicate top-level field", strings.Replace(syncCompletedReceiptJSON, `"version": 1`, `"version": 1, "version": 1`, 1)},
		{"duplicate identity field", strings.Replace(syncCompletedReceiptJSON, `"action": "submit"`, `"action": "submit", "action": "submit"`, 1)},
		{"duplicate output field", strings.Replace(syncCompletedReceiptJSON, `"sequence": 1`, `"sequence": 1, "sequence": 1`, 1)},
		{"unknown top-level field", strings.Replace(syncCompletedReceiptJSON, `"version": 1`, `"version": 1, "url": "https://invalid.example/private"`, 1)},
		{"unknown identity field", strings.Replace(syncCompletedReceiptJSON, `"action": "submit"`, `"action": "submit", "secret": "private-marker"`, 1)},
		{"unknown output field", strings.Replace(syncCompletedReceiptJSON, `"sequence": 1`, `"sequence": 1, "object_key": "private-marker"`, 1)},
		{"noncanonical top-level field", strings.Replace(syncCompletedReceiptJSON, `"version"`, `"Version"`, 1)},
		{"noncanonical identity field", strings.Replace(syncCompletedReceiptJSON, `"action"`, `"Action"`, 1)},
		{"noncanonical output field", strings.Replace(syncCompletedReceiptJSON, `"sequence"`, `"Sequence"`, 1)},
		{"over capacity", "{" + strings.Repeat(" ", 4096) + syncCompletedReceiptJSON[1:]},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var input application.CompleteProviderCallInput
			err := json.Unmarshal([]byte(syncCompletedCallJSON(test.receipt)), &input)
			if err == nil {
				t.Fatal("receipt decoder accepted an ambiguous or open payload")
			}
			if strings.Contains(err.Error(), "private-marker") || strings.Contains(err.Error(), "invalid.example") {
				t.Fatal("receipt decoding error exposed untrusted content")
			}
		})
	}
}

func TestM1ReceiptRejectsInvalidEvidence(t *testing.T) {
	for _, test := range []struct {
		name string
		old  string
		new  string
	}{
		{"zero project", "11111111-1111-4111-8111-111111111111", "00000000-0000-0000-0000-000000000000"},
		{"zero operation", "22222222-2222-4222-8222-222222222222", "00000000-0000-0000-0000-000000000000"},
		{"zero model version", "33333333-3333-4333-8333-333333333333", "00000000-0000-0000-0000-000000000000"},
		{"zero price version", "44444444-4444-4444-8444-444444444444", "00000000-0000-0000-0000-000000000000"},
		{"wrong action", `"action": "submit"`, `"action": "query"`},
		{"zero attempt", `"attempt": 1`, `"attempt": 0`},
		{"empty request key", `"request_key": "operation/22222222-2222-4222-8222-222222222222"`, `"request_key": ""`},
		{"untrimmed request key", `"request_key": "operation/22222222-2222-4222-8222-222222222222"`, `"request_key": " request "`},
		{"overlong request key", `"request_key": "operation/22222222-2222-4222-8222-222222222222"`, `"request_key": "` + strings.Repeat("r", 257) + `"`},
		{"unsupported version", `"version": 1`, `"version": 2`},
		{"noncanonical manifest digest", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", "0123456789ABCDEF0123456789abcdef0123456789abcdef0123456789abcdef"},
		{"short manifest digest", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", "abcdef"},
		{"nonhex output digest", "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789", strings.Repeat("z", 64)},
		{"wrong sequence", `"sequence": 1`, `"sequence": 2`},
		{"zero output size", `"size_bytes": 100`, `"size_bytes": 0`},
		{"negative output size", `"size_bytes": 100`, `"size_bytes": -1`},
		{"oversized output", `"size_bytes": 100`, `"size_bytes": 33554433`},
		{"invalid MIME", `"mime_type": "image/png"`, `"mime_type": "text/html"`},
		{"null output", `"outputs": [{`, `"outputs": [null, {`},
	} {
		t.Run(test.name, func(t *testing.T) {
			raw := strings.Replace(syncCompletedReceiptJSON, test.old, test.new, 1)
			var receipt application.ProviderReceipt
			if err := json.Unmarshal([]byte(raw), &receipt); !errors.Is(err, application.ErrInvalidProviderCall) {
				t.Fatalf("invalid receipt evidence must be rejected, got %v", err)
			}
		})
	}
}

func TestM1ReceiptValidateBoundaries(t *testing.T) {
	for _, mime := range []string{"image/png", "image/jpeg", "image/webp"} {
		for _, size := range []int64{1, 32 * 1024 * 1024} {
			receipt := *decodedSyncCompletedCall(t).Receipt
			receipt.Outputs[0].MIMEType, receipt.Outputs[0].SizeBytes = mime, size
			if err := receipt.Validate(); err != nil {
				t.Fatalf("valid MIME and boundary size rejected: %v", err)
			}
		}
	}
	for _, test := range []struct {
		name   string
		modify func(*application.ProviderReceipt)
	}{
		{"missing outputs", func(r *application.ProviderReceipt) { r.Outputs = nil }},
		{"multiple outputs", func(r *application.ProviderReceipt) { r.Outputs = append(r.Outputs, r.Outputs[0]) }},
		{"bad output digest", func(r *application.ProviderReceipt) { r.Outputs[0].SHA256 = "invalid" }},
		{"bad manifest digest", func(r *application.ProviderReceipt) { r.ManifestSHA256 = "invalid" }},
		{"bad output MIME", func(r *application.ProviderReceipt) { r.Outputs[0].MIMEType = "image/svg+xml" }},
		{"bad identity", func(r *application.ProviderReceipt) { r.Identity.Action = "cancel" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			receipt := *decodedSyncCompletedCall(t).Receipt
			test.modify(&receipt)
			if err := receipt.Validate(); !errors.Is(err, application.ErrInvalidProviderCall) {
				t.Fatalf("directly constructed invalid receipt accepted: %v", err)
			}
		})
	}
}

func TestM1CompletedCallBindsReceipt(t *testing.T) {
	for _, test := range []struct {
		name   string
		modify func(*application.CompleteProviderCallInput)
	}{
		{"missing receipt", func(i *application.CompleteProviderCallInput) { i.Receipt = nil }},
		{"foreign operation", func(i *application.CompleteProviderCallInput) {
			i.Receipt.Identity.OperationID = i.Receipt.Identity.ProjectID
		}},
		{"wrong attempt", func(i *application.CompleteProviderCallInput) { i.Receipt.Identity.Attempt++ }},
		{"wrong action", func(i *application.CompleteProviderCallInput) { i.Action = "query" }},
		{"not successful", func(i *application.CompleteProviderCallInput) { i.Outcome = "error" }},
		{"fake task ID", func(i *application.CompleteProviderCallInput) { taskID := "local-receipt"; i.ProviderTaskID = &taskID }},
		{"receipt on accepted", func(i *application.CompleteProviderCallInput) {
			i.State = "accepted"
			taskID := "real-task"
			i.ProviderTaskID = &taskID
		}},
		{"receipt on unknown", func(i *application.CompleteProviderCallInput) { i.State, i.Outcome = "unknown", "unknown" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := decodedSyncCompletedCall(t)
			test.modify(&input)
			if err := input.Validate(); !errors.Is(err, application.ErrInvalidProviderCall) {
				t.Fatalf("unbound completion evidence accepted: %v", err)
			}
		})
	}
}

func TestM1BeginProviderCallDispatchMarker(t *testing.T) {
	operationID := decodedSyncCompletedCall(t).OperationID
	for _, test := range []struct {
		action           string
		dispatchRequired bool
		wantError        bool
	}{
		{"submit", false, false},
		{"submit", true, false},
		{"query", false, false},
		{"query", true, true},
		{"cancel", false, false},
		{"cancel", true, true},
	} {
		input := application.BeginProviderCallInput{
			OperationID: operationID, Action: test.action, Attempt: 1, DispatchRequired: test.dispatchRequired,
		}
		if test.action == "cancel" {
			taskID := "real-task"
			input.ProviderTaskID = &taskID
		}
		if err := input.Validate(); (err != nil) != test.wantError {
			t.Errorf("action=%s dispatch=%t error=%v, wantError=%t", test.action, test.dispatchRequired, err, test.wantError)
		}
	}
}

func TestM1DispatchMarkerRejectsExistingTask(t *testing.T) {
	taskID := "already-accepted-task"
	input := application.BeginProviderCallInput{
		OperationID: decodedSyncCompletedCall(t).OperationID, Action: "submit", Attempt: 1,
		DispatchRequired: true, ProviderTaskID: &taskID,
	}
	if err := input.Validate(); !errors.Is(err, application.ErrInvalidProviderCall) {
		t.Fatalf("an existing upstream task must not gain new sending ownership: %v", err)
	}
	input.DispatchRequired = false
	if err := input.Validate(); err != nil {
		t.Fatalf("legacy submit identity validation changed: %v", err)
	}
}

func TestM1DispatchIdentityValidate(t *testing.T) {
	identity := decodedSyncCompletedCall(t).Receipt.Identity
	for _, key := range []string{"r", strings.Repeat("r", 256)} {
		identity.RequestKey = key
		if err := identity.Validate(); err != nil {
			t.Fatalf("valid request-key boundary rejected: %v", err)
		}
	}
	for _, key := range []string{"", " r", "r ", strings.Repeat("r", 257), strings.Repeat("图", 86)} {
		identity.RequestKey = key
		if err := identity.Validate(); !errors.Is(err, application.ErrInvalidProviderCall) {
			t.Fatalf("invalid request-key boundary accepted: %v", err)
		}
	}
}

func TestM1SubmitDTOHistoricalJSON(t *testing.T) {
	legacyOutput := `{"outcome":"accepted","provider_task_id":"real-task","error":null}`
	var output operationflow.ProviderSubmitOutput
	if err := json.Unmarshal([]byte(legacyOutput), &output); err != nil {
		t.Fatalf("decode historical accepted: %v", err)
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("encode historical accepted: %v", err)
	}
	var expected, actual map[string]any
	if err := json.Unmarshal([]byte(legacyOutput), &expected); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, &actual); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(expected, actual) {
		t.Fatal("historical accepted gained or lost JSON fields")
	}
	output = operationflow.ProviderSubmitOutput{
		Outcome: operationflow.ProviderSubmitCompleted,
		Receipt: decodedSyncCompletedCall(t).Receipt,
		Usage:   json.RawMessage(`{"output_count":1}`),
	}
	encoded, err = json.Marshal(output)
	if err != nil {
		t.Fatalf("encode synchronous output: %v", err)
	}
	var decoded operationflow.ProviderSubmitOutput
	if err := json.Unmarshal(encoded, &decoded); err != nil || !reflect.DeepEqual(output, decoded) {
		t.Fatalf("synchronous output did not round trip: %v", err)
	}
	if output.ProviderTaskID != nil {
		t.Fatal("synchronous output fabricated a task ID")
	}
	// Validate still requires a provider-issued ID on the asynchronous path.
	accepted := decodedSyncCompletedCall(t)
	accepted.State, accepted.Receipt = "accepted", nil
	if err := accepted.Validate(); !errors.Is(err, application.ErrInvalidProviderCall) {
		t.Fatalf("accepted without real task ID was allowed: %v", err)
	}
	taskID := "real-task"
	accepted.ProviderTaskID = &taskID
	accepted.Usage = json.RawMessage(`{"legacy_field":"preserved"}`)
	if err := accepted.Validate(); err != nil {
		t.Fatalf("historical accepted validation changed: %v", err)
	}
}

func decodedSyncCompletedCall(t *testing.T) application.CompleteProviderCallInput {
	t.Helper()
	var input application.CompleteProviderCallInput
	if err := json.Unmarshal([]byte(syncCompletedCallJSON(syncCompletedReceiptJSON)), &input); err != nil {
		t.Fatalf("decode completed call: %v", err)
	}
	return input
}

func TestM1CompletedUsageIsNormalized(t *testing.T) {
	for _, usage := range []string{
		`{"secret":"private-marker"}`,
		`{"output_count":1,"output_count":1}`,
		`{"output_count":-1}`,
		`{"output_count":null}`,
		`{"output_count":1.5}`,
		`{}`,
	} {
		var input application.CompleteProviderCallInput
		if err := json.Unmarshal([]byte(syncCompletedCallJSON(syncCompletedReceiptJSON)), &input); err != nil {
			t.Fatalf("decode completed call: %v", err)
		}
		input.Usage = json.RawMessage(usage)
		if err := input.Validate(); !errors.Is(err, application.ErrInvalidProviderCall) {
			t.Errorf("untrusted completed usage must be rejected safely, got %v", err)
		}
	}
}
