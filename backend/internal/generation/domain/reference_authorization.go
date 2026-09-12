package domain

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	owner "github.com/StephenQiu30/lanverse/backend/internal/platform/ownercollection"
)

type InitialReferenceGenerationAuthorizationInput struct {
	ApprovedReferencePlanVersionRef owner.VersionRef `json:"approved_reference_plan_version_ref"`
	ReferencePlanTargetRef          owner.VersionRef `json:"reference_plan_target_ref"`
	RequestedCandidateBundleCount   int              `json:"requested_candidate_bundle_count"`
	HumanActionRef                  string           `json:"human_action_ref"`
	MembershipTokenVersion          int              `json:"membership_token_version"`
	AuthorizedBy                    string           `json:"authorized_by"`
	AuthorizedAt                    time.Time        `json:"authorized_at"`
}

type ReferenceGenerationAuthorization struct {
	ContractID string `json:"contract_id"`
	Kind       string `json:"kind"`
	InitialReferenceGenerationAuthorizationInput
	BaseGenerationTargetRef *owner.VersionRef `json:"base_generation_target_ref"`
	BaseCandidateSetRef     *owner.VersionRef `json:"base_candidate_set_ref"`
	ReasonCode              string            `json:"reason_code"`
	ContentHash             string            `json:"content_hash"`
}

func BuildInitialReferenceGenerationAuthorization(input InitialReferenceGenerationAuthorizationInput) (ReferenceGenerationAuthorization, error) {
	plan, target := input.ApprovedReferencePlanVersionRef, input.ReferencePlanTargetRef
	for _, id := range []string{plan.WorkspaceID, plan.ProjectID, plan.OwnerLogicalID, plan.OwnerVersionID, target.OwnerVersionID, input.HumanActionRef, input.AuthorizedBy} {
		parsed, err := uuid.Parse(id)
		if err != nil || parsed == uuid.Nil || parsed.String() != id {
			return ReferenceGenerationAuthorization{}, errors.New("invalid Reference generation authorization identity")
		}
	}
	if target.OwnerLogicalID == "" || strings.TrimSpace(target.OwnerLogicalID) != target.OwnerLogicalID ||
		plan.OwnerKind != "production/reference" || plan.VersionFamily != "reference_plan_set" || plan.OwnerRevision < 1 || plan.OwnerRevision > 9007199254740991 ||
		target.OwnerKind != plan.OwnerKind || target.VersionFamily != plan.VersionFamily || target.WorkspaceID != plan.WorkspaceID || target.ProjectID != plan.ProjectID ||
		target.OwnerRevision != plan.OwnerRevision || target.OwnerVersionID == plan.OwnerVersionID || !targetHashPattern.MatchString(plan.OwnerContentHash) || !targetHashPattern.MatchString(target.OwnerContentHash) ||
		input.RequestedCandidateBundleCount < 1 || input.RequestedCandidateBundleCount > 4 || input.MembershipTokenVersion < 1 || input.AuthorizedAt.IsZero() {
		return ReferenceGenerationAuthorization{}, errors.New("invalid initial Reference generation authorization")
	}
	input.AuthorizedAt = input.AuthorizedAt.UTC().Truncate(time.Microsecond)
	value := ReferenceGenerationAuthorization{ContractID: "reference-generation-authorization-production", Kind: "initial_generation", InitialReferenceGenerationAuthorizationInput: input, ReasonCode: "initial_generation"}
	raw, err := json.Marshal(value)
	if err != nil {
		return ReferenceGenerationAuthorization{}, err
	}
	value.ContentHash, err = canonical.Hash(raw)
	return value, err
}

func DecodeReferenceGenerationAuthorization(raw json.RawMessage) (ReferenceGenerationAuthorization, error) {
	var value ReferenceGenerationAuthorization
	if err := canonical.Decode(raw, &value); err != nil {
		return ReferenceGenerationAuthorization{}, err
	}
	rebuilt, err := BuildInitialReferenceGenerationAuthorization(value.InitialReferenceGenerationAuthorizationInput)
	if err != nil || !reflect.DeepEqual(value, rebuilt) {
		return ReferenceGenerationAuthorization{}, errors.New("Reference generation authorization has drifted")
	}
	return value, nil
}
