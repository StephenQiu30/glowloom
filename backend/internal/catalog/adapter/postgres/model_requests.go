package postgres

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

func catalogEventRequest(event identityapp.OutboxEvent) uuid.UUID {
	var payload struct {
		Data struct {
			RequestID string `json:"request_id"`
		} `json:"data"`
	}
	if json.Unmarshal(event.Payload, &payload) != nil {
		return uuid.Nil
	}
	id, _ := uuid.Parse(payload.Data.RequestID)
	return id
}

// CreateModelWithAudit commits a model once and retains its original safe receipt.
func (s *Store) CreateModelWithAudit(ctx context.Context, actor identityapp.Principal, model domain.ModelProfile, event identityapp.OutboxEvent) (domain.ModelProfile, error) {
	request := catalogEventRequest(event)
	if s == nil || s.db == nil || request == uuid.Nil || !validModelCreatedAudit(actor.ID, actor.OrgID, model, event) {
		return domain.ModelProfile{}, application.ErrInvalidCreateModel
	}

	return catalogRequest(ctx, s.db, s.actors, actor, "model.create", uuid.Nil, request, model, false, func(tx *gorm.DB) (domain.ModelProfile, error) {
		return s.withDB(tx).createModelWithAuditOnce(ctx, actor, model, event)
	})
}

// PublishModelVersionWithAudit persists configuration drafts with replay protection.
func (s *Store) PublishModelVersionWithAudit(ctx context.Context, actor identityapp.Principal, version domain.ModelVersion, revision int64, event identityapp.OutboxEvent) (domain.ModelVersion, error) {
	request := catalogEventRequest(event)
	if s == nil || s.db == nil || request == uuid.Nil || !validModelVersionPublishedAudit(actor.ID, actor.OrgID, version, revision, event) {
		return domain.ModelVersion{}, application.ErrInvalidPublishModelVersion
	}

	input := struct {
		Version  domain.ModelVersion
		Revision int64
	}{version, revision}
	return catalogRequest(ctx, s.db, s.actors, actor, "model.version", version.ModelID, request, input, false, func(tx *gorm.DB) (domain.ModelVersion, error) {
		return s.withDB(tx).publishModelVersionWithAuditOnce(ctx, actor, version, revision, event)
	})
}

// PublishPriceRuleWithAudit persists prices without changing an active release.
func (s *Store) PublishPriceRuleWithAudit(ctx context.Context, actor identityapp.Principal, price domain.PriceRuleVersion, revision int64, event identityapp.OutboxEvent) (domain.PriceRuleVersion, error) {
	request := catalogEventRequest(event)
	if s == nil || s.db == nil || request == uuid.Nil || !validPriceRulePublishedAudit(actor.ID, actor.OrgID, price, revision, event) {
		return domain.PriceRuleVersion{}, application.ErrInvalidPublishPriceRule
	}

	input := struct {
		Price    domain.PriceRuleVersion
		Revision int64
	}{price, revision}
	saved, err := catalogRequest(ctx, s.db, s.actors, actor, "model.price", price.ModelID, request, input, false, func(tx *gorm.DB) (domain.PriceRuleVersion, error) {
		return s.withDB(tx).publishPriceRuleWithAuditOnce(ctx, actor, price, revision, event)
	})
	if err == nil {
		saved.Rule = append([]byte(nil), price.Rule...)
	}
	return saved, err
}

// DisableCredentialWithEvents makes exact revocation safe to retry, including draining keys.
func (s *Store) DisableCredentialWithEvents(ctx context.Context, actor identityapp.Principal, target application.CredentialToDisable, events []identityapp.OutboxEvent) (application.SavedCredential, error) {
	if s == nil || s.db == nil || len(events) != 2 {
		return application.SavedCredential{}, application.ErrInvalidDisableCredential
	}
	request := catalogEventRequest(events[1])
	if request == uuid.Nil {
		return application.SavedCredential{}, application.ErrInvalidDisableCredential
	}
	return catalogRequest(ctx, s.db, s.actors, actor, "credential.disable", target.ID, request, target, false, func(tx *gorm.DB) (application.SavedCredential, error) {
		return s.withDB(tx).disableCredentialWithEventsOnce(ctx, actor, target, events)
	})
}
