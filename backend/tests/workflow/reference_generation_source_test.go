package workflow_test

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	contract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	generationapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	storygraph "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
)

// The persistence journey supplies accepted Agent facts and the actual published world.
func assertBaseReferenceGenerationSource(t *testing.T, input contract.ReferenceBriefInput, brief contract.ReferenceBriefCandidate, world storygraph.Version) contract.ReferencePlanOwnerRef {
	t.Helper()
	source, err := generationapp.CompileBaseReferenceGenerationSource(input, brief, world)
	if err != nil || len(source.ContentHash) != 64 || source.ProductionWorldOwnerSetHash != world.OwnerSetHash {
		t.Fatalf("compile exact %s source: %#v err=%v", input.TargetKind, source, err)
	}
	if hash, hashErr := canonical.Hash(source.Payload); hashErr != nil || hash != source.ContentHash {
		t.Fatalf("source content identity drifted: %v", hashErr)
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(source.Payload, &fields); err != nil {
		t.Fatal(err)
	}
	var binding contract.ReferencePlanOwnerRef
	if err = json.Unmarshal(fields["production_binding_ref"], &binding); err != nil || binding.FragmentKey == nil || !strings.HasPrefix(*binding.FragmentKey, "binding:") || binding.WorkspaceID != input.WorkspaceID || binding.ProjectID != input.ProjectID {
		t.Fatalf("source lacks exact Production Binding: %#v err=%v", binding, err)
	}
	branchKeys := map[string][]string{
		"character_identity_anchor": {"identity_ref", "character_specification_ref", "identity_anchor_asset_state_ref"},
		"location_board":            {"location_identity_ref", "location_specification_ref", "location_asset_state_ref"},
		"prop_sheet":                {"prop_identity_ref", "prop_specification_ref", "prop_asset_state_ref"},
	}[input.TargetKind]
	if len(branchKeys) != 3 {
		t.Fatal("unexpected base source kind")
	}
	expected := map[string]any{
		branchKeys[0]: input.SourceRefs.Identity[0], branchKeys[1]: input.SourceRefs.Specification[0], branchKeys[2]: input.SourceRefs.State[0],
		"occurrence_refs": input.SourceRefs.Occurrence, "required_view_roles": brief.RequiredViewRoles,
	}
	for key, want := range expected {
		raw, marshalErr := json.Marshal(want)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		wantHash, _ := canonical.Hash(raw)
		gotHash, fieldErr := canonical.Hash(fields[key])
		if fieldErr != nil || wantHash != gotHash {
			t.Fatalf("source lost exact %s: %v", key, fieldErr)
		}
	}
	var briefFields map[string]json.RawMessage
	if err = json.Unmarshal(brief.Brief, &briefFields); err != nil {
		t.Fatal(err)
	}
	for key, want := range briefFields {
		wantHash, _ := canonical.Hash(want)
		gotHash, fieldErr := canonical.Hash(fields[key])
		if fieldErr != nil || wantHash != gotHash {
			t.Fatalf("source lost Brief requirement %s: %v", key, fieldErr)
		}
	}
	if len(fields) != len(expected)+len(briefFields)+1 {
		t.Fatal("source contains unexpected branch fields")
	}
	replayed, err := generationapp.CompileBaseReferenceGenerationSource(input, brief, world)
	if err != nil || !reflect.DeepEqual(source, replayed) {
		t.Fatalf("Reference source replay changed content identity: %v", err)
	}
	for name, mutate := range map[string]func(*storygraph.Version){
		"project":       func(value *storygraph.Version) { value.ProjectID = uuid.NewString() },
		"owner hash":    func(value *storygraph.Version) { value.OwnerSetHash = strings.Repeat("f", 64) },
		"missing nodes": func(value *storygraph.Version) { value.Nodes = nil },
		"noncanonical immutable nodes": func(value *storygraph.Version) {
			value.Nodes = slices.Clone(value.Nodes)
			slices.Reverse(value.Nodes)
		},
	} {
		changed := world
		mutate(&changed)
		if _, err = generationapp.CompileBaseReferenceGenerationSource(input, brief, changed); err == nil {
			t.Fatalf("Reference source accepted world %s drift", name)
		}
	}
	for name, mutate := range map[string]func(*contract.ReferencePlanTargetOwnerRefs){
		"identity hash": func(refs *contract.ReferencePlanTargetOwnerRefs) {
			refs.Identity[0].OwnerContentHash = strings.Repeat("f", 64)
		},
		"specification fragment": func(refs *contract.ReferencePlanTargetOwnerRefs) {
			hash := strings.Repeat("f", 64)
			refs.Specification[0].FragmentContentHash = &hash
		},
		"state revision": func(refs *contract.ReferencePlanTargetOwnerRefs) { refs.State[0].OwnerRevision++ },
		"occurrence version": func(refs *contract.ReferencePlanTargetOwnerRefs) {
			refs.Occurrence[0].OwnerVersionID = uuid.NewString()
		},
		"scene hash": func(refs *contract.ReferencePlanTargetOwnerRefs) {
			refs.Scene[0].OwnerContentHash = strings.Repeat("f", 64)
		},
	} {
		changedInput, changedBrief := input, brief
		changedInput.SourceRefs = contract.ReferencePlanTargetOwnerRefs{}
		raw, marshalErr := json.Marshal(input.SourceRefs)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if err = json.Unmarshal(raw, &changedInput.SourceRefs); err != nil {
			t.Fatal(err)
		}
		mutate(&changedInput.SourceRefs)
		changedBrief.SourceRefs = changedInput.SourceRefs
		if err = changedBrief.ValidateFor(changedInput); err != nil {
			t.Fatalf("source drift fixture must pass the Brief structural fence: %s: %v", name, err)
		}
		if _, err = generationapp.CompileBaseReferenceGenerationSource(changedInput, changedBrief, world); err == nil {
			t.Fatalf("Reference source accepted %s drift", name)
		}
	}
	return binding
}
