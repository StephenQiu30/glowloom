package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// NewTeamActorStore supplies live identity authorization and transactional encrypted notices.
func NewTeamActorStore(db *gorm.DB, protector application.ProofProtector) *Store {
	return &Store{db: db, noticeProtector: protector}
}

// NotifyTeamAccounts queues a generic notification for distinct still-retained recipients.
func (s *Store) NotifyTeamAccounts(ctx context.Context, ids []uuid.UUID, now time.Time) error {
	if s.noticeProtector == nil {
		return application.ErrRegistrationUnavailable
	}
	if _, ok := s.db.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return application.ErrGovernanceConflict
	}
	var rows []struct{ Email string }
	if err := s.db.WithContext(ctx).Raw(`SELECT DISTINCT email_key AS email FROM identity."user" WHERE id IN ? AND NOT is_delete AND email_verified_at IS NOT NULL AND status IN ('active','disabled')`, ids).Scan(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		event := uuid.New()
		payload, err := json.Marshal(map[string]string{})
		if err != nil {
			return err
		}
		encrypted, err := s.noticeProtector.Seal(event, payload)
		if err != nil {
			return err
		}
		if err := (&authTx{db: s.db.WithContext(ctx)}).EnqueueMail(application.QueuedMail{EventID: event, Template: "team_governance", Recipient: row.Email, KeyID: s.noticeProtector.KeyID(), EncryptedPayload: encrypted, CreatedAt: now, ExpiresAt: now.Add(48 * time.Hour)}); err != nil {
			return err
		}
	}
	return nil
}
