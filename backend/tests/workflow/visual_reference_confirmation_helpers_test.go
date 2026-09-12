package workflow_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	referenceapp "github.com/StephenQiu30/lanverse/backend/internal/production/reference/application"
	referencedomain "github.com/StephenQiu30/lanverse/backend/internal/production/reference/domain"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	workflowdomain "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func seedVisualFoundationImageCapability(
	t *testing.T,
	create func(any) error,
	fixture sceneAnalysisFixture,
	now time.Time,
) {
	t.Helper()
	credentialID, connectionID, profileID, bindingID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	const (
		connectionKey   = "visual-foundation-reference-assets"
		providerKey     = "controlled-image-test"
		adapterContract = "controlled-image-test-contract"
	)
	records := []any{
		&model.ProviderCredentialVersion{
			ID: credentialID, WorkspaceID: fixture.workspaceID, ConnectionKey: connectionKey, Revision: 1,
			ProviderKey: providerKey, CipherSuite: "aes-256-gcm", KeyID: "test-key",
			Nonce: []byte("0123456789ab"), Ciphertext: []byte("test-ciphertext"),
			SecretFingerprint: sceneTextHash("visual-foundation-provider-secret"),
			CreatedBy:         fixture.userID, CreatedAt: now,
		},
		&model.ProviderConnectionVersion{
			ID: connectionID, WorkspaceID: fixture.workspaceID, ConnectionKey: connectionKey, Revision: 1,
			SourcePresetKey: "controlled.image", SourcePresetVersion: 1,
			PresetSnapshotHash: sceneTextHash("visual-foundation-provider-preset"),
			ProviderKey:        providerKey, DisplayName: "Visual Foundation test image provider",
			CredentialVersionID: credentialID, ResolvedConfig: []byte(`{}`),
			State: "enabled", AdapterContractVersion: adapterContract,
			ContentHash: sceneTextHash("visual-foundation-provider-connection"),
			CreatedBy:   fixture.userID, CreatedAt: now,
		},
		&model.ProviderModelProfileVersion{
			ID: profileID, WorkspaceID: fixture.workspaceID, ProfileKey: "visual-foundation-reference-image", Revision: 1,
			CreationSource: []byte(`{"kind":"test"}`), ConnectionKey: connectionKey,
			ProviderKey: providerKey, ExternalModelID: "controlled-image", Modality: "image", Family: "controlled_image",
			AdapterTransportContract: adapterContract, CapabilitySchemaVersion: adapterContract,
			BillingMetric: "generation.image.call", Defaults: []byte(`{}`), State: "enabled",
			ContentHash: sceneTextHash("visual-foundation-provider-profile"),
			CreatedBy:   fixture.userID, CreatedAt: now,
		},
		&model.ProjectProviderBindingVersion{
			ID: bindingID, WorkspaceID: fixture.workspaceID, ProjectID: fixture.projectID,
			Purpose: "reference_asset", Revision: 1, ConnectionVersionID: connectionID,
			CredentialVersionID: credentialID, ModelProfileVersionID: profileID,
			ProviderKey: providerKey, Modality: "image", AdapterContractVersion: adapterContract,
			ContentHash: sceneTextHash("visual-foundation-provider-binding"),
			CreatedBy:   fixture.userID, CreatedAt: now,
		},
	}
	for _, record := range records {
		if err := create(record); err != nil {
			t.Fatalf("seed Visual Foundation image capability %T: %v", record, err)
		}
	}
}

func visualReferenceTargetDrafts(projection workflowapp.ReferencePlanCandidateProjection) []referencedomain.TargetDraft {
	result := make([]referencedomain.TargetDraft, len(projection.Targets))
	for index, target := range projection.Targets {
		result[index] = referencedomain.TargetDraft{
			TargetBusinessKey: target.TargetBusinessKey,
			TargetKind:        target.TargetKind,
			Fulfillment:       target.Fulfillment,
			OwnerRefs: agentcontract.ReferencePlanTargetOwnerRefs{
				Identity:      append([]agentcontract.ReferencePlanOwnerRef{}, target.OwnerRefs.Identity...),
				Specification: append([]agentcontract.ReferencePlanOwnerRef{}, target.OwnerRefs.Specification...),
				State:         append([]agentcontract.ReferencePlanOwnerRef{}, target.OwnerRefs.State...),
				Scene:         append([]agentcontract.ReferencePlanOwnerRef{}, target.OwnerRefs.Scene...),
				Occurrence:    append([]agentcontract.ReferencePlanOwnerRef{}, target.OwnerRefs.Occurrence...),
				Interaction:   append([]agentcontract.ReferencePlanOwnerRef{}, target.OwnerRefs.Interaction...),
			},
			CoverageScopeKeys:           append([]string(nil), target.CoverageScopeKeys...),
			DependsOnTargetBusinessKeys: append([]string(nil), target.DependsOnTargetBusinessKeys...),
			Constraints: referencedomain.TargetConstraints{
				ProductionWorldOwnerSetHash:           target.Constraints.ProductionWorldOwnerSetHash,
				ReferenceTargetSeedRoot:               target.Constraints.ReferenceTargetSeedRoot,
				VisualFoundationCandidateRevisionID:   target.Constraints.VisualFoundationCandidateRevisionID,
				VisualFoundationCandidateRevisionHash: target.Constraints.VisualFoundationCandidateRevisionHash,
				PresetReleaseContentHash:              target.Constraints.PresetReleaseContentHash,
				DesignFocus:                           append([]string(nil), target.Constraints.DesignFocus...),
				ForbiddenChanges:                      append([]string(nil), target.Constraints.ForbiddenChanges...),
			},
		}
	}
	return result
}

func visualReferenceExpectedHead(
	t *testing.T,
	gate workflowdomain.VisualFoundationScopeGateInput,
	ownerKind string,
) referenceapp.ExpectedHead {
	t.Helper()
	for _, head := range gate.EffectPlan.AtomicStep.ExpectedHeads {
		if head.OwnerKind == ownerKind {
			return referenceapp.ExpectedHead{
				OwnerKind: head.OwnerKind, LogicalID: head.LogicalID,
				Revision: head.Revision, ContentHash: head.ContentHash,
			}
		}
	}
	t.Fatalf("Visual Foundation Scope Gate is missing %s expected head", ownerKind)
	return referenceapp.ExpectedHead{}
}
