package postgres

import (
	"context"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/billing/application"
)

// ReadWallet runs on a connection whose caller has already locked the current
// session and creation-scope membership. The cursor is local to that scope.
func (s *Store) ReadWallet(ctx context.Context, scope uuid.UUID, history bool, cursor *uuid.UUID) (application.WalletView, error) {
	view, _, err := s.ReadWalletWithBalance(ctx, scope, history, cursor, nil)
	return view, err
}

// ReadWalletWithBalance uses a versioned cache candidate for balances while
// keeping every history page in PostgreSQL.
func (s *Store) ReadWalletWithBalance(ctx context.Context, scope uuid.UUID, history bool, cursor *uuid.UUID, cached *application.WalletBalance) (application.WalletView, application.WalletBalance, error) {
	view := application.WalletView{HistoryAvailable: history, Entries: []application.WalletEntry{}}
	tx := s.db.WithContext(ctx)
	var balance application.WalletBalance
	if cached != nil {
		balance = *cached
	} else {
		err := tx.Raw(`SELECT
		EXISTS(SELECT 1 FROM billing.wallet_account WHERE scope_id=@scope) AS enabled,
		COALESCE((SELECT version FROM billing.wallet_account WHERE scope_id=@scope),0) AS version,
		(SELECT trim_scale(COALESCE(sum(available_micros),0)*0.00001)::text FROM billing.wallet_lot
		  WHERE scope_id=@scope AND model_key IS NULL AND (expires_at IS NULL OR expires_at>transaction_timestamp())) AS general_credits,
		(SELECT trim_scale(COALESCE(sum(available_micros),0)*0.00001)::text FROM billing.wallet_lot
		  WHERE scope_id=@scope AND model_key IS NOT NULL AND (expires_at IS NULL OR expires_at>transaction_timestamp())) AS restricted_credits,
		trim_scale((
		  COALESCE((SELECT sum(r.amount_micros) FROM billing.wallet_hold h
		    JOIN billing.reservation r ON r.id=h.reservation_id AND r.status='held' AND NOT r.is_delete
		    WHERE h.scope_id=@scope),0) +
		  COALESCE((SELECT sum(amount_micros) FROM billing.credit_hold
		    WHERE scope_id=@scope AND status='held'),0)
		)*0.00001)::text AS held_credits,
		(SELECT trim_scale(COALESCE(sum(amount_micros),0)*0.00001)::text FROM billing.wallet_entry
		  WHERE scope_id=@scope AND kind='redeem'
		    AND created_at>=date_trunc('day',transaction_timestamp(),'Asia/Shanghai')) AS redeemed_today`,
			map[string]any{"scope": scope}).Scan(&balance).Error
		if err != nil {
			return view, application.WalletBalance{}, err
		}
	}
	view.Enabled = balance.Enabled
	view.Version = balance.Version
	view.GeneralCredits, view.RestrictedCredits = balance.GeneralCredits, balance.RestrictedCredits
	view.HeldCredits, view.RedeemedToday = balance.HeldCredits, balance.RedeemedToday
	if !view.Enabled || !history {
		return view, balance, nil
	}
	const pageSize = 25
	result := tx.Raw(`SELECT e.id,e.kind,trim_scale(e.amount_micros::numeric*0.00001)::text AS credits,
		l.source_kind AS source,coalesce(attribution.model_key,l.model_key) AS model_key,l.expires_at,e.created_at,r.operation_id
		FROM billing.wallet_entry e JOIN billing.wallet_lot l ON l.id=e.lot_id
		LEFT JOIN billing.reservation r ON r.id=e.reservation_id
		LEFT JOIN LATERAL (SELECT model_key FROM billing.ledger_entry
		  WHERE operation_id=r.operation_id AND project_id=r.project_id AND entry_type='reserve' AND NOT is_delete
		  ORDER BY id LIMIT 1) attribution ON true
		WHERE e.scope_id=@scope AND (CAST(@cursor AS uuid) IS NULL OR (e.created_at,e.id)<
		  (SELECT created_at,id FROM billing.wallet_entry WHERE id=CAST(@cursor AS uuid) AND scope_id=@scope))
		ORDER BY e.created_at DESC,e.id DESC LIMIT @limit`,
		map[string]any{"scope": scope, "cursor": cursor, "limit": pageSize + 1}).Scan(&view.Entries)
	if result.Error != nil {
		return view, balance, result.Error
	}
	if len(view.Entries) > pageSize {
		view.Entries = view.Entries[:pageSize]
		next := view.Entries[pageSize-1].ID
		view.NextCursor = &next
	}
	return view, balance, nil
}
