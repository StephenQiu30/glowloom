package contract

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"

	generation "github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
)

const VisionReviewInputContractID = "vision-review-input-production"

// VisionReviewVisualGrammar is the complete review-facing Preset grammar, not
// the smaller design-stage snapshot or an independently writable Preset fact.
type VisionReviewVisualGrammar struct {
	Medium              string   `json:"medium"`
	Realism             string   `json:"realism"`
	ShapeLanguage       string   `json:"shape_language"`
	Proportion          string   `json:"proportion"`
	Palette             string   `json:"palette"`
	Linework            string   `json:"linework"`
	Texture             string   `json:"texture"`
	MaterialRendering   string   `json:"material_rendering"`
	Lighting            string   `json:"lighting"`
	Contrast            string   `json:"contrast"`
	Composition         string   `json:"composition"`
	Camera              string   `json:"camera"`
	NegativeConstraints []string `json:"negative_constraints"`
}

type VisionReviewPolicyRef struct {
	Owner       string `json:"owner"`
	Key         string `json:"key"`
	ContentHash string `json:"content_hash"`
}

type VisionReviewVisualContext struct {
	ApplicationMode             string                      `json:"application_mode"`
	ProductionWorldOwnerSetHash string                      `json:"production_world_owner_set_hash"`
	VisualGrammar               VisionReviewVisualGrammar   `json:"visual_grammar"`
	StylePolicy                 VisualFoundationPolicy      `json:"style_policy"`
	FidelityInvariants          []string                    `json:"fidelity_invariants"`
	WorldAdaptations            []WorldAdaptationProposal   `json:"world_adaptations"`
	WorldConflicts              []VisualWorldConflict       `json:"world_conflicts"`
	CreativeFillProposals       []CreativeFillProposal      `json:"creative_fill_proposals"`
	PurposeProfile              ReferencePlanPurposeProfile `json:"purpose_profile"`
	QCPolicyRef                 VisionReviewPolicyRef       `json:"qc_policy_ref"`
}

type VisionReviewInput struct {
	ContractID       string                              `json:"contract_id"`
	Subject          VisionReviewSubject                 `json:"subject"`
	BriefInput       ReferenceBriefInput                 `json:"brief_input"`
	BriefCandidate   ReferenceBriefCandidate             `json:"brief_candidate"`
	BriefContentHash string                              `json:"brief_content_hash"`
	VisualContext    VisionReviewVisualContext           `json:"visual_context"`
	Admission        generation.ReferenceBundleAdmission `json:"admission"`
	Attachments      []VisionReviewMediaAttachment       `json:"attachments"`
}

// BuildVisionReviewInput seals a typed projection. Only the Backend compiler
// can establish its Owner provenance; sealing never establishes authorization.
func BuildVisionReviewInput(input VisionReviewInput) (VisionReviewInput, error) {
	input.ContractID = VisionReviewInputContractID
	input.Subject.InputHash = strings.Repeat("0", 64)
	if err := input.validateShape(); err != nil {
		return VisionReviewInput{}, err
	}
	hash, err := visionReviewInputHash(input)
	if err != nil {
		return VisionReviewInput{}, err
	}
	input.Subject.InputHash = hash
	// Detach all nested mutable slices and maps from the caller's Owner snapshots.
	raw, err := json.Marshal(input)
	if err != nil {
		return VisionReviewInput{}, err
	}
	return DecodeVisionReviewInput(raw)
}

func (input VisionReviewInput) Validate() error {
	if err := input.validateShape(); err != nil {
		return err
	}
	hash, err := visionReviewInputHash(input)
	if err != nil || hash != input.Subject.InputHash {
		return errors.New("Vision Review input hash has drifted")
	}
	return nil
}

