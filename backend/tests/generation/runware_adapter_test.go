package generation_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	runwareadapter "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/runware"
	generationapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
)

func TestRunwareSubmitUsesFrozenTargetAndStableProviderJobUUIDOnce(t *testing.T) {
	target := generationReferenceTarget(t)
	transport := &recordingRunwareTransport{responses: []runwareHTTPResponse{{
		status: http.StatusOK,
		body:   `{"data":[{"taskType":"imageInference","taskUUID":"90000000-0000-4000-8000-000000000009"}]}`,
	}}}
	gateway, err := runwareadapter.New(runwareadapter.Config{
		APIKey: "test-secret-must-not-leak", Client: &http.Client{Transport: transport},
		Stager: &recordingImageStager{}, RequestTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := gateway.Submit(context.Background(), runwareSubmission(target))
	if err != nil || outcome.Status != generationapp.ProviderOutcomeAccepted ||
		outcome.ProviderJobKey != "90000000-0000-4000-8000-000000000009" {
		t.Fatalf("Runware submit outcome = %#v err=%v", outcome, err)
	}
	requests := transport.snapshot()
	if len(requests) != 1 {
		t.Fatalf("Runware submit count = %d, want 1", len(requests))
	}
	request := requests[0]
	if request.URL != "https://api.runware.ai/v1" || request.Method != http.MethodPost ||
		request.Authorization != "Bearer test-secret-must-not-leak" {
		t.Fatalf("Runware HTTP boundary drifted: %#v", request.withoutAuthorization())
	}
	var tasks []map[string]any
	if err = json.Unmarshal(request.Body, &tasks); err != nil || len(tasks) != 1 {
		t.Fatalf("decode Runware request: %v body=%s", err, request.Body)
	}
	task := tasks[0]
	for key, want := range map[string]any{
		"taskType": "imageInference", "taskUUID": "90000000-0000-4000-8000-000000000009",
		"model": "runware:z-image@turbo", "deliveryMethod": "async", "outputFormat": "PNG",
		"positivePrompt": target.ReferenceAsset.PositivePrompt,
		"negativePrompt": target.ReferenceAsset.NegativePrompt,
		"width":          float64(1536), "height": float64(1024), "numberResults": float64(4),
	} {
		if task[key] != want {
			t.Errorf("Runware request %s = %#v, want %#v", key, task[key], want)
		}
	}
	if _, exists := task["credentialRef"]; exists || strings.Contains(string(request.Body), "test-secret-must-not-leak") {
		t.Fatal("Runware request leaked credential metadata or secret into the JSON body")
	}
}

func TestRunwareQueryStagesExactOutputsAndRejectsIdentityDrift(t *testing.T) {
	target := generationReferenceTarget(t)
	response := `{"data":[` + runwareImagesJSON("90000000-0000-4000-8000-000000000009") + `]}`
	transport := &recordingRunwareTransport{responses: []runwareHTTPResponse{{status: http.StatusOK, body: response}}}
	stager := &recordingImageStager{}
	gateway, err := runwareadapter.New(runwareadapter.Config{
		APIKey: "test-secret-must-not-leak", Client: &http.Client{Transport: transport},
		Stager: stager, RequestTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := gateway.Query(context.Background(), runwareSubmission(target))
	if err != nil || outcome.Status != generationapp.ProviderOutcomeSucceeded ||
		outcome.ProviderJobKey != "90000000-0000-4000-8000-000000000009" ||
		outcome.ActualUnits != 4 || len(outcome.Outputs) != 4 || len(outcome.ProviderEventID) != 64 {
		t.Fatalf("Runware query outcome = %#v err=%v", outcome, err)
	}
	staged := stager.snapshot()
	if len(staged) != 4 {
		t.Fatalf("staged Runware outputs = %d, want 4", len(staged))
	}
	for index, request := range staged {
		if request.ProviderJobID != "90000000-0000-4000-8000-000000000009" ||
			request.ImageURL != "https://im.runware.ai/image/"+request.ImageUUID+".png" ||
			request.Width != 1536 || request.Height != 1024 || request.OutputFormat != "PNG" {
			t.Errorf("staging request %d drifted: %#v", index, request)
		}
	}
	requests := transport.snapshot()
	var tasks []map[string]any
	if len(requests) != 1 || json.Unmarshal(requests[0].Body, &tasks) != nil || len(tasks) != 1 ||
		tasks[0]["taskType"] != "getResponse" ||
		tasks[0]["taskUUID"] != "90000000-0000-4000-8000-000000000009" {
		t.Fatalf("Runware query did not use the same task UUID: %#v", requests)
	}

	driftTransport := &recordingRunwareTransport{responses: []runwareHTTPResponse{{status: http.StatusOK, body: strings.ReplaceAll(
		response, "90000000-0000-4000-8000-000000000009", "90000000-0000-4000-8000-000000000099",
	)}}}
	driftGateway, err := runwareadapter.New(runwareadapter.Config{
		APIKey: "test-secret-must-not-leak", Client: &http.Client{Transport: driftTransport},
		Stager: &recordingImageStager{}, RequestTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = driftGateway.Query(context.Background(), runwareSubmission(target)); err == nil ||
		strings.Contains(err.Error(), "test-secret-must-not-leak") {
		t.Fatalf("Runware identity drift did not fail safely: %v", err)
	}
}

func TestRunwareTransportFailureIsReturnedWithoutAutomaticResubmitOrSecretLeak(t *testing.T) {
	transport := &recordingRunwareTransport{err: errors.New("connection reset after request write")}
	gateway, err := runwareadapter.New(runwareadapter.Config{
		APIKey: "test-secret-must-not-leak", Client: &http.Client{Transport: transport},
		Stager: &recordingImageStager{}, RequestTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = gateway.Submit(context.Background(), runwareSubmission(generationReferenceTarget(t))); err == nil ||
		strings.Contains(err.Error(), "test-secret-must-not-leak") {
		t.Fatalf("Runware transport failure was not sanitized: %v", err)
	}
	if calls := len(transport.snapshot()); calls != 1 {
		t.Fatalf("Runware POST was retried %d times, want exactly one call", calls)
	}
}

func TestRunwareQueryRecoversHistoricalResponseWithTheSameFrozenRequest(t *testing.T) {
	target := generationReferenceTarget(t)
	history := `{"data":[{"taskType":"getTaskDetails","taskUUID":"90000000-0000-4000-8000-000000000009",` +
		`"request":[{"taskType":"imageInference","taskUUID":"90000000-0000-4000-8000-000000000009",` +
		`"model":"runware:z-image@turbo","positivePrompt":"same character, front profile and back views",` +
		`"negativePrompt":"different identities, missing view, text, watermark","width":1536,"height":1024,` +
		`"numberResults":4,"outputFormat":"PNG","outputType":"URL","deliveryMethod":"async"}],` +
		`"response":{"data":[` + runwareImagesJSON("90000000-0000-4000-8000-000000000009") + `]}}]}`
	transport := &recordingRunwareTransport{responses: []runwareHTTPResponse{
		{status: http.StatusOK, body: `{"data":[]}`},
		{status: http.StatusOK, body: history},
	}}
	gateway, err := runwareadapter.New(runwareadapter.Config{
		APIKey: "test-secret-must-not-leak", Client: &http.Client{Transport: transport},
		Stager: &recordingImageStager{}, RequestTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := gateway.Query(context.Background(), runwareSubmission(target))
	if err != nil || outcome.Status != generationapp.ProviderOutcomeSucceeded || len(outcome.Outputs) != 4 {
		t.Fatalf("recover Runware task details: outcome=%#v err=%v", outcome, err)
	}
	requests := transport.snapshot()
	if len(requests) != 2 {
		t.Fatalf("Runware historical recovery calls = %d, want getResponse + getTaskDetails", len(requests))
	}
	var lookup []map[string]any
	if json.Unmarshal(requests[1].Body, &lookup) != nil || len(lookup) != 1 ||
		lookup[0]["taskType"] != "getTaskDetails" ||
		lookup[0]["taskUUID"] != "90000000-0000-4000-8000-000000000009" {
		t.Fatalf("Runware historical lookup changed task identity: %s", requests[1].Body)
	}

	driftedHistory := strings.Replace(history, `"width":1536`, `"width":1024`, 1)
	driftTransport := &recordingRunwareTransport{responses: []runwareHTTPResponse{
		{status: http.StatusOK, body: `{"data":[]` + `}`},
		{status: http.StatusOK, body: driftedHistory},
	}}
	driftGateway, err := runwareadapter.New(runwareadapter.Config{
		APIKey: "test-secret-must-not-leak", Client: &http.Client{Transport: driftTransport},
		Stager: &recordingImageStager{}, RequestTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = driftGateway.Query(context.Background(), runwareSubmission(target)); err == nil {
		t.Fatal("Runware task details with a drifted original request were accepted")
	}
}

func TestRunwareQueryKeepsTaskNotFoundUnknownAndMapsExplicitProviderFailure(t *testing.T) {
	target := generationReferenceTarget(t)
	transport := &recordingRunwareTransport{responses: []runwareHTTPResponse{
		{status: http.StatusOK, body: `{"errors":[{"code":"taskNotFound","taskUUID":"90000000-0000-4000-8000-000000000009"}]}`},
		{status: http.StatusOK, body: `{"errors":[` +
			`{"code":"timeoutProvider","taskUUID":"90000000-0000-4000-8000-000000000009"},` +
			`{"code":"contentRejected","taskUUID":"90000000-0000-4000-8000-000000000009"}]}`},
	}}
	gateway, err := runwareadapter.New(runwareadapter.Config{
		APIKey: "test-secret-must-not-leak", Client: &http.Client{Transport: transport},
		Stager: &recordingImageStager{}, RequestTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	unknown, err := gateway.Query(context.Background(), runwareSubmission(target))
	if err != nil || unknown.Status != generationapp.ProviderOutcomeUnknown ||
		unknown.ProviderJobKey != "90000000-0000-4000-8000-000000000009" {
		t.Fatalf("taskNotFound must retain reservations as unknown: %#v err=%v", unknown, err)
	}
	failed, err := gateway.Query(context.Background(), runwareSubmission(target))
	if err != nil || failed.Status != generationapp.ProviderOutcomeFailed ||
		failed.FailureCode != "runware_contentrejected" || len(failed.ProviderEventID) != 64 {
		t.Fatalf("explicit Runware failure was not normalized: %#v err=%v", failed, err)
	}
}

func TestRunwareQueryKeepsProcessingAndPartialResultsAcceptedWithoutStaging(t *testing.T) {
	target := generationReferenceTarget(t)
	transport := &recordingRunwareTransport{responses: []runwareHTTPResponse{{
		status: http.StatusOK,
		body: `{"data":[` +
			`{"taskType":"imageInference","taskUUID":"90000000-0000-4000-8000-000000000009",` +
			`"status":"success","imageUUID":"a0000000-0000-4000-8000-000000000001",` +
			`"imageURL":"https://im.runware.ai/image/a0000000-0000-4000-8000-000000000001.png"},` +
			`{"taskType":"imageInference","taskUUID":"90000000-0000-4000-8000-000000000009",` +
			`"status":"processing"}]}`,
	}}}
	stager := &recordingImageStager{}
	gateway, err := runwareadapter.New(runwareadapter.Config{
		APIKey: "test-secret-must-not-leak", Client: &http.Client{Transport: transport},
		Stager: stager, RequestTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := gateway.Query(context.Background(), runwareSubmission(target))
	if err != nil || outcome.Status != generationapp.ProviderOutcomeAccepted ||
		outcome.ProviderJobKey != "90000000-0000-4000-8000-000000000009" {
		t.Fatalf("processing Runware result must remain accepted: %#v err=%v", outcome, err)
	}
	if staged := stager.snapshot(); len(staged) != 0 {
		t.Fatalf("partial Runware result staged %d outputs before the candidate set completed", len(staged))
	}
}

func generationReferenceTarget(t *testing.T) domain.GenerationTarget {
	t.Helper()
	target, err := domain.NewGenerationTarget(domain.GenerationTargetInput{
		ID: "10000000-0000-4000-8000-000000000001", WorkspaceID: "20000000-0000-4000-8000-000000000002",
		ProjectID: "30000000-0000-4000-8000-000000000003", Kind: domain.GenerationTargetReferenceAsset,
		SourceOwnerRef: domain.FrozenOwnerReference{
			Owner: "storyboard", Resource: "approved_storyboard_intents",
			ID: "40000000-0000-4000-8000-000000000004", Revision: 1, ContentHash: generationHash("1"),
		},
		PolicySnapshotRef: domain.FrozenOwnerReference{
			Owner: "preset", Resource: "effective_style_snapshot",
			ID: "30000000-0000-4000-8000-000000000003", Revision: 3, ContentHash: generationHash("2"),
		},
		ReferenceAsset: &domain.ReferenceAssetTarget{
			AssetID: "50000000-0000-4000-8000-000000000005", AssetKind: "character",
			SpecificationVersionRef: domain.FrozenOwnerReference{
				Owner: "production", Resource: "production_bible_specification_version",
				ID: "60000000-0000-4000-8000-000000000006", Revision: 1, ContentHash: generationHash("3"),
			},
			AssetStateRef: domain.FrozenOwnerReference{
				Owner: "asset", Resource: "asset_state", ID: "70000000-0000-4000-8000-000000000007",
				Revision: 1, ContentHash: generationHash("4"),
			},
			OutputKind: "reference_sheet", RequiredViewRoles: []string{"front", "profile", "back"},
			PromptVersion: "character-reference-sheet-v1", PositivePrompt: "same character, front profile and back views",
			NegativePrompt: "different identities, missing view, text, watermark",
			Width:          1536, Height: 1024, NumberResults: 4, OutputFormat: "PNG",
		},
		Revision: 1, CreatedBy: "80000000-0000-4000-8000-000000000008",
		CreatedAt: time.Date(2026, 8, 29, 9, 30, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	return target
}

func runwareSubmission(target domain.GenerationTarget) generationapp.ProviderSubmission {
	return generationapp.ProviderSubmission{
		WorkspaceID: target.WorkspaceID, ProjectID: target.ProjectID,
		ProviderJobID: "90000000-0000-4000-8000-000000000009",
		RequestID:     "91000000-0000-4000-8000-000000000010", RequestKey: "reference-asset-request",
		IntentID: "92000000-0000-4000-8000-000000000011", ProviderKey: "runware",
		ModelKey: "runware:z-image@turbo", CredentialRef: "env/runware_api_key",
		TargetHash: target.TargetHash, Units: 4, Target: target,
	}
}

func runwareImagesJSON(taskUUID string) string {
	items := make([]string, 4)
	for index := range items {
		imageUUID := "a0000000-0000-4000-8000-00000000000" + string(rune('1'+index))
		items[index] = `{"taskType":"imageInference","taskUUID":"` + taskUUID +
			`","imageUUID":"` + imageUUID + `","imageURL":"https://im.runware.ai/image/` + imageUUID + `.png"}`
	}
	return strings.Join(items, ",")
}

type runwareHTTPResponse struct {
	status int
	body   string
}

type recordedRunwareRequest struct {
	Method, URL, Authorization string
	Body                       []byte
}

func (request recordedRunwareRequest) withoutAuthorization() recordedRunwareRequest {
	request.Authorization = "<redacted>"
	return request
}

type recordingRunwareTransport struct {
	mu        sync.Mutex
	requests  []recordedRunwareRequest
	responses []runwareHTTPResponse
	err       error
}

func (transport *recordingRunwareTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	body, _ := io.ReadAll(request.Body)
	transport.mu.Lock()
	transport.requests = append(transport.requests, recordedRunwareRequest{
		Method: request.Method, URL: request.URL.String(), Authorization: request.Header.Get("Authorization"), Body: body,
	})
	index := len(transport.requests) - 1
	transport.mu.Unlock()
	if transport.err != nil {
		return nil, transport.err
	}
	response := transport.responses[index]
	return &http.Response{
		StatusCode: response.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(response.body)),
		Request: request,
	}, nil
}

func (transport *recordingRunwareTransport) snapshot() []recordedRunwareRequest {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	return append([]recordedRunwareRequest(nil), transport.requests...)
}

type recordingImageStager struct {
	mu       sync.Mutex
	requests []runwareadapter.StageImageRequest
}

func (stager *recordingImageStager) StageImage(_ context.Context, request runwareadapter.StageImageRequest) (generationapp.ProviderOutput, error) {
	stager.mu.Lock()
	stager.requests = append(stager.requests, request)
	stager.mu.Unlock()
	return generationapp.ProviderOutput{
		OutputKey:        request.ImageUUID,
		StagingObjectKey: "staging/" + request.WorkspaceID + "/" + request.ProviderJobID + "/" + request.ImageUUID + ".png",
		SHA256:           generationHash("a"), Bytes: 1024, MediaType: "image/png",
		Width: request.Width, Height: request.Height,
	}, nil
}

func (stager *recordingImageStager) snapshot() []runwareadapter.StageImageRequest {
	stager.mu.Lock()
	defer stager.mu.Unlock()
	return append([]runwareadapter.StageImageRequest(nil), stager.requests...)
}
