package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	"github.com/google/uuid"
)

// ReferenceCandidateBundle records a reviewed whole group, not approval for
// selection or publication. Vision findings remain owned by the exact revision.
type ReferenceCandidateBundle struct {
	ContractID           string                `json:"contract_id"`
	ID                   string                `json:"candidate_bundle_id"`
	WorkspaceID          string                `json:"workspace_id"`
	ProjectID            string                `json:"project_id"`
	TargetRef            GenerationRevisionRef `json:"generation_target_ref"`
	ExecutionRef         GenerationRevisionRef `json:"execution_ref"`
	GenerationRound      int64                 `json:"generation_round"`
	CandidateBundleIndex int                   `json:"candidate_bundle_index"`
	BundleInputRef       GenerationActionRef   `json:"bundle_input_ref"`
	VisionReviewRef      GenerationRevisionRef `json:"bundle_vision_review_candidate_revision_ref"`
	DependencyRootHash   string                `json:"dependency_root_hash"`
	CreatedAt            time.Time             `json:"created_at"`
	ContentHash          string                `json:"content_hash"`
}

// The caller must authenticate and verify the persisted review and its current
// fences. A revision reference alone cannot establish that authorization.
func BuildReferenceCandidateBundle(workspaceID, projectID string, inputs ReferenceBundleInputCollection, index int, review GenerationRevisionRef, at time.Time) (ReferenceCandidateBundle, error) {
	raw, err := json.Marshal(inputs)
	if err != nil {
		return ReferenceCandidateBundle{}, err
	}
	if _, err = DecodeReferenceBundleInputs(raw); err != nil {
		return ReferenceCandidateBundle{}, err
	}
	if index < 0 || index >= len(inputs.Bundles) || inputs.Bundles[index].Admission.ValidateInternalReview() != nil {
		return ReferenceCandidateBundle{}, errors.New("candidate Bundle requires a complete technically valid group")
	}
	input := inputs.Bundles[index].Input
	value := ReferenceCandidateBundle{ContractID: "generation-candidate-bundle-production", WorkspaceID: workspaceID, ProjectID: projectID, TargetRef: input.TargetRef, ExecutionRef: input.ExecutionRef, GenerationRound: input.GenerationRound, CandidateBundleIndex: index, BundleInputRef: GenerationActionRef{ID: input.ID, ContentHash: input.ContentHash}, VisionReviewRef: review, DependencyRootHash: input.DependencyRootHash, CreatedAt: at.UTC().Truncate(time.Microsecond)}
	value.ID = referenceCandidateBundleID(value)
	return buildReferenceCandidateBundle(value)
}

func referenceCandidateBundleID(v ReferenceCandidateBundle) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("reference-candidate-bundle:"+v.WorkspaceID+":"+v.ProjectID+":"+v.BundleInputRef.ID+":"+v.BundleInputRef.ContentHash+":"+v.VisionReviewRef.ID+":"+v.VisionReviewRef.ContentHash)).String()
}

func buildReferenceCandidateBundle(v ReferenceCandidateBundle) (ReferenceCandidateBundle, error) {
	for _, id := range []string{v.ID, v.WorkspaceID, v.ProjectID} {
		if !(GenerationRevisionRef{ID: id, Revision: 1, ContentHash: v.DependencyRootHash}).Valid() {
			return ReferenceCandidateBundle{}, errors.New("invalid candidate Bundle scope")
		}
	}
	if v.ContractID != "generation-candidate-bundle-production" || !v.TargetRef.Valid() || !v.ExecutionRef.Valid() || v.ExecutionRef.Revision != 1 || !v.BundleInputRef.Valid() || !v.VisionReviewRef.Valid() || v.VisionReviewRef.Revision != 1 || v.GenerationRound != 1 || v.CandidateBundleIndex < 0 || v.CandidateBundleIndex > 3 || v.CreatedAt.IsZero() || v.ID != referenceCandidateBundleID(v) || !v.CreatedAt.Equal(v.CreatedAt.Truncate(time.Microsecond)) {
		return ReferenceCandidateBundle{}, errors.New("invalid candidate Bundle identity")
	}
	v.ContentHash = ""
	hash, err := referenceBundleHash(v)
	v.ContentHash = hash
	return v, err
}

func DecodeReferenceCandidateBundle(raw json.RawMessage) (ReferenceCandidateBundle, error) {
	var value ReferenceCandidateBundle
	if err := canonical.Decode(raw, &value); err != nil {
		return value, err
	}
	expected, err := buildReferenceCandidateBundle(value)
	if err != nil || expected.ContentHash != value.ContentHash {
		return ReferenceCandidateBundle{}, errors.New("candidate Bundle content drifted")
	}
	typed, err := json.Marshal(expected)
	if err != nil {
		return ReferenceCandidateBundle{}, err
	}
	left, err := canonical.JSON(raw)
	if err != nil {
		return ReferenceCandidateBundle{}, err
	}
	right, err := canonical.JSON(typed)
	if err != nil || !bytes.Equal(left, right) {
		return ReferenceCandidateBundle{}, errors.New("candidate Bundle wire shape is incomplete")
	}
	return expected, nil
}
