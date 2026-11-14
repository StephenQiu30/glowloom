package microservices_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

func TestAuthorityStandaloneBeginCannotRetainCallerTransactionAcrossControl(t *testing.T) {
	database := authorityDatabase(t, "identity")
	authority := writefence.Authority{Issuer: writefence.Identity, Kind: writefence.Session, ID: uuid.New()}
	if err := database.WithContext(t.Context()).Transaction(func(tx *gorm.DB) error {
		_, err := pgfence.NewAuthorityStore(tx).Begin(t.Context(), authority, uuid.New(), true)
		if !errors.Is(err, writefence.ErrTransaction) {
			t.Fatalf("standalone Begin returned without committing source closure: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
