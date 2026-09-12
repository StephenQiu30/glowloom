package storygraph_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	agentapp "github.com/StephenQiu30/lanverse/backend/internal/agent/application"
	contract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	openaiadapter "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/openai"
	generationapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
)

func TestReferenceImageCompilerFreezesIndependentViews(t *testing.T) {
	repo, service, actor, command := referenceTargetFixture(t)
	target, err := service.BuildInitial(context.Background(), actor, command)
	if err != nil {
		t.Fatal(err)
	}
	profile := referenceImageProfile(t, target.WorkspaceID)
	compiled, err := openaiadapter.CompileReferenceImages(target, repo.brief, profile)
	if err != nil {
		t.Fatal(err)
	}
	if len(compiled.Requests) != 6 || compiled.ManifestHash == "" {
		t.Fatal("missing Bundle views or manifest identity")
	}
	replayed, err := openaiadapter.CompileReferenceImages(target, repo.brief, profile)
	if err != nil || !reflect.DeepEqual(compiled, replayed) {
		t.Fatalf("unstable compilation: %v", err)
	}
	for i, request := range compiled.Requests {
		if request.BundleIndex != i/3 || request.SlotKey != target.OutputContract.Slots[i%3].SlotKey {
			t.Fatal("wrong call identity")
		}
		var body map[string]any
		if err := json.Unmarshal(request.Body, &body); err != nil {
			t.Fatal(err)
		}
		if len(body) != 7 || body["model"] != "gpt-image-2" || body["n"] != float64(1) || body["quality"] != "high" || body["output_format"] != "png" || body["size"] != "1024x1024" || body["stream"] != false {
			t.Fatal("wrong Provider parameters")
		}
		prompt := body["prompt"].(string)
		for _, required := range []string{request.SlotKey, "source_design_slots", "negative_instructions", "layout_requirements", "scale_requirements", "rights_requirements", "provenance_requirements", "identity_invariant_slots"} {
			if !strings.Contains(prompt, required) {
				t.Fatalf("lost %s", required)
			}
		}
		if strings.Contains(prompt, target.WorkspaceID) || strings.Contains(prompt, "required_view_roles") {
			t.Fatal("leaked Owner identity or requested a sheet")
		}
		hash, err := canonical.Hash(request.Body)
		if err != nil || hash != request.ContentHash {
			t.Fatal("request identity mismatch")
		}
	}
	if compiled.Requests[0].ContentHash == compiled.Requests[1].ContentHash || compiled.Requests[0].ContentHash != compiled.Requests[3].ContentHash {
		t.Fatal("view requests or Bundle identities confused")
	}
	profile.ID = uuid.NewString()
	changed, err := openaiadapter.CompileReferenceImages(target, repo.brief, profile)
	if err != nil || changed.ManifestHash == compiled.ManifestHash {
		t.Fatal("profile version not frozen")
	}
}

