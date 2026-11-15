package postgres

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/creation/prompt/application"
	"github.com/StephenQiu30/glowloom-server/backend/creation/prompt/domain"
)

// FreezeConversationInput requires ready=true, unlike saveable clipboard fragments.
func (s *DraftStore) FreezeConversationInput(ctx context.Context, a application.Actor, p uuid.UUID, d domain.Draft) (domain.ConversationInput, error) {
	input, err := domain.NewConversationInput(d)
	if err != nil {
		return input, err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.authorize(ctx, tx, a, p, true); err != nil {
			return err
		}
		return ValidateDraftSources(tx, a, p, input.Draft, true, s.sources(tx))
	})
	return input, err
}

// CheckConversationSources checks current read authority for all referenced draft sources.
func (s *DraftStore) CheckConversationSources(ctx context.Context, a application.Actor, p uuid.UUID, d domain.Draft) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.authorize(ctx, tx, a, p, false); err != nil {
			return err
		}
		return ValidateDraftSources(tx, a, p, d, true, s.sources(tx))
	})
}
