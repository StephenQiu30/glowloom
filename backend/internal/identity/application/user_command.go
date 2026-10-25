package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

// UserCommandReceipt proves an exact account mutation committed with its audit and mail.
type UserCommandReceipt struct {
	RequestID   uuid.UUID `json:"request_id"`
	TargetID    uuid.UUID `json:"target_id"`
	Action      string    `json:"action"`
	RequestHash string    `json:"-"`
}

func userCommandHash(target uuid.UUID, revision int64, reason, action string) string {
	body, _ := json.Marshal([]any{target, revision, reason, action})
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

// ExecuteUserCommand preserves individual authorization and revision boundaries during bulk work.
func (s *Governance) ExecuteUserCommand(ctx context.Context, actor Principal, target uuid.UUID, revision int64, reason, action string, key uuid.UUID) error {
	if key == uuid.Nil || target == actor.ID {
		return ErrInvalidGovernance
	}
	switch action {
	case "disable", "enable", "revoke_sessions":
	default:
		return ErrInvalidGovernance
	}
	return s.mutateUser(ctx, actor, target, revision, reason, UserGovernanceChange{Action: action, RequestID: key})
}

// UserCommandReceipt reads only the actor's original command under current platform authority.
func (s *Governance) UserCommandReceipt(ctx context.Context, actor Principal, target, request uuid.UUID) (UserCommandReceipt, error) {
	var out UserCommandReceipt
	if target == uuid.Nil || request == uuid.Nil {
		return out, ErrInvalidGovernance
	}
	err := s.store.GovernanceTransaction(ctx, actor, false, s.now, func(tx GovernanceTx) error {
		if err := tx.RequirePlatform(); err != nil {
			return err
		}
		r, found, err := tx.ReadUserCommand(request)
		if err != nil {
			return err
		}
		if !found || r.TargetID != target {
			return domain.ErrUserNotFound
		}
		out = r
		return nil
	})
	return out, err
}
