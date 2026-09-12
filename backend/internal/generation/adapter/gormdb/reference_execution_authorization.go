package gormdb

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
)

type referenceExecutionAuthorizationRepository struct {
	application.ReferenceGenerationTargetReadRepository
	*providerConfigurationRepository
}

func (store *Store) WithinReferenceExecutionAuthorization(ctx context.Context, operation func(application.ReferenceExecutionAuthorizationRepository) error) error {
	if store == nil || store.database == nil || operation == nil {
		return errors.New("Reference execution authorization store is unavailable")
	}
	return platformdatabase.WithinSerializableTransaction(ctx, store.database, func(tx *gorm.DB) error {
		targets := &referenceTargetRepository{referenceAuthorizationRepository{repository{database: tx}}}
		return operation(&referenceExecutionAuthorizationRepository{targets, &providerConfigurationRepository{database: tx}})
	})
}

var _ application.ReferenceExecutionAuthorizationTransactions = (*Store)(nil)
