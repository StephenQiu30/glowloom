package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	GenerationTargetReferenceAsset = "reference_asset"
	GenerationTargetShotFrame      = "shot_frame"
)

var (
	targetHashPattern       = regexp.MustCompile(`^[0-9a-f]{64}$`)
	targetNamePattern       = regexp.MustCompile(`^[a-z][a-z0-9._-]{1,119}$`)
	targetPromptNamePattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{2,119}$`)
)

type FrozenOwnerReference struct {
	Owner       string `json:"owner"`
	Resource    string `json:"resource"`
	ID          string `json:"id"`
	Revision    int64  `json:"revision"`
	ContentHash string `json:"content_hash"`
}

type ReferenceAssetTarget struct {
	AssetID                 string               `json:"asset_id"`
	AssetKind               string               `json:"asset_kind"`
	SpecificationVersionRef FrozenOwnerReference `json:"specification_version_ref"`
	AssetStateRef           FrozenOwnerReference `json:"asset_state_ref"`
	OutputKind              string               `json:"output_kind"`
	RequiredViewRoles       []string             `json:"required_view_roles"`
	PromptVersion           string               `json:"prompt_version"`
	PositivePrompt          string               `json:"positive_prompt"`
	NegativePrompt          string               `json:"negative_prompt"`
	Width                   int                  `json:"width"`
	Height                  int                  `json:"height"`
	NumberResults           int                  `json:"number_results"`
	OutputFormat            string               `json:"output_format"`
}

type ShotFrameTarget struct {
	ShotRef                         FrozenOwnerReference   `json:"shot_ref"`
	ShotProductionBindingVersionRef FrozenOwnerReference   `json:"shot_production_binding_version_ref"`
	ExactAssetVersionRefs           []FrozenOwnerReference `json:"exact_asset_version_refs"`
	FrameRole                       string                 `json:"frame_role"`
	PromptVersion                   string                 `json:"prompt_version"`
	PositivePrompt                  string                 `json:"positive_prompt"`
	NegativePrompt                  string                 `json:"negative_prompt"`
	Width                           int                    `json:"width"`
	Height                          int                    `json:"height"`
	NumberResults                   int                    `json:"number_results"`
	OutputFormat                    string                 `json:"output_format"`
}

type GenerationTarget struct {
	ID, WorkspaceID, ProjectID string
	Kind                       string
	SourceOwnerRef             FrozenOwnerReference
	PolicySnapshotRef          FrozenOwnerReference
	ReferenceAsset             *ReferenceAssetTarget
	ShotFrame                  *ShotFrameTarget
	TargetHash                 string
	Revision                   int64
	CreatedBy                  string
	CreatedAt                  time.Time
}

type GenerationTargetInput struct {
	ID, WorkspaceID, ProjectID string
	Kind                       string
	SourceOwnerRef             FrozenOwnerReference
	PolicySnapshotRef          FrozenOwnerReference
	ReferenceAsset             *ReferenceAssetTarget
	ShotFrame                  *ShotFrameTarget
	Revision                   int64
	CreatedBy                  string
	CreatedAt                  time.Time
}

type generationTargetHashInput struct {
	WorkspaceID       string                `json:"workspace_id"`
	ProjectID         string                `json:"project_id"`
	Kind              string                `json:"kind"`
	SourceOwnerRef    FrozenOwnerReference  `json:"source_owner_ref"`
	PolicySnapshotRef FrozenOwnerReference  `json:"policy_snapshot_ref"`
	ReferenceAsset    *ReferenceAssetTarget `json:"reference_asset,omitempty"`
	ShotFrame         *ShotFrameTarget      `json:"shot_frame,omitempty"`
	Revision          int64                 `json:"revision"`
}

func NewGenerationTarget(input GenerationTargetInput) (GenerationTarget, error) {
	input.ID, input.WorkspaceID, input.ProjectID = strings.TrimSpace(input.ID), strings.TrimSpace(input.WorkspaceID), strings.TrimSpace(input.ProjectID)
	input.Kind, input.CreatedBy = strings.TrimSpace(input.Kind), strings.TrimSpace(input.CreatedBy)
	input.SourceOwnerRef = normalizeFrozenOwnerReference(input.SourceOwnerRef)
	input.PolicySnapshotRef = normalizeFrozenOwnerReference(input.PolicySnapshotRef)
	for _, identifier := range []string{input.ID, input.WorkspaceID, input.ProjectID, input.CreatedBy} {
		if _, err := uuid.Parse(identifier); err != nil {
			return GenerationTarget{}, errors.New("invalid GenerationTarget identity")
		}
	}
	if input.Revision != 1 || input.CreatedAt.IsZero() || !validFrozenOwnerReference(input.SourceOwnerRef) ||
		!validFrozenOwnerReference(input.PolicySnapshotRef) ||
		input.SourceOwnerRef.Owner != "storyboard" || input.SourceOwnerRef.Resource != "approved_storyboard_intents" ||
		input.PolicySnapshotRef.Resource != "effective_style_snapshot" {
		return GenerationTarget{}, errors.New("invalid GenerationTarget owner snapshot")
	}
	if (input.ReferenceAsset == nil) == (input.ShotFrame == nil) {
		return GenerationTarget{}, errors.New("GenerationTarget must contain exactly one payload")
	}
	var referenceAsset *ReferenceAssetTarget
	var shotFrame *ShotFrameTarget
	switch input.Kind {
	case GenerationTargetReferenceAsset:
		if input.ReferenceAsset == nil || input.ShotFrame != nil {
			return GenerationTarget{}, errors.New("reference_asset target has the wrong payload")
		}
		normalized, err := normalizeReferenceAssetTarget(*input.ReferenceAsset)
		if err != nil {
			return GenerationTarget{}, err
		}
		referenceAsset = &normalized
	case GenerationTargetShotFrame:
		if input.ShotFrame == nil || input.ReferenceAsset != nil {
			return GenerationTarget{}, errors.New("shot_frame target has the wrong payload")
		}
		normalized, err := normalizeShotFrameTarget(*input.ShotFrame)
		if err != nil {
			return GenerationTarget{}, err
		}
		shotFrame = &normalized
	default:
		return GenerationTarget{}, errors.New("unsupported GenerationTarget kind")
	}
	target := GenerationTarget{
		ID: input.ID, WorkspaceID: input.WorkspaceID, ProjectID: input.ProjectID, Kind: input.Kind,
		SourceOwnerRef: input.SourceOwnerRef, PolicySnapshotRef: input.PolicySnapshotRef,
		ReferenceAsset: referenceAsset, ShotFrame: shotFrame, Revision: input.Revision,
		CreatedBy: input.CreatedBy, CreatedAt: input.CreatedAt.UTC().Truncate(time.Microsecond),
	}
	var err error
	target.TargetHash, err = generationTargetHash(target)
	return target, err
}

func ValidateGenerationTarget(target GenerationTarget) error {
	rebuilt, err := NewGenerationTarget(GenerationTargetInput{
		ID: target.ID, WorkspaceID: target.WorkspaceID, ProjectID: target.ProjectID, Kind: target.Kind,
		SourceOwnerRef: target.SourceOwnerRef, PolicySnapshotRef: target.PolicySnapshotRef,
		ReferenceAsset: target.ReferenceAsset, ShotFrame: target.ShotFrame, Revision: target.Revision,
		CreatedBy: target.CreatedBy, CreatedAt: target.CreatedAt,
	})
	if err != nil || rebuilt.TargetHash != target.TargetHash || !targetHashPattern.MatchString(target.TargetHash) ||
		!reflect.DeepEqual(rebuilt, target) {
		return errors.New("GenerationTarget has drifted")
	}
	return nil
}

func normalizeReferenceAssetTarget(value ReferenceAssetTarget) (ReferenceAssetTarget, error) {
	value.AssetID, value.AssetKind = strings.TrimSpace(value.AssetID), strings.TrimSpace(value.AssetKind)
	value.SpecificationVersionRef = normalizeFrozenOwnerReference(value.SpecificationVersionRef)
	value.AssetStateRef = normalizeFrozenOwnerReference(value.AssetStateRef)
	value.OutputKind, value.PromptVersion = strings.TrimSpace(value.OutputKind), strings.TrimSpace(value.PromptVersion)
	value.PositivePrompt, value.NegativePrompt = strings.TrimSpace(value.PositivePrompt), strings.TrimSpace(value.NegativePrompt)
	value.OutputFormat = strings.ToUpper(strings.TrimSpace(value.OutputFormat))
	roles := make([]string, len(value.RequiredViewRoles))
	for index, role := range value.RequiredViewRoles {
		roles[index] = strings.TrimSpace(role)
	}
	roleOrder := map[string]int{"front": 0, "profile": 1, "back": 2}
	slices.SortFunc(roles, func(left, right string) int { return roleOrder[left] - roleOrder[right] })
	value.RequiredViewRoles = roles
	if _, err := uuid.Parse(value.AssetID); err != nil || value.AssetKind != "character" ||
		value.OutputKind != "reference_sheet" || !slices.Equal(roles, []string{"front", "profile", "back"}) ||
		!validFrozenOwnerReference(value.SpecificationVersionRef) ||
		value.SpecificationVersionRef.Owner != "production" ||
		value.SpecificationVersionRef.Resource != "production_bible_specification_version" ||
		!validFrozenOwnerReference(value.AssetStateRef) || value.AssetStateRef.Owner != "asset" ||
		value.AssetStateRef.Resource != "asset_state" || value.PromptVersion != "character-reference-sheet-v1" ||
		len(value.PositivePrompt) < 2 || len(value.PositivePrompt) > 10_000 ||
		len(value.NegativePrompt) < 2 || len(value.NegativePrompt) > 3_000 ||
		value.Width != 1536 || value.Height != 1024 || value.NumberResults != 4 || value.OutputFormat != "PNG" {
		return ReferenceAssetTarget{}, errors.New("invalid character reference_asset target")
	}
	return value, nil
}

func normalizeShotFrameTarget(value ShotFrameTarget) (ShotFrameTarget, error) {
	value.ShotRef = normalizeFrozenOwnerReference(value.ShotRef)
	value.ShotProductionBindingVersionRef = normalizeFrozenOwnerReference(value.ShotProductionBindingVersionRef)
	value.FrameRole, value.PromptVersion = strings.TrimSpace(value.FrameRole), strings.TrimSpace(value.PromptVersion)
	value.PositivePrompt, value.NegativePrompt = strings.TrimSpace(value.PositivePrompt), strings.TrimSpace(value.NegativePrompt)
	value.OutputFormat = strings.ToUpper(strings.TrimSpace(value.OutputFormat))
	refs := append([]FrozenOwnerReference(nil), value.ExactAssetVersionRefs...)
	for index := range refs {
		refs[index] = normalizeFrozenOwnerReference(refs[index])
	}
	slices.SortFunc(refs, func(left, right FrozenOwnerReference) int { return strings.Compare(left.ID, right.ID) })
	value.ExactAssetVersionRefs = refs
	if !validFrozenOwnerReference(value.ShotRef) || value.ShotRef.Owner != "storyboard" || value.ShotRef.Resource != "shot" ||
		!validFrozenOwnerReference(value.ShotProductionBindingVersionRef) ||
		value.ShotProductionBindingVersionRef.Owner != "storyboard" ||
		value.ShotProductionBindingVersionRef.Resource != "shot_production_binding_version" ||
		len(refs) == 0 || !oneOf(value.FrameRole, "first", "key", "last") ||
		!targetPromptNamePattern.MatchString(value.PromptVersion) || len(value.PositivePrompt) < 2 ||
		len(value.PositivePrompt) > 10_000 || len(value.NegativePrompt) < 2 || len(value.NegativePrompt) > 3_000 ||
		value.Width < 128 || value.Width > 2048 || value.Width%16 != 0 ||
		value.Height < 128 || value.Height > 2048 || value.Height%16 != 0 ||
		value.NumberResults != 4 || value.OutputFormat != "PNG" {
		return ShotFrameTarget{}, errors.New("invalid shot_frame target")
	}
	for index, reference := range refs {
		if !validFrozenOwnerReference(reference) || reference.Owner != "asset" || reference.Resource != "asset_version" ||
			(index > 0 && refs[index-1].ID == reference.ID) {
			return ShotFrameTarget{}, errors.New("invalid shot_frame AssetVersion reference")
		}
	}
	return value, nil
}

func normalizeFrozenOwnerReference(value FrozenOwnerReference) FrozenOwnerReference {
	value.Owner, value.Resource, value.ID = strings.TrimSpace(value.Owner), strings.TrimSpace(value.Resource), strings.TrimSpace(value.ID)
	value.ContentHash = strings.TrimSpace(value.ContentHash)
	return value
}

func validFrozenOwnerReference(value FrozenOwnerReference) bool {
	_, idErr := uuid.Parse(value.ID)
	return idErr == nil && targetNamePattern.MatchString(value.Owner) && targetNamePattern.MatchString(value.Resource) &&
		value.Revision >= 1 && targetHashPattern.MatchString(value.ContentHash)
}

func generationTargetHash(value GenerationTarget) (string, error) {
	encoded, err := json.Marshal(generationTargetHashInput{
		WorkspaceID: value.WorkspaceID, ProjectID: value.ProjectID, Kind: value.Kind,
		SourceOwnerRef: value.SourceOwnerRef, PolicySnapshotRef: value.PolicySnapshotRef,
		ReferenceAsset: value.ReferenceAsset, ShotFrame: value.ShotFrame, Revision: value.Revision,
	})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func oneOf(value string, allowed ...string) bool {
	return slices.Contains(allowed, value)
}
