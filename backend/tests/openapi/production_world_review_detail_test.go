package openapi_test

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/api/openapi"
)

type productionWorldOpenAPISchema struct {
	Type       any                                     `json:"type"`
	Ref        string                                  `json:"$ref"`
	Required   []string                                `json:"required"`
	Properties map[string]productionWorldOpenAPISchema `json:"properties"`
	OneOf      []productionWorldOpenAPISchema          `json:"oneOf"`
	Items      *productionWorldOpenAPISchema           `json:"items"`
}

func TestProductionWorldReviewDetailExposesSixTypedViews(t *testing.T) {
	var document struct {
		Components struct {
			Schemas map[string]productionWorldOpenAPISchema `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(openapi.Document(), &document); err != nil {
		t.Fatal(err)
	}
	detail, ok := document.Components.Schemas["ProductionWorldReviewSubjectResponse"]
	if !ok {
		t.Fatal("missing ProductionWorldReviewSubjectResponse")
	}
	wantRequired := []string{
		"schema_version", "gate_key", "input_hash", "candidate_revision", "partition_roots",
		"allowed_decisions", "repair_targets", "views", "world_claims", "design_gaps", "review_issues",
	}
	if !slices.Equal(detail.Required, wantRequired) ||
		detail.Properties["views"].Ref != "#/components/schemas/ProductionWorldReviewViewsResponse" {
		t.Fatalf("Production World review detail schema = %#v", detail)
	}
	if detail.Properties["repair_targets"].Type != "array" || detail.Properties["repair_targets"].Items == nil ||
		detail.Properties["repair_targets"].Items.Ref != "#/components/schemas/ProductionWorldRepairTargetSetResponse" {
		t.Fatalf("Production World repair target inventory is not typed: %#v", detail.Properties["repair_targets"])
	}
	repairTarget, ok := document.Components.Schemas["ProductionWorldRepairTargetSetResponse"]
	if !ok || repairTarget.Type != "object" || repairTarget.Properties["operation"].Type != "string" ||
		repairTarget.Properties["target_keys"].Type != "array" {
		t.Fatalf("Production World repair target schema is incomplete: %#v", repairTarget)
	}
	views, ok := document.Components.Schemas["ProductionWorldReviewViewsResponse"]
	if !ok || !slices.Equal(views.Required, []string{
		"character_appearances", "locations", "prop_states", "scene_occurrences", "interactions", "continuity",
	}) {
		t.Fatalf("Production World six views = %#v", views)
	}
	for name, want := range map[string]string{
		"character_appearances": "#/components/schemas/ProductionWorldEntityReviewItemResponse",
		"locations":             "#/components/schemas/ProductionWorldEntityReviewItemResponse",
		"prop_states":           "#/components/schemas/ProductionWorldEntityReviewItemResponse",
		"scene_occurrences":     "#/components/schemas/ProductionWorldSceneOccurrenceReviewItemResponse",
		"interactions":          "#/components/schemas/ProductionWorldInteractionResponse",
	} {
		property := views.Properties[name]
		if property.Items == nil || property.Items.Ref != want {
			t.Errorf("Production World %s view item = %#v", name, property.Items)
		}
	}
	if views.Properties["continuity"].Ref != "#/components/schemas/ProductionWorldContinuityReviewResponse" {
		t.Fatal("Production World continuity view is not typed")
	}
	humanTask := document.Components.Schemas["HumanTaskDetailEnvelope"]
	data := humanTask.Properties["data"].Properties
	if len(data) == 0 {
		t.Fatal("HumanTask detail data schema is missing")
	}
	subject := data["subject"]
	found := false
	for _, variant := range subject.OneOf {
		found = found || variant.Ref == "#/components/schemas/ProductionWorldReviewSubjectResponse"
	}
	if !found {
		t.Fatal("HumanTask detail subject does not include Production World review detail")
	}
}