func TestReferenceImageCompilerRejectsDriftAndUnsupportedInputs(t *testing.T) {
	for name, mutate := range map[string]func(*generationapp.ReferenceGenerationTarget, *agentapp.AcceptedReferenceBrief, *domain.ProviderModelProfileVersion){
		"target_hash": func(v *generationapp.ReferenceGenerationTarget, _ *agentapp.AcceptedReferenceBrief, _ *domain.ProviderModelProfileVersion) {
			v.ContentHash = strings.Repeat("f", 64)
		},
		"brief_revision": func(_ *generationapp.ReferenceGenerationTarget, b *agentapp.AcceptedReferenceBrief, _ *domain.ProviderModelProfileVersion) {
			b.Revision++
		},
		"brief_content": func(_ *generationapp.ReferenceGenerationTarget, b *agentapp.AcceptedReferenceBrief, _ *domain.ProviderModelProfileVersion) {
			b.Candidate.ScaleRequirements = []string{"different"}
		},
		"brief_input": func(_ *generationapp.ReferenceGenerationTarget, b *agentapp.AcceptedReferenceBrief, _ *domain.ProviderModelProfileVersion) {
			b.Input.TypedReadSetRoot = strings.Repeat("f", 64)
		},
		"profile_hash": func(_ *generationapp.ReferenceGenerationTarget, _ *agentapp.AcceptedReferenceBrief, p *domain.ProviderModelProfileVersion) {
			p.ContentHash = strings.Repeat("f", 64)
		},
		"model": func(_ *generationapp.ReferenceGenerationTarget, _ *agentapp.AcceptedReferenceBrief, p *domain.ProviderModelProfileVersion) {
			p.ExternalModelID = "another-model"
			hashReferenceImageProfile(t, p)
		},
		"disabled": func(_ *generationapp.ReferenceGenerationTarget, _ *agentapp.AcceptedReferenceBrief, p *domain.ProviderModelProfileVersion) {
			p.State = "disabled"
			hashReferenceImageProfile(t, p)
		},
		"defaults": func(_ *generationapp.ReferenceGenerationTarget, _ *agentapp.AcceptedReferenceBrief, p *domain.ProviderModelProfileVersion) {
			p.Defaults["quality"] = "low"
			hashReferenceImageProfile(t, p)
		},
		"scope": func(_ *generationapp.ReferenceGenerationTarget, _ *agentapp.AcceptedReferenceBrief, p *domain.ProviderModelProfileVersion) {
			p.WorkspaceID = uuid.NewString()
			hashReferenceImageProfile(t, p)
		},
	} {
		t.Run(name, func(t *testing.T) {
			repo, service, actor, command := referenceTargetFixture(t)
			target, err := service.BuildInitial(context.Background(), actor, command)
			if err != nil {
				t.Fatal(err)
			}
			profile := referenceImageProfile(t, target.WorkspaceID)
			mutate(&target, &repo.brief, &profile)
			got, err := openaiadapter.CompileReferenceImages(target, repo.brief, profile)
			if err == nil || !reflect.DeepEqual(got, openaiadapter.ReferenceImageCompilation{}) {
				t.Fatal("unsafe input returned requests")
			}
		})
	}
}

func referenceImageProfile(t *testing.T, workspace string) domain.ProviderModelProfileVersion {
	t.Helper()
	p := domain.ProviderModelProfileVersion{ID: uuid.NewString(), WorkspaceID: workspace, ProfileKey: "reference-image", ConnectionKey: "openai", Revision: 1, CreationSource: map[string]any{"preset_key": "openai.gpt-image-2"}, ProviderKey: "openai", ExternalModelID: "gpt-image-2", Modality: "image", Family: "gpt_image", AdapterTransportContract: "openai-image-api-nonstreaming", CapabilitySchemaVersion: "gpt_image-capability", BillingMetric: "generation.image.call", Defaults: map[string]any{}, State: "enabled", CreatedBy: uuid.NewString(), CreatedAt: time.Now().UTC()}
	hashReferenceImageProfile(t, &p)
	return p
}

