package openapi_test

import (
	"encoding/json"
	"regexp"
	"slices"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/api/openapi"
)

func TestDocumentIsThePublicAPIContract(t *testing.T) {
	var document struct {
		OpenAPI    string                     `json:"openapi"`
		Paths      map[string]json.RawMessage `json:"paths"`
		Components struct {
			Schemas map[string]json.RawMessage `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(openapi.Document(), &document); err != nil {
		t.Fatalf("decode OpenAPI document: %v", err)
	}
	if document.OpenAPI != "3.1.0" {
		t.Fatalf("openapi version = %q, want 3.1.0", document.OpenAPI)
	}
	for _, path := range []string{
		"/api/auth/register",
		"/api/projects",
		"/api/projects/{project_id}/cost-budget",
		"/api/projects/{project_id}/media-model-profiles/{profile_version_id}/cost-price",
		"/api/projects/{project_id}/current-script-document",
		"/api/projects/{project_id}/script-sources",
		"/api/projects/{project_id}/script-sources/{revision_id}",
		"/api/projects/{project_id}/scene-analysis-candidates/{candidate_id}",
		"/api/document-revisions/{revision_id}/production-bibles",
		"/api/episodes/{episode_id}/storyboard-drafts",
		"/api/storyboard-draft-batches/{batch_id}/apply",
		"/api/storyboard-exports/{export_id}/download",
		"/api/projects/{project_id}/storygraph/current",
		"/api/projects/{project_id}/storygraph/versions/{version_id}",
		"/api/projects/{project_id}/storygraph/versions/{version_ref}/lens",
		"/api/projects/{project_id}/storygraph/versions/{version_ref}/nodes/{story_node_key}/trace",
		"/api/projects/{project_id}/storygraph/diff",
		"/api/projects/{project_id}/human-tasks",
		"/api/human-tasks/{human_task_id}",
		"/api/human-tasks/{human_task_id}/claims",
		"/api/human-tasks/{human_task_id}/claim-renewals",
		"/api/human-tasks/{human_task_id}/claim-releases",
		"/api/human-tasks/{human_task_id}/decisions",
		"/api/review-decisions/{review_decision_id}/resume",
	} {
		if _, exists := document.Paths[path]; !exists {
			t.Errorf("public contract is missing %s", path)
		}
	}
	if _, exists := document.Paths["/api/episodes/{episode_id}/storyboard-draft-batches"]; exists {
		t.Error("public contract still exposes the removed legacy storyboard route")
	}
	if _, exists := document.Paths["/api/projects/{project_id}/budget-limit"]; exists {
		t.Error("public contract still exposes the Production-owned legacy budget route")
	}
	if _, exists := document.Paths["/api/projects/{project_id}/cost-estimates"]; exists {
		t.Error("public contract exposes internal estimates before GenerationIntent coordination")
	}
	if _, exists := document.Paths["/api/projects/{project_id}/cost-prices/{metric}"]; exists {
		t.Error("public contract still exposes the removed metric-scoped price route")
	}
	if _, exists := document.Paths["/api/projects/{project_id}/provider-model-profiles/{model_profile_version_id}/cost-price"]; exists {
		t.Error("public contract still exposes the rejected provider-model-profiles price route")
	}
	if _, exists := document.Paths["/api/projects/{project_id}/generation/image-provider-bindings"]; exists {
		t.Error("public contract still exposes the removed fixed image Provider binding route")
	}
	var projectSchema struct {
		Required   []string                   `json:"required"`
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(document.Components.Schemas["ProjectResponse"], &projectSchema); err != nil {
		t.Fatalf("decode ProjectResponse: %v", err)
	}
	if _, exists := projectSchema.Properties["budget_limit"]; exists {
		t.Error("ProjectResponse still owns budget_limit")
	}
	if _, exists := projectSchema.Properties["currency"]; exists {
		t.Error("ProjectResponse still owns budget currency")
	}
	if _, exists := document.Components.Schemas["CostBudgetResponse"]; !exists {
		t.Error("public contract is missing CostBudgetResponse")
	}
	if _, exists := document.Components.Schemas["CostPriceQuoteResponse"]; !exists {
		t.Error("public contract is missing CostPriceQuoteResponse")
	}
	for _, schema := range []string{
		"AcceptScriptSourceRequest", "AcceptedScriptSourceResponse",
		"ScriptSpanCandidate", "SceneFactCandidate", "SceneAnalysisCandidateResponse",
	} {
		if _, exists := document.Components.Schemas[schema]; !exists {
			t.Errorf("public contract is missing %s", schema)
		}
	}
	var priceQuoteSetSchema struct {
		Required   []string `json:"required"`
		Properties map[string]struct {
			Not *struct {
				Pattern string `json:"pattern"`
			} `json:"not"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(document.Components.Schemas["CostPriceQuoteSetRequest"], &priceQuoteSetSchema); err != nil {
		t.Fatalf("decode CostPriceQuoteSetRequest: %v", err)
	}
	if !slices.Contains(priceQuoteSetSchema.Required, "expected_revision") {
		t.Error("CostPriceQuoteSetRequest does not require explicit expected_revision")
	}
	amount := priceQuoteSetSchema.Properties["reservation_unit_amount"]
	if amount.Not == nil || amount.Not.Pattern == "" {
		t.Fatal("CostPriceQuoteSetRequest does not explicitly reject zero reservation amount")
	}
	zeroPattern, err := regexp.Compile(amount.Not.Pattern)
	if err != nil {
		t.Fatalf("compile zero reservation amount pattern: %v", err)
	}
	for _, zero := range []string{"0", "0.0", "0.000000"} {
		if !zeroPattern.MatchString(zero) {
			t.Errorf("zero reservation amount %q is not rejected by the public schema", zero)
		}
	}
	for _, positive := range []string{"0.000001", "0.125000", "1"} {
		if zeroPattern.MatchString(positive) {
			t.Errorf("positive reservation amount %q is rejected by the public schema", positive)
		}
	}
	var priceQuoteSchema struct {
		Required   []string                   `json:"required"`
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(document.Components.Schemas["CostPriceQuoteResponse"], &priceQuoteSchema); err != nil {
		t.Fatalf("decode CostPriceQuoteResponse: %v", err)
	}
	for _, field := range []string{"model_profile_version_id", "billing_metric", "reservation_unit_amount", "content_hash"} {
		if _, exists := priceQuoteSchema.Properties[field]; !exists {
			t.Errorf("CostPriceQuoteResponse is missing exact field %s", field)
		}
	}
	for _, legacy := range []string{"metric", "unit_amount"} {
		if _, exists := priceQuoteSchema.Properties[legacy]; exists {
			t.Errorf("CostPriceQuoteResponse still exposes legacy field %s", legacy)
		}
	}
	for _, schema := range []string{"ImageProviderBindingPublishRequest", "ImageProviderBindingResponse"} {
		if _, exists := document.Components.Schemas[schema]; exists {
			t.Errorf("public contract still exposes removed schema %s", schema)
		}
	}
	for _, schema := range []string{"StoryGraphVersionResponse", "StoryGraphSubgraphResponse", "StoryGraphDiffResponse"} {
		if _, exists := document.Components.Schemas[schema]; !exists {
			t.Errorf("public contract is missing %s", schema)
		}
	}
	for _, schema := range []string{
		"HumanTaskListEnvelope", "HumanTaskDetailEnvelope", "HumanTaskCommandEnvelope",
		"HumanGateDecisionEnvelope", "HumanGateResumeEnvelope", "HumanGateCoordinationResponse",
	} {
		if _, exists := document.Components.Schemas[schema]; !exists {
			t.Errorf("public contract is missing %s", schema)
		}
	}
	var listClaim struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(document.Components.Schemas["HumanTaskClaimSummary"], &listClaim); err != nil {
		t.Fatalf("decode HumanTaskClaimSummary: %v", err)
	}
	if _, exposesToken := listClaim.Properties["claim_token"]; exposesToken {
		t.Error("HumanTask list claim summary exposes claim_token")
	}
}
