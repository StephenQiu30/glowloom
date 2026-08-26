package gormdb_test

import (
	"context"
	"strings"
	"testing"

	"gorm.io/gorm"

	costapp "github.com/StephenQiu30/lanverse/backend/internal/cost/application"
	generationgorm "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/gormdb"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	quotaapp "github.com/StephenQiu30/lanverse/backend/internal/quota/application"
)

func TestPreparationStoreRejectsDisabledNestedTransactions(t *testing.T) {
	store := generationgorm.NewPreparationStore(
		&gorm.DB{Config: &gorm.Config{DisableNestedTransaction: true}}, costapp.Config{}, quotaapp.Config{},
	)
	err := store.WithinPreparationTransaction(context.Background(), func(
		application.PreparationRepository,
		application.CostPreparationOwner,
		application.QuotaPreparationOwner,
	) error {
		t.Fatal("preparation operation ran without nested transaction savepoints")
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "requires GORM nested transaction savepoints") {
		t.Fatalf("disabled nested transactions were accepted: %T %v", err, err)
	}
}