func TestReferenceImageCompilerChecksExactOutputPolicy(t *testing.T) {
	for _, tc := range []struct {
		name          string
		width, height int
		ratio, media  string
		allowed       bool
	}{
		{"portrait", 1024, 1536, "2:3", "image/png", true},
		{"custom_landscape", 2048, 1152, "16:9", "image/png", true},
		{"max_pixels", 3840, 2160, "16:9", "image/png", true},
		{"min_pixels", 640, 1024, "5:8", "image/png", true},
		{"not_multiple", 1025, 1025, "1:1", "image/png", false},
		{"too_few_pixels", 640, 640, "1:1", "image/png", false},
		{"too_many_pixels", 3840, 3840, "1:1", "image/png", false},
		{"edge_limit", 4096, 2048, "2:1", "image/png", false},
		{"long_ratio", 3072, 768, "4:1", "image/png", false},
		{"wrong_ratio", 1536, 1024, "1:1", "image/png", false},
		{"jpeg_only", 1024, 1024, "1:1", "image/jpeg", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, service, actor, command := referenceTargetFixture(t)
			for i := range command.SlotPolicies {
				command.SlotPolicies[i].MinWidth, command.SlotPolicies[i].MinHeight, command.SlotPolicies[i].AspectRatio, command.SlotPolicies[i].AllowedMediaTypes = tc.width, tc.height, tc.ratio, []string{tc.media}
			}
			target, err := service.BuildInitial(context.Background(), actor, command)
			if err != nil {
				t.Fatal(err)
			}
			got, err := openaiadapter.CompileReferenceImages(target, repo.brief, referenceImageProfile(t, target.WorkspaceID))
			if tc.allowed {
				if err != nil || len(got.Requests) != 6 {
					t.Fatalf("valid explicit dimensions rejected: %v", err)
				}
			} else if err == nil || !reflect.DeepEqual(got, openaiadapter.ReferenceImageCompilation{}) {
				t.Fatal("invalid policy produced requests")
			}
		})
	}
}

func TestReferenceImageCompilerRejectsOversizePromptWithoutTruncation(t *testing.T) {
	repo, service, actor, command := referenceTargetFixture(t)
	repo.brief.Candidate.SourceDesignSlots[0].DesignRequirement = strings.Repeat("形", 11000)
	raw, err := json.Marshal(repo.brief.Candidate)
	if err != nil {
		t.Fatal(err)
	}
	repo.brief.ContentHash, err = canonical.Hash(raw)
	if err != nil {
		t.Fatal(err)
	}
	target, err := service.BuildInitial(context.Background(), actor, command)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := openaiadapter.CompileReferenceImages(target, repo.brief, referenceImageProfile(t, target.WorkspaceID)); err == nil || !reflect.DeepEqual(got, openaiadapter.ReferenceImageCompilation{}) {
		t.Fatal("oversized prompt produced requests")
	}
}

