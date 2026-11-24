package aigateway

import (
	"context"
	"strconv"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	billing "github.com/StephenQiu30/glowloom-server/backend/execution/billing/application"
	pg "github.com/StephenQiu30/glowloom-server/backend/execution/gateway/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
)

// Wallet projects the one new account and event log into the existing workbench display contract.
func (r *Runtime) Wallet(ctx context.Context, a identity.Principal, cursor *uuid.UUID, limit int) (billing.WalletView, error) {
	out := billing.WalletView{Enabled: true, RestrictedCredits: "0", Entries: []billing.WalletEntry{}}
	err := r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := Authority(ctx, tx, a, a.ScopeID, uuid.Nil, false); err != nil {
			return err
		}
		var account struct {
			Payer, RedeemedToday     string
			Available, Held, Version int64
		}
		if err := tx.Raw(`SELECT s.kind AS payer, coalesce(a.available,0) AS available,
            coalesce(a.held,0) AS held, coalesce(a.version,0) AS version,
            (SELECT coalesce(sum(e.spent_delta),0)::text FROM ai.money_event e
             WHERE e.scope_id=s.id AND e.kind='settle'
               AND e.created_at>=date_trunc('day',transaction_timestamp(),'Asia/Shanghai')) AS redeemed_today
            FROM workspace.creation_scope s LEFT JOIN ai.account a ON a.scope_id=s.id
            WHERE s.id=?`, a.ScopeID).Scan(&account).Error; err != nil {
			return err
		}
		out.Payer = account.Payer
		out.RedeemedToday = account.RedeemedToday
		out.Version = account.Version
		out.GeneralCredits = strconv.FormatInt(account.Available, 10)
		out.HeldCredits = strconv.FormatInt(account.Held, 10)
		allowed, err := pg.CanReadLedger(tx, a.ScopeID, a.ID)
		if err != nil {
			return err
		}
		out.HistoryAvailable = allowed
		if !allowed {
			return nil
		}
		var rows []struct {
			ID                                    uuid.UUID
			RequestID                             *uuid.UUID
			Kind, Origin, Capability              string
			ModelKey                              *string
			AvailableDelta, HeldDelta, SpentDelta int64
			CreatedAt                             time.Time
		}
		query := `SELECT e.id,e.request_id,e.kind,e.available_delta,e.held_delta,e.spent_delta,e.created_at,r.origin,r.contract->>'capability' AS capability,r.contract->>'name' AS model_key FROM ai.money_event e LEFT JOIN ai.request r ON r.id=e.request_id WHERE e.scope_id=?`
		args := []any{a.ScopeID}
		if cursor != nil {
			query += " AND (e.created_at,e.id)<(SELECT created_at,id FROM ai.money_event WHERE id=? AND scope_id=?)"
			args = append(args, *cursor, a.ScopeID)
		}
		query += " ORDER BY e.created_at DESC,e.id DESC LIMIT ?"
		args = append(args, limit+1)
		if err := tx.Raw(query, args...).Scan(&rows).Error; err != nil {
			return err
		}
		if len(rows) > limit {
			next := rows[limit-1].ID
			out.NextCursor = &next
			rows = rows[:limit]
		}
		for _, e := range rows {
			amount := e.AvailableDelta
			if e.Kind == "settle" {
				amount = -e.SpentDelta
			}
			out.Entries = append(out.Entries, billing.WalletEntry{ID: e.ID, Kind: e.Kind, Credits: strconv.FormatInt(amount, 10), Source: e.Origin, Capability: e.Capability, ModelKey: e.ModelKey, OperationID: e.RequestID, CreatedAt: e.CreatedAt})
		}
		return nil
	})
	return out, err
}
