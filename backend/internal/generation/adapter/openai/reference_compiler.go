package openai

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"

	agentapp "github.com/StephenQiu30/lanverse/backend/internal/agent/application"
	contract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	app "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
)

const ReferenceImageCompilerContractID = "openai-reference-image-compiler"

// Body is invocation-scoped material. It must not enter a Snapshot, receipt or log.
type ReferenceImageRequest struct {
	BundleIndex int             `json:"bundle_index"`
	SlotKey     string          `json:"slot_key"`
	ContentHash string          `json:"content_hash"`
	Body        json.RawMessage `json:"-"`
}

type ReferenceImageCompilation struct {
	ContractID   string                        `json:"contract_id"`
	TargetRef    domain.GenerationRevisionRef  `json:"target_ref"`
	BriefRef     app.ReferenceBriefRevisionRef `json:"brief_ref"`
	ProfileRef   domain.GenerationRevisionRef  `json:"profile_ref"`
	Requests     []ReferenceImageRequest       `json:"requests"`
	ManifestHash string                        `json:"manifest_hash"`
}

// CompileReferenceImages performs no IO and grants no sending permission. The
// preparation transaction must separately revalidate Owner/Binding/Head facts,
// consume execution authorization and freeze this exact manifest identity.
func CompileReferenceImages(target app.ReferenceGenerationTarget, brief agentapp.AcceptedReferenceBrief, profile domain.ProviderModelProfileVersion) (ReferenceImageCompilation, error) {
	if err := validateReferenceCompilationInputs(target, brief); err != nil {
		return ReferenceImageCompilation{}, err
	}
	if err := app.ValidateProviderModelProfileVersion(profile); err != nil {
		return ReferenceImageCompilation{}, err
	}
	if profile.WorkspaceID != target.WorkspaceID || profile.State != domain.ProviderStateEnabled || profile.ProviderKey != domain.MediaProviderOpenAI || profile.ExternalModelID != "gpt-image-2" || profile.Modality != "image" || profile.Family != "gpt_image" || profile.CapabilitySchemaVersion != "gpt_image-capability" || profile.AdapterTransportContract != "openai-image-api-nonstreaming" || len(profile.Defaults) != 0 {
		return ReferenceImageCompilation{}, errors.New("unsupported Reference image profile")
	}
	result := ReferenceImageCompilation{ContractID: ReferenceImageCompilerContractID, TargetRef: domain.GenerationRevisionRef{ID: target.ID, Revision: target.Revision, ContentHash: target.ContentHash}, BriefRef: target.ReferenceBriefRevisionRef, ProfileRef: domain.GenerationRevisionRef{ID: profile.ID, Revision: profile.Revision, ContentHash: profile.ContentHash}, Requests: []ReferenceImageRequest{}}
	for bundle := 0; bundle < target.OutputContract.CandidateBundleCount; bundle++ {
		for _, slot := range target.OutputContract.Slots {
			if err := validateReferenceImageSize(slot); err != nil {
				return ReferenceImageCompilation{}, err
			}
			prompt, err := compileReferenceImagePrompt(brief.Candidate, slot.ViewRole)
			if err != nil {
				return ReferenceImageCompilation{}, err
			}
			body, err := json.Marshal(struct {
				Model        string `json:"model"`
				Prompt       string `json:"prompt"`
				Size         string `json:"size"`
				N            int    `json:"n"`
				Quality      string `json:"quality"`
				OutputFormat string `json:"output_format"`
				Stream       bool   `json:"stream"`
			}{profile.ExternalModelID, prompt, fmt.Sprintf("%dx%d", slot.MinWidth, slot.MinHeight), 1, "high", "png", false})
			if err != nil {
				return ReferenceImageCompilation{}, err
			}
			body, err = canonical.JSON(body)
			if err != nil {
				return ReferenceImageCompilation{}, err
			}
			hash, err := canonical.Hash(body)
			if err != nil {
				return ReferenceImageCompilation{}, err
			}
			result.Requests = append(result.Requests, ReferenceImageRequest{BundleIndex: bundle, SlotKey: slot.SlotKey, ContentHash: hash, Body: body})
		}
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return ReferenceImageCompilation{}, err
	}
	result.ManifestHash, err = canonical.Hash(raw)
	if err != nil {
		return ReferenceImageCompilation{}, err
	}
	return result, nil
}

func validateReferenceImageSize(slot domain.ReferenceOutputSlot) error {
	w, h := slot.MinWidth, slot.MinHeight
	a, b, ok := strings.Cut(slot.AspectRatio, ":")
	x, xerr := strconv.Atoi(a)
	y, yerr := strconv.Atoi(b)
	if !ok || xerr != nil || yerr != nil || x < 1 || y < 1 || w < 1 || h < 1 || w > 3840 || h > 3840 || w%16 != 0 || h%16 != 0 || w > 3*h || h > 3*w || w*h < 655360 || w*h > 8294400 || w*y != h*x || !slices.Contains(slot.AllowedMediaTypes, "image/png") {
		return errors.New("Reference image slot is outside exact GPT Image 2 capability")
	}
	return nil
}

