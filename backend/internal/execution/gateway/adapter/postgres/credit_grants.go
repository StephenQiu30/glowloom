package postgres

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/domain"
)

const creditGrantSelect = `SELECT o.id,o.scope_id,o.source_key AS key,o.credits,o.reason,o.proposer_id,
 o.recipient_kind,o.recipient_label,o.created_at,r.reviewer_id,r.reviewed_at,r.event_id,
 CASE WHEN r.approved THEN 'issued' WHEN r.approved=false THEN 'rejected' ELSE 'pending' END AS state
 FROM ai.credit_grant_order o LEFT JOIN ai.credit_grant_review r ON r.order_id=o.id`

func listCreditGrants(tx *gorm.DB) ([]domain.CreditGrantOrder, error) {
	rows := make([]domain.CreditGrantOrder, 0)
	err := tx.Raw(creditGrantSelect + ` ORDER BY (r.order_id IS NULL) DESC,o.created_at DESC,o.id DESC LIMIT 100`).Scan(&rows).Error
	return rows, err
}

func readCreditGrant(tx *gorm.DB, clause string, key any) (domain.CreditGrantOrder, error) {
	var order domain.CreditGrantOrder
	result := tx.Raw(creditGrantSelect+clause, key).Scan(&order)
	if result.Error != nil {
		return order, result.Error
	}
	if result.RowsAffected != 1 {
		return order, domain.ErrInvalid
	}
	return order, nil
}

// SubmitCreditGrant records a globally unique immutable proposal, without moving credits.
func (s *Store) SubmitCreditGrant(ctx context.Context, in domain.CreditGrantInput) (domain.CreditGrantOrder, error) {
	var order domain.CreditGrantOrder
	in, err := in.Normalize()
	if err != nil {
		return order, err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.check(ctx, tx, in.ScopeID, uuid.Nil, true); err != nil {
			return err
		}
		// Serialize submissions even when the same key targets a different wallet.
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "credit-grant/"+in.Key).Error; err != nil {
			return err
		}
		var existing struct{ ID uuid.UUID }
		r := tx.Raw("SELECT id FROM ai.credit_grant_order WHERE source_key=?", in.Key).Scan(&existing)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 0 {
			order, err = readCreditGrant(tx, " WHERE o.id=?", existing.ID)
			if err != nil {
				return err
			}
			if order.CreditGrantInput != in || order.ProposerID != s.actor.ID {
				return domain.ErrConflict
			}
			return nil
		}
		kind, label, err := creditRecipient(tx, in.ScopeID)
		if err != nil {
			return err
		}
		id := uuid.New()
		if err := tx.Exec(`INSERT INTO ai.credit_grant_order(id,source_key,scope_id,proposer_id,credits,reason,recipient_kind,recipient_label)
		 VALUES(?,?,?,?,?,?,?,?)`, id, in.Key, in.ScopeID, s.actor.ID, in.Credits, in.Reason, kind, label).Error; err != nil {
			return err
		}
		order, err = readCreditGrant(tx, " WHERE o.id=?", id)
		return err
	})
	return order, err
}

// ReviewCreditGrant records a decision once. Approval posts the grant in the same transaction.
func (s *Store) ReviewCreditGrant(ctx context.Context, id uuid.UUID, approve bool) (domain.CreditGrantOrder, error) {
	var out domain.CreditGrantOrder
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.check(ctx, tx, s.actor.ScopeID, uuid.Nil, true); err != nil {
			return err
		}
		// Decisions use their own row in order to keep the proposed terms immutable.
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "credit-review/"+id.String()).Error; err != nil {
			return err
		}
		order, err := readCreditGrant(tx, " WHERE o.id=?", id)
		if err != nil {
			return err
		}
		if order.ProposerID == s.actor.ID {
			return domain.ErrForbidden
		}
		if order.State != "pending" {
			if (order.State == "issued") != approve {
				return domain.ErrConflict
			}
			out = order
			return nil
		}
		var eventID *uuid.UUID
		if approve {
			if _, _, err := creditRecipient(tx, order.ScopeID); err != nil {
				return err
			}
			payload, err := json.Marshal(order.CreditGrantInput)
			if err != nil {
				return err
			}
			event, err := s.grantCredits(tx, order.ScopeID, order.ID.String(), "credit-order/"+order.ID.String(), domain.Digest(payload), order.Credits)
			if err != nil {
				return err
			}
			eventID = &event
		}
		if err := tx.Exec(`INSERT INTO ai.credit_grant_review(order_id,proposer_id,reviewer_id,approved,event_id)
		 VALUES(?,?,?,?,?)`, order.ID, order.ProposerID, s.actor.ID, approve, eventID).Error; err != nil {
			return err
		}
		out, err = readCreditGrant(tx, " WHERE o.id=?", id)
		return err
	})
	return out, err
}

func creditRecipient(tx *gorm.DB, scope uuid.UUID) (string, string, error) {
	var recipient struct {
		Kind  string
		Owner *uuid.UUID
		Team  *uuid.UUID
	}
	row := tx.Raw("SELECT kind,owner_user_id AS owner,team_id AS team FROM workspace.creation_scope WHERE id=? FOR SHARE", scope).Scan(&recipient)
	if row.Error != nil {
		return "", "", row.Error
	}
	if row.RowsAffected != 1 {
		return "", "", domain.ErrInvalid
	}
	var named struct{ Label string }
	if recipient.Kind == "personal" {
		row = tx.Raw(`SELECT display_name AS label FROM identity."user"
		 WHERE id=? AND status='active' AND NOT is_delete FOR SHARE`, recipient.Owner).Scan(&named)
	} else {
		row = tx.Raw(`SELECT name AS label FROM workspace.team
		 WHERE id=? AND status='active' AND NOT governance_blocked FOR SHARE`, recipient.Team).Scan(&named)
	}
	if row.Error != nil {
		return "", "", row.Error
	}
	if row.RowsAffected != 1 {
		return "", "", domain.ErrInvalid
	}
	return recipient.Kind, named.Label, nil
}
