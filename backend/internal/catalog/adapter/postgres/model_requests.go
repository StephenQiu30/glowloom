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
func (s *Store) CreateModelWithAudit(ctx context.Context, actorID, orgID uuid.UUID, model domain.ModelProfile, event identityapp.OutboxEvent) (domain.ModelProfile, error) {
	request := catalogEventRequest(event)
	if s == nil || s.db == nil || request == uuid.Nil || !validModelCreatedAudit(actorID, orgID, model, event) {
		return domain.ModelProfile{}, application.ErrInvalidCreateModel
	}
	actor := identityapp.Principal{ID: actorID, OrgID: orgID}
	return catalogRequest(ctx, s.db, actor, "model.create", uuid.Nil, request, model, false, func(tx *gorm.DB) (domain.ModelProfile, error) {
		return (&Store{db: tx}).createModelWithAuditOnce(ctx, actorID, orgID, model, event)
	})
}

// PublishModelVersionWithAudit persists configuration drafts with replay protection.
func (s *Store) PublishModelVersionWithAudit(ctx context.Context, actorID, orgID uuid.UUID, version domain.ModelVersion, revision int64, event identityapp.OutboxEvent) (domain.ModelVersion, error) {
	request := catalogEventRequest(event)
	if s == nil || s.db == nil || request == uuid.Nil || !validModelVersionPublishedAudit(actorID, orgID, version, revision, event) {
		return domain.ModelVersion{}, application.ErrInvalidPublishModelVersion
	}
	actor := identityapp.Principal{ID: actorID, OrgID: orgID}
	input := struct {
		Version  domain.ModelVersion
		Revision int64
	}{version, revision}
	return catalogRequest(ctx, s.db, actor, "model.version", version.ModelID, request, input, false, func(tx *gorm.DB) (domain.ModelVersion, error) {
		return (&Store{db: tx}).publishModelVersionWithAuditOnce(ctx, actorID, orgID, version, revision, event)
	})
}

// PublishPriceRuleWithAudit persists prices without changing an active release.
func (s *Store) PublishPriceRuleWithAudit(ctx context.Context, actorID, orgID uuid.UUID, price domain.PriceRuleVersion, revision int64, event identityapp.OutboxEvent) (domain.PriceRuleVersion, error) {
	request := catalogEventRequest(event)
	if s == nil || s.db == nil || request == uuid.Nil || !validPriceRulePublishedAudit(actorID, orgID, price, revision, event) {
		return domain.PriceRuleVersion{}, application.ErrInvalidPublishPriceRule
	}
	actor := identityapp.Principal{ID: actorID, OrgID: orgID}
	input := struct {
		Price    domain.PriceRuleVersion
		Revision int64
	}{price, revision}
	saved, err := catalogRequest(ctx, s.db, actor, "model.price", price.ModelID, request, input, false, func(tx *gorm.DB) (domain.PriceRuleVersion, error) {
		return (&Store{db: tx}).publishPriceRuleWithAuditOnce(ctx, actorID, orgID, price, revision, event)
	})
	if err == nil {
		saved.Rule = append([]byte(nil), price.Rule...)
	}
	return saved, err
}

// DisableCredentialWithEvents makes exact revocation safe to retry, including draining keys.
func (s *Store) DisableCredentialWithEvents(ctx context.Context, actorID, orgID uuid.UUID, target application.CredentialToDisable, events []identityapp.OutboxEvent) (application.SavedCredential, error) {
	if s == nil || s.db == nil || len(events) != 2 {
		return application.SavedCredential{}, application.ErrInvalidDisableCredential
	}
	request := catalogEventRequest(events[1])
	if request == uuid.Nil {
		return application.SavedCredential{}, application.ErrInvalidDisableCredential
	}
	return catalogRequest(ctx, s.db, identityapp.Principal{ID: actorID, OrgID: orgID}, "credential.disable", target.ID, request, target, false, func(tx *gorm.DB) (application.SavedCredential, error) {
		return (&Store{db: tx}).disableCredentialWithEventsOnce(ctx, actorID, orgID, target, events)
	})
}