func compileReferenceImagePrompt(brief contract.ReferenceBriefCandidate, role string) (string, error) {
	// Send only visual instructions, not internal Owner IDs, full read sets or a
	// list of other views that could turn an independent image into a contact sheet.
	raw, err := json.Marshal(struct {
		ViewRole          string                                    `json:"view_role"`
		SourceDesignSlots []contract.ReferenceBriefSourceDesignSlot `json:"source_design_slots"`
		Positive          []string                                  `json:"positive_instructions"`
		Negative          []string                                  `json:"negative_instructions"`
		Layout            []string                                  `json:"layout_requirements"`
		Scale             []string                                  `json:"scale_requirements"`
		Rights            []string                                  `json:"rights_requirements"`
		Provenance        []string                                  `json:"provenance_requirements"`
		Purpose           json.RawMessage                           `json:"brief"`
	}{role, brief.SourceDesignSlots, brief.PositiveInstructions, brief.NegativeInstructions, brief.LayoutRequirements, brief.ScaleRequirements, brief.RightsRequirements, brief.ProvenanceRequirements, brief.Brief})
	if err != nil {
		return "", err
	}
	raw, err = canonical.JSON(raw)
	if err != nil {
		return "", err
	}
	prompt := "Generate exactly one independent image of the specified view_role. Do not create a collage, contact sheet, or multiple views. Follow the approved source and design requirements below without inventing identities, objects, or states. Negative instructions are exclusions.\n" + string(raw)
	if len(prompt) > 32000 {
		return "", errors.New("Reference image prompt exceeds local byte budget")
	}
	return prompt, nil
}

func validateReferenceCompilationInputs(target app.ReferenceGenerationTarget, brief agentapp.AcceptedReferenceBrief) error {
	raw, err := json.Marshal(target)
	if err != nil {
		return err
	}
	if _, err = app.DecodeReferenceGenerationTarget(raw); err != nil {
		return err
	}
	if err = brief.Candidate.ValidateFor(brief.Input); err != nil {
		return err
	}
	raw, err = json.Marshal(brief.Candidate)
	if err != nil {
		return err
	}
	hash, err := canonical.Hash(raw)
	if err != nil {
		return err
	}
	ref := app.ReferenceBriefRevisionRef{ID: brief.RevisionID, Revision: brief.Revision, RevisionHash: brief.RevisionHash, ContentHash: brief.ContentHash}
	i := brief.Input
	if hash != brief.ContentHash || ref != target.ReferenceBriefRevisionRef || i.WorkspaceID != target.WorkspaceID || i.ProjectID != target.ProjectID || i.TargetKind != target.TargetKind || i.TargetBusinessKey != target.TargetBusinessKey || i.TargetFulfillment != target.Fulfillment || !reflect.DeepEqual(i.ApprovedReferencePlanVersionRef, target.ApprovedReferencePlanVersionRef) || !reflect.DeepEqual(i.ReferencePlanTargetRef, target.ReferencePlanTargetRef) || !reflect.DeepEqual(i.EffectiveStyleSnapshotRef, target.EffectiveStyleSnapshotRef) || !reflect.DeepEqual(i.EffectivePolicySnapshotRef, target.EffectivePolicySnapshotRef) || len(i.DependencySelections) != 0 {
		return errors.New("Reference image Target and accepted Brief differ")
	}
	policies := make([]app.ReferenceOutputSlotPolicy, len(target.OutputContract.Slots))
	for n, slot := range target.OutputContract.Slots {
		policies[n] = app.ReferenceOutputSlotPolicy{ViewRole: slot.ViewRole, AllowedMediaTypes: slot.AllowedMediaTypes, AspectRatio: slot.AspectRatio, MinWidth: slot.MinWidth, MinHeight: slot.MinHeight, MaxBytes: slot.MaxBytes}
	}
	output, err := app.CompileReferenceOutputContract(i, brief.Candidate, target.OutputContract.CandidateBundleCount, policies)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(output, target.OutputContract) {
		return errors.New("Reference image output semantics have drifted")
	}
	if err = validateReferenceSourceBrief(target, brief.Candidate); err != nil {
		return err
	}
	return nil
}

func validateReferenceSourceBrief(target app.ReferenceGenerationTarget, brief contract.ReferenceBriefCandidate) error {
	var identity, specification, state contract.ReferencePlanOwnerRef
	var base app.ReferenceBaseSource
	var purpose any
	switch target.TargetKind {
	case "character_identity_anchor":
		var value app.CharacterIdentityAnchorSource
		if err := canonical.Decode(target.SourcePayload, &value); err != nil {
			return err
		}
		base, purpose, identity, specification, state = value.ReferenceBaseSource, value.CharacterIdentityAnchorBrief, value.IdentityRef, value.CharacterSpecificationRef, value.IdentityAnchorAssetStateRef
	case "location_board":
		var value app.LocationBoardSource
		if err := canonical.Decode(target.SourcePayload, &value); err != nil {
			return err
		}
		base, purpose, identity, specification, state = value.ReferenceBaseSource, value.LocationBoardBrief, value.LocationIdentityRef, value.LocationSpecificationRef, value.LocationAssetStateRef
	case "prop_sheet":
		var value app.PropSheetSource
		if err := canonical.Decode(target.SourcePayload, &value); err != nil {
			return err
		}
		base, purpose, identity, specification, state = value.ReferenceBaseSource, value.PropSheetBrief, value.PropIdentityRef, value.PropSpecificationRef, value.PropAssetStateRef
	default:
		return errors.New("Reference image compiler requires a dependency-free base Target")
	}
	raw, err := json.Marshal(purpose)
	if err != nil {
		return err
	}
	hash, err := canonical.Hash(raw)
	if err != nil {
		return err
	}
	briefHash, err := canonical.Hash(brief.Brief)
	if err != nil {
		return err
	}
	refs := brief.SourceRefs
	if hash != briefHash || len(refs.Identity) != 1 || len(refs.Specification) != 1 || len(refs.State) != 1 || !reflect.DeepEqual(identity, refs.Identity[0]) || !reflect.DeepEqual(specification, refs.Specification[0]) || !reflect.DeepEqual(state, refs.State[0]) || !reflect.DeepEqual(base.OccurrenceRefs, refs.Occurrence) || !slices.Equal(base.RequiredViewRoles, brief.RequiredViewRoles) {
		return errors.New("Reference image source and Brief purpose have drifted")
	}
	return nil
}
