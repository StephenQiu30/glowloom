package workflow

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.temporal.io/sdk/temporal"

	"github.com/StephenQiu30/lanverse/backend/internal/operation/application"
)

// ProviderImageIdentityReader reads the latest explicitly gated frozen call.
type ProviderImageIdentityReader interface {
	LoadProviderImageIdentity(context.Context, uuid.UUID) (application.ProviderDispatchIdentity, bool, error)
}

// ProviderImageRecoverer verifies a complete manifest without starting a turn.
type ProviderImageRecoverer interface {
	RecoverImage(context.Context, application.ProviderDispatchIdentity) (*application.ProviderReceipt, error)
}

// NewActivitiesWithImageRecovery injects recovery independently of paid execution.
// Disabling the execution worker must not discard existing private manifests.
func NewActivitiesWithImageRecovery(store Store, finalizer Finalizer, identities ProviderImageIdentityReader, recovery ProviderImageRecoverer) *Activities {
	return &Activities{store: store, finalizer: finalizer, imageIdentity: identities, imageRecovery: recovery}
}

// RecoverProviderImage only verifies the same call's durable manifest and bytes.
// The payload contains no artifact path, task ID, price, or new sending right.
func (a *Activities) RecoverProviderImage(ctx context.Context, operationID string) (*application.ProviderReceipt, error) {
	id, err := uuid.Parse(operationID)
	if err != nil || id.String() != operationID {
		return nil, ErrInvalidOperationInput
	}
	if a.imageIdentity == nil || a.imageRecovery == nil {
		return nil, temporal.NewNonRetryableApplicationError("private image recovery is unavailable", "image_recovery_unavailable", nil)
	}
	identity, found, err := a.imageIdentity.LoadProviderImageIdentity(ctx, id)
	if err != nil {
		return nil, permanentActivityError(err)
	}
	if !found {
		return nil, nil
	}
	receipt, err := a.imageRecovery.RecoverImage(ctx, identity)
	if err != nil {
		return nil, permanentActivityError(fmt.Errorf("recover provider image evidence: %w", err))
	}
	if receipt != nil && (receipt.Validate() != nil || receipt.Identity != identity) {
		return nil, permanentActivityError(application.ErrProviderCallConflict)
	}
	return receipt, nil
}