func TestReferenceImageCompilerPreservesLocationAndPropPurpose(t *testing.T) {
	for _, kind := range []string{"location_board", "prop_sheet"} {
		t.Run(kind, func(t *testing.T) {
			repo, service, actor, command := referenceTargetFixture(t)
			target, err := service.BuildInitial(context.Background(), actor, command)
			if err != nil {
				t.Fatal(err)
			}
			var anchor generationapp.CharacterIdentityAnchorSource
			if err = json.Unmarshal(target.SourcePayload, &anchor); err != nil {
				t.Fatal(err)
			}
			// Pure compiler fixtures: valid immutable input identities, not evidence
			// that another kind was published against live Owner facts.
			refs := repo.brief.Input.SourceRefs
			parts := []any{kind}
			for _, ref := range []contract.ReferencePlanOwnerRef{refs.Identity[0], refs.Specification[0], refs.State[0]} {
				parts = append(parts, []string{ref.OwnerKind, ref.VersionFamily, ref.OwnerLogicalID, ""})
			}
			key, err := json.Marshal(parts)
			if err != nil {
				t.Fatal(err)
			}
			target.TargetKind, target.TargetBusinessKey = kind, string(key)
			target.ReferencePlanTargetRef.OwnerLogicalID = string(key)
			b := &repo.brief.Candidate
			i := &repo.brief.Input
			b.TargetKind, i.TargetKind, b.TargetBusinessKey, i.TargetBusinessKey = kind, kind, string(key), string(key)
			b.ReferencePlanTargetRef, i.ReferencePlanTargetRef = target.ReferencePlanTargetRef, target.ReferencePlanTargetRef
			b.RequiredViewRoles, i.RequiredViewRoles = contract.ReferenceBriefRequiredViewRoles(kind), contract.ReferenceBriefRequiredViewRoles(kind)
			base := anchor.ReferenceBaseSource
			base.RequiredViewRoles = b.RequiredViewRoles
			var purpose, source any
			if kind == "location_board" {
				p := contract.LocationBoardBrief{TargetKind: kind, TopologyConstraints: []string{"preserve entrance"}, ScaleAnchors: []string{"approved scale"}, MaterialSlots: []string{"stone"}, OccupancyPolicy: "empty"}
				purpose, source = p, generationapp.LocationBoardSource{ReferenceBaseSource: base, LocationBoardBrief: p, LocationIdentityRef: refs.Identity[0], LocationSpecificationRef: refs.Specification[0], LocationAssetStateRef: refs.State[0]}
			} else {
				p := contract.PropSheetBrief{TargetKind: kind, PhysicalDimensions: "approved dimensions", StructuralSlots: []string{"handle"}, StateSlots: []string{"closed"}, ContentOrMechanismSlots: []string{"hinge"}, OccupancyPolicy: "no_hands_no_people"}
				purpose, source = p, generationapp.PropSheetSource{ReferenceBaseSource: base, PropSheetBrief: p, PropIdentityRef: refs.Identity[0], PropSpecificationRef: refs.Specification[0], PropAssetStateRef: refs.State[0]}
			}
			b.Brief, err = json.Marshal(purpose)
			if err != nil {
				t.Fatal(err)
			}
			target.SourcePayload, err = json.Marshal(source)
			if err != nil {
				t.Fatal(err)
			}
			policies := []generationapp.ReferenceOutputSlotPolicy{}
			for _, role := range b.RequiredViewRoles {
				policies = append(policies, generationapp.ReferenceOutputSlotPolicy{ViewRole: role, AllowedMediaTypes: []string{"image/png"}, AspectRatio: "1:1", MinWidth: 1024, MinHeight: 1024, MaxBytes: 8 << 20})
			}
			target.OutputContract, err = generationapp.CompileReferenceOutputContract(*i, *b, 2, policies)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(b)
			if err != nil {
				t.Fatal(err)
			}
			repo.brief.ContentHash, err = canonical.Hash(raw)
			if err != nil {
				t.Fatal(err)
			}
			target.ReferenceBriefRevisionRef.ContentHash = repo.brief.ContentHash
			hashReferenceImageTarget(t, &target)
			compiled, err := openaiadapter.CompileReferenceImages(target, repo.brief, referenceImageProfile(t, target.WorkspaceID))
			if err != nil || len(compiled.Requests) != 2*len(b.RequiredViewRoles) {
				t.Fatalf("compile %s: %v", kind, err)
			}
			for _, request := range compiled.Requests {
				if !strings.Contains(string(request.Body), "occupancy_policy") {
					t.Fatal("lost purpose")
				}
			}
			// Re-hashing a Target cannot silently substitute a different source.
			target.SourcePayload = json.RawMessage(strings.Replace(string(target.SourcePayload), refs.State[0].OwnerContentHash, strings.Repeat("f", 64), 1))
			hashReferenceImageTarget(t, &target)
			if _, err := openaiadapter.CompileReferenceImages(target, repo.brief, referenceImageProfile(t, target.WorkspaceID)); err == nil {
				t.Fatal("rehashed source drift accepted")
			}
		})
	}
}

func hashReferenceImageTarget(t *testing.T, target *generationapp.ReferenceGenerationTarget) {
	t.Helper()
	value := *target
	value.ContentHash, value.CreatedBy, value.CreatedAt = "", "", time.Time{}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	target.ContentHash, err = canonical.Hash(raw)
	if err != nil {
		t.Fatal(err)
	}
}

func hashReferenceImageProfile(t *testing.T, p *domain.ProviderModelProfileVersion) {
	t.Helper()
	value := *p
	value.ID, value.ContentHash, value.CreatedBy, value.CreatedAt = "", "", "", time.Time{}
	hash, err := platformcommand.InputHash(value)
	if err != nil {
		t.Fatal(err)
	}
	p.ContentHash = hash
}
