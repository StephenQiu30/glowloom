package costgormtest

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
)

type ProviderFacts struct {
	ProfileID   uuid.UUID
	BindingID   uuid.UUID
	ProfileHash string
	BindingHash string
}

func SeedProviderFacts(
	t testing.TB,
	database *gorm.DB,
	workspaceID, projectID, ownerID uuid.UUID,
	now time.Time,
	prefix string,
) ProviderFacts {
	t.Helper()
	credentialID, connectionID, profileID, bindingID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	profileHash, bindingHash := strings.Repeat("1", 64), strings.Repeat("2", 64)
	records := []any{
		&model.ProviderCredentialVersion{
			ID: credentialID, WorkspaceID: workspaceID, ConnectionKey: prefix, Revision: 1,
			ProviderKey: "controlled", CipherSuite: "aes-256-gcm", KeyID: "test", Nonce: []byte{1},
			Ciphertext: []byte{2}, SecretFingerprint: strings.Repeat("3", 64), CreatedBy: ownerID, CreatedAt: now,
		},
		&model.ProviderConnectionVersion{
			ID: connectionID, WorkspaceID: workspaceID, ConnectionKey: prefix, Revision: 1,
			SourcePresetKey: "controlled", SourcePresetVersion: 1, PresetSnapshotHash: strings.Repeat("4", 64),
			ProviderKey: "controlled", DisplayName: prefix, CredentialVersionID: credentialID,
			ResolvedConfig: datatypes.JSON([]byte(`{}`)), State: "enabled", AdapterContractVersion: "controlled",
			ContentHash: strings.Repeat("5", 64), CreatedBy: ownerID, CreatedAt: now,
		},
		&model.ProviderModelProfileVersion{
			ID: profileID, WorkspaceID: workspaceID, ProfileKey: prefix, Revision: 1,
			CreationSource: datatypes.JSON([]byte(`{}`)), ConnectionKey: prefix, ProviderKey: "controlled",
			ExternalModelID: "controlled-image", Modality: "image", Family: "controlled",
			AdapterTransportContract: "controlled", CapabilitySchemaVersion: "controlled",
			BillingMetric: "generation.image.call", Defaults: datatypes.JSON([]byte(`{}`)), State: "enabled",
			ContentHash: profileHash, CreatedBy: ownerID, CreatedAt: now,
		},
		&model.ProjectProviderBindingVersion{
			ID: bindingID, WorkspaceID: workspaceID, ProjectID: projectID, Purpose: "shot_frame", Revision: 1,
			ConnectionVersionID: connectionID, CredentialVersionID: credentialID, ModelProfileVersionID: profileID,
			ProviderKey: "controlled", Modality: "image", AdapterContractVersion: "controlled",
			ContentHash: bindingHash, CreatedBy: ownerID, CreatedAt: now,
		},
	}
	for _, record := range records {
		if err := database.Create(record).Error; err != nil {
			t.Fatalf("seed exact provider facts: %v", err)
		}
	}
	return ProviderFacts{
		ProfileID: profileID, BindingID: bindingID, ProfileHash: profileHash, BindingHash: bindingHash,
	}
}

func AssertEstimateImmutable(t testing.TB, database *gorm.DB, estimateID string) {
	t.Helper()
	if err := database.Model(&model.CostEstimate{}).Where("id = ?", estimateID).
		Update("content_hash", strings.Repeat("0", 64)).Error; !errors.Is(err, model.ErrImmutableCostEstimate) {
		t.Fatalf("cost estimate update was not rejected by the ORM boundary: %v", err)
	}
	if err := database.Delete(&model.CostEstimate{ID: uuid.MustParse(estimateID)}).Error; !errors.Is(err, model.ErrImmutableCostEstimate) {
		t.Fatalf("cost estimate delete was not rejected by the ORM boundary: %v", err)
	}
}

func AssertPriceQuoteImmutable(t testing.TB, database *gorm.DB, quoteID string) {
	t.Helper()
	if err := database.Model(&model.CostPriceQuote{}).Where("id = ?", quoteID).
		Update("content_hash", strings.Repeat("0", 64)).Error; !errors.Is(err, model.ErrImmutableCostPriceQuote) {
		t.Fatalf("cost price quote update was not rejected by the ORM boundary: %v", err)
	}
	if err := database.Delete(&model.CostPriceQuote{ID: uuid.MustParse(quoteID)}).Error; !errors.Is(err, model.ErrImmutableCostPriceQuote) {
		t.Fatalf("cost price quote delete was not rejected by the ORM boundary: %v", err)
	}
}
