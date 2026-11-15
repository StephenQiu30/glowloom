package microservices_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/managed"
	executionpg "github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/postgres"
	executionapp "github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
	operationdomain "github.com/StephenQiu30/glowloom-server/backend/execution/operation/domain"
)

func TestOwnedExecutionMediaInputWithoutGrantCannotClaimOrSend(t *testing.T) {
	f, observation := supplierObservationFixture(t)
	if err := f.admin.Exec(`DELETE FROM operation.provider_result WHERE operation_id=?`, observation.Identity.OperationID).Error; err != nil {
		t.Fatal(err)
	}
	asset := uuid.New()
	for _, kind := range []string{"asset", "mask", "subject"} {
		t.Run(kind, func(t *testing.T) {
			input := operationdomain.OperationInput{ID: uuid.New(), OperationID: observation.Identity.OperationID, SeqNo: 0, Role: "reference", RefType: "media_asset"}
			switch kind {
			case "asset":
				input.MediaAssetID = &asset
			case "mask":
				input.MaskAssetID = &asset
			case "subject":
				input.RefType = "subject_media"
				input.MediaAssetID = &asset
			}
			store := &observedActivityStore{Store: executionpg.NewStore(f.execution), loaded: executionapp.WorkflowOperation{ProviderRequestKey: observation.Identity.RequestKey, Provider: executionapp.WorkflowProvider{AdapterKey: "openrouter"}, Inputs: []operationdomain.OperationInput{input}, Operation: operationdomain.Operation{ID: observation.Identity.OperationID, ProjectID: observation.Identity.ProjectID, ModelProfileVersionID: &observation.Identity.ModelProfileVersionID, PriceRuleVersionID: &observation.Identity.PriceRuleVersionID, Status: operationdomain.StatusSubmitting, Capability: "image.generate", Mode: "image_to_image", OutputCount: 1}}}
			results, err := executionapp.NewOwnedManagedResults(store, executionapp.NewArtifactDelivery(&failObservedArtifact{}, unusedArtifactMedia{}))
			if err != nil {
				t.Fatal(err)
			}
			provider := &observedActivityProvider{}
			activity, err := managed.NewOwnedActivities(store, observedActivityKeys{}, provider, results, &ownedMusicDownload{})
			if err != nil {
				t.Fatal(err)
			}
			progress, err := activity.Step(t.Context(), executionapp.ManagedStep{OperationID: observation.Identity.OperationID, Action: "submit"})
			if !errors.Is(err, executionapp.ErrFrozenReferenceUnavailable) || progress.State != executionapp.ManagedProgressUnknown || store.claimed || provider.calls != 0 {
				t.Fatalf("external Media input bypassed grant: progress=%+v err=%v claimed=%v supplier=%d", progress, err, store.claimed, provider.calls)
			}
		})
	}
}
