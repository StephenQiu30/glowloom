package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/text/unicode/norm"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
)

const ReferenceOutputContractID = "reference-output-production"

type ReferenceOutputQCRubricRef struct {
	ContractID  string `json:"contract_id"`
	ContentHash string `json:"content_hash"`
}

type ReferenceOutputSlot struct {
	SlotKey              string                       `json:"slot_key"`
	ViewRole             string                       `json:"view_role"`
	Required             bool                         `json:"required"`
	AllowedMediaTypes    []string                     `json:"allowed_media_types"`
	AspectRatio          string                       `json:"aspect_ratio"`
	MinWidth             int                          `json:"min_width"`
	MinHeight            int                          `json:"min_height"`
	MaxBytes             int64                        `json:"max_bytes"`
	SemanticRequirements []string                     `json:"semantic_requirements"`
	QCRubricRefs         []ReferenceOutputQCRubricRef `json:"qc_rubric_refs"`
}

type ReferenceOutputContract struct {
	ContractID           string                `json:"contract_id"`
	Modality             string                `json:"modality"`
	CandidateBundleCount int                   `json:"candidate_bundle_count"`
	Slots                []ReferenceOutputSlot `json:"slots"`
	BundleCompleteness   string                `json:"bundle_completeness"`
	ContentHash          string                `json:"content_hash"`
}

// BuildReferenceOutputContract freezes a complete set of independently generated
// views. It validates an explicit output policy without selecting a Provider.
func BuildReferenceOutputContract(targetKind string, bundleCount int, slots []ReferenceOutputSlot) (ReferenceOutputContract, error) {
	roles := referenceOutputViewRoles(targetKind)
	if len(roles) == 0 || len(slots) != len(roles) || bundleCount < 1 || bundleCount > 4 {
		return ReferenceOutputContract{}, errors.New("invalid Reference output purpose, slot count or Bundle count")
	}
	ordered := slices.Clone(slots)
	slices.SortFunc(ordered, func(a, b ReferenceOutputSlot) int { return strings.Compare(a.ViewRole, b.ViewRole) })
	var bundleBytes int64
	for i, slot := range ordered {
		if slot.ViewRole != roles[i] || slot.SlotKey != slot.ViewRole || !slot.Required {
			return ReferenceOutputContract{}, errors.New("Reference output does not cover each required view exactly once")
		}
		normalized, err := normalizeReferenceOutputSlot(slot)
		if err != nil {
			return ReferenceOutputContract{}, fmt.Errorf("Reference output slot %s: %w", slot.SlotKey, err)
		}
		ordered[i] = normalized
		bundleBytes += slot.MaxBytes
	}
	if bundleBytes > 32<<20 {
		return ReferenceOutputContract{}, errors.New("Reference output Bundle exceeds Vision byte budget")
	}
	output := ReferenceOutputContract{
		ContractID: ReferenceOutputContractID, Modality: "image", CandidateBundleCount: bundleCount,
		Slots: ordered, BundleCompleteness: "all_required_slots",
	}
	raw, err := json.Marshal(output)
	if err != nil {
		return ReferenceOutputContract{}, err
	}
	output.ContentHash, err = canonical.Hash(raw)
	return output, err
}

func DecodeReferenceOutputContract(raw json.RawMessage, targetKind string) (ReferenceOutputContract, error) {
	var output ReferenceOutputContract
	if err := canonical.Decode(raw, &output); err != nil {
		return ReferenceOutputContract{}, fmt.Errorf("decode Reference output: %w", err)
	}
	rebuilt, err := BuildReferenceOutputContract(targetKind, output.CandidateBundleCount, output.Slots)
	if err != nil || !reflect.DeepEqual(rebuilt, output) {
		return ReferenceOutputContract{}, errors.New("Reference output contract has drifted")
	}
	return output, nil
}

func normalizeReferenceOutputSlot(slot ReferenceOutputSlot) (ReferenceOutputSlot, error) {
	if slot.MinWidth < 1 || slot.MinHeight < 1 || slot.MinWidth > 8192 || slot.MinHeight > 8192 ||
		slot.MaxBytes < 1 || slot.MaxBytes > 10<<20 || !validReferenceAspectRatio(slot.AspectRatio) {
		return ReferenceOutputSlot{}, errors.New("invalid media dimensions, ratio or byte limit")
	}
	slot.AllowedMediaTypes = slices.Clone(slot.AllowedMediaTypes)
	slices.Sort(slot.AllowedMediaTypes)
	if len(slot.AllowedMediaTypes) < 1 || len(slot.AllowedMediaTypes) > 2 {
		return ReferenceOutputSlot{}, errors.New("media types are required")
	}
	for i, mediaType := range slot.AllowedMediaTypes {
		if (mediaType != "image/png" && mediaType != "image/jpeg") || i > 0 && slot.AllowedMediaTypes[i-1] == mediaType {
			return ReferenceOutputSlot{}, errors.New("invalid or duplicate media type")
		}
	}
	if len(slot.SemanticRequirements) == 0 {
		return ReferenceOutputSlot{}, errors.New("semantic requirements are required")
	}
	slot.SemanticRequirements = slices.Clone(slot.SemanticRequirements)
	for i, requirement := range slot.SemanticRequirements {
		if strings.TrimSpace(requirement) != requirement || requirement == "" {
			return ReferenceOutputSlot{}, errors.New("invalid semantic requirement")
		}
		slot.SemanticRequirements[i] = norm.NFC.String(requirement)
	}
	slices.Sort(slot.SemanticRequirements)
	slot.SemanticRequirements = slices.Compact(slot.SemanticRequirements)
	if len(slot.QCRubricRefs) == 0 {
		return ReferenceOutputSlot{}, errors.New("QC rubric references are required")
	}
	slot.QCRubricRefs = slices.Clone(slot.QCRubricRefs)
	slices.SortFunc(slot.QCRubricRefs, func(a, b ReferenceOutputQCRubricRef) int { return strings.Compare(a.ContractID, b.ContractID) })
	for i, ref := range slot.QCRubricRefs {
		if !targetNamePattern.MatchString(ref.ContractID) || !targetHashPattern.MatchString(ref.ContentHash) ||
			i > 0 && slot.QCRubricRefs[i-1].ContractID == ref.ContractID {
			return ReferenceOutputSlot{}, errors.New("invalid or duplicate QC rubric reference")
		}
	}
	return slot, nil
}

func validReferenceAspectRatio(value string) bool {
	a, b, ok := strings.Cut(value, ":")
	if !ok {
		return false
	}
	width, widthErr := strconv.Atoi(a)
	height, heightErr := strconv.Atoi(b)
	if widthErr != nil || heightErr != nil || width < 1 || height < 1 || width > 100 || height > 100 ||
		strconv.Itoa(width) != a || strconv.Itoa(height) != b {
		return false
	}
	for height != 0 {
		width, height = height, width%height
	}
	return width == 1
}

func referenceOutputViewRoles(kind string) []string {
	switch kind {
	case "character_identity_anchor", "character_appearance":
		return []string{"back", "front", "profile"}
	case "location_board":
		return []string{"empty_establishing", "material_scale_detail", "spatial_orientation"}
	case "prop_sheet":
		return []string{"back", "front", "side", "state_detail"}
	case "scene_composition":
		return []string{"composition_master"}
	case "interaction_composition":
		return []string{"interaction_master"}
	default:
		return nil
	}
}
