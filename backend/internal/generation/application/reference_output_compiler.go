package application

import (
	"errors"
	"fmt"
	"slices"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
)

type ReferenceOutputSlotPolicy struct {
	ViewRole          string
	AllowedMediaTypes []string
	AspectRatio       string
	MinWidth          int
	MinHeight         int
	MaxBytes          int64
}

// CompileReferenceOutputContract validates the Brief's frozen input fence. The
// caller must separately verify its accepted revision and current Owner heads.
func CompileReferenceOutputContract(
	input agentcontract.ReferenceBriefInput,
	brief agentcontract.ReferenceBriefCandidate,
	bundleCount int,
	policies []ReferenceOutputSlotPolicy,
) (domain.ReferenceOutputContract, error) {
	if err := brief.ValidateFor(input); err != nil {
		return domain.ReferenceOutputContract{}, fmt.Errorf("Reference output Brief fence: %w", err)
	}
	if len(policies) != len(brief.RequiredViewRoles) {
		return domain.ReferenceOutputContract{}, errors.New("Reference output policies do not cover required views")
	}
	requirements := slices.Clone(brief.PositiveInstructions)
	requirements = append(requirements, brief.LayoutRequirements...)
	requirements = append(requirements, brief.ScaleRequirements...)
	rubrics := make([]domain.ReferenceOutputQCRubricRef, len(brief.QCRubricRefs))
	for i, ref := range brief.QCRubricRefs {
		rubrics[i] = domain.ReferenceOutputQCRubricRef{ContractID: ref.ContractID, ContentHash: ref.ContentHash}
	}
	slots := make([]domain.ReferenceOutputSlot, len(policies))
	for i, policy := range policies {
		slots[i] = domain.ReferenceOutputSlot{
			SlotKey: policy.ViewRole, ViewRole: policy.ViewRole, Required: true,
			AllowedMediaTypes: policy.AllowedMediaTypes, AspectRatio: policy.AspectRatio,
			MinWidth: policy.MinWidth, MinHeight: policy.MinHeight, MaxBytes: policy.MaxBytes,
			SemanticRequirements: requirements, QCRubricRefs: rubrics,
		}
	}
	return domain.BuildReferenceOutputContract(input.TargetKind, bundleCount, slots)
}
