package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// instantiatePresetModels shares the connection's outer transaction and receipt.
// Nothing becomes public and no validation or default is copied from the source.
func (s *Store) instantiatePresetModels(ctx context.Context, tx *gorm.DB, actor identityapp.Principal, providerID, configID uuid.UUID, doc domain.PresetDocument) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	for _, template := range doc.ModelTemplates {
		id := uuid.New()
		model := domain.ModelProfile{ID: id, Key: "preset-" + id.String(), ProviderID: providerID, Capability: template.Capability, DisplayName: template.Name, Status: domain.ModelDisabled, Revision: 1}
		if err := createModelInTx(tx, model); err != nil {
			return nil, err
		}
		version := template.Version(id, actor.ID)
		if err := appendModelVersionInTx(tx, version, 1); err != nil {
			return nil, err
		}
		price := template.PriceVersion(id, actor.ID, time.Now().UTC())
		if err := appendPriceRuleInTx(tx, price, 2); err != nil {
			return nil, err
		}
		cost := template.UpstreamCost
		_, err := application.NewModelReleases(s.withDB(tx)).Create(ctx, actor, id, application.CreateReleaseInput{ModelVersionID: version.ID, PriceVersionID: price.ID, ConfigVersionID: configID, UpstreamCost: application.UpstreamCost{Currency: cost.Currency, MaxMicros: cost.MaxMicros, Source: cost.Source, ReviewedAt: cost.ReviewedAt}, RequestID: uuid.New()})
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}