func (input VisionReviewInput) validateShape() error {
	if input.ContractID != VisionReviewInputContractID || input.Subject.Validate() != nil || input.BriefCandidate.ValidateFor(input.BriefInput) != nil ||
		input.Admission.ValidateInternalReview() != nil || ValidateVisionReviewAttachments(input.Subject, input.Attachments) != nil {
		return errors.New("invalid Vision Review input")
	}
	brief, subject := input.BriefInput, input.Subject
	if !slices.Contains([]string{"character_identity_anchor", "location_board", "prop_sheet"}, subject.TargetKind) || len(brief.DependencySelections) != 0 {
		return errors.New("Vision Review dependency assets are not ready")
	}
	if subject.WorkspaceID != brief.WorkspaceID || subject.ProjectID != brief.ProjectID || subject.TargetKind != brief.TargetKind || subject.GenerationRound != 1 {
		return errors.New("Vision Review Brief scope has drifted")
	}
	raw, err := json.Marshal(input.BriefCandidate)
	if err != nil {
		return err
	}
	hash, err := canonical.Hash(raw)
	if err != nil || hash != input.BriefContentHash {
		return errors.New("Vision Review Brief content has drifted")
	}
	return input.VisualContext.validateFor(subject.TargetKind)
}

func (visual VisionReviewVisualContext) validateFor(kind string) error {
	g := visual.VisualGrammar
	for _, value := range []string{g.Medium, g.Realism, g.ShapeLanguage, g.Proportion, g.Palette, g.Linework, g.Texture, g.MaterialRendering, g.Lighting, g.Contrast, g.Composition, g.Camera, visual.QCPolicyRef.Owner, visual.QCPolicyRef.Key} {
		if value == "" || strings.TrimSpace(value) != value {
			return errors.New("invalid Vision Review visual context text")
		}
	}
	if !validVisualFoundationMode(visual.ApplicationMode) || !hashPattern.MatchString(visual.ProductionWorldOwnerSetHash) ||
		!hashPattern.MatchString(visual.QCPolicyRef.ContentHash) || visual.PurposeProfile.TargetKind != kind ||
		!reflect.DeepEqual(visual.FidelityInvariants, visualFoundationFidelityInvariants) || validateVisualFoundationPolicy(visual.StylePolicy) != nil ||
		visual.WorldAdaptations == nil || visual.WorldConflicts == nil || visual.CreativeFillProposals == nil ||
		validateWorldAdaptationProposals(visual.WorldAdaptations) != nil || validateVisualWorldConflicts(visual.WorldConflicts) != nil || validateCreativeFillProposals(visual.CreativeFillProposals) != nil ||
		visual.ApplicationMode == "faithful" && len(visual.WorldAdaptations) != 0 {
		return errors.New("invalid Vision Review visual policy")
	}
	for _, values := range [][]string{g.NegativeConstraints, visual.PurposeProfile.DesignFocus, visual.PurposeProfile.ForbiddenChanges} {
		if len(values) == 0 || !sortedUnique(values) {
			return errors.New("Vision Review visual rules must be explicit and ordered")
		}
		for _, value := range values {
			if value == "" || strings.TrimSpace(value) != value {
				return errors.New("invalid Vision Review visual rule")
			}
		}
	}
	return nil
}

func visionReviewInputHash(input VisionReviewInput) (string, error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	var material map[string]json.RawMessage
	if err = json.Unmarshal(raw, &material); err != nil {
		return "", err
	}
	var subject map[string]json.RawMessage
	if err = json.Unmarshal(material["subject"], &subject); err != nil {
		return "", err
	}
	delete(subject, "input_hash")
	material["subject"], err = json.Marshal(subject)
	if err != nil {
		return "", err
	}
	raw, err = json.Marshal(material)
	if err != nil {
		return "", err
	}
	return canonical.Hash(raw)
}

func DecodeVisionReviewInput(raw json.RawMessage) (VisionReviewInput, error) {
	var input VisionReviewInput
	if err := canonical.Decode(raw, &input); err != nil {
		return VisionReviewInput{}, err
	}
	if err := input.Validate(); err != nil {
		return VisionReviewInput{}, err
	}
	wire, err := canonical.JSON(raw)
	if err != nil {
		return VisionReviewInput{}, err
	}
	typed, err := json.Marshal(input)
	if err != nil {
		return VisionReviewInput{}, err
	}
	typedWire, err := canonical.JSON(typed)
	if err != nil || !bytes.Equal(wire, typedWire) {
		return VisionReviewInput{}, errors.New("Vision Review input wire shape is incomplete")
	}
	return input, nil
}
