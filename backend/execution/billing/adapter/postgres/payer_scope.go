package postgres

import (
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/execution/billing/application"
	"github.com/StephenQiu30/glowloom-server/backend/execution/billing/domain"
)

// walletPayerForReserve consumes metadata read by Operation from the exact accepted
// Creation input in this funding transaction. It does not authorize a reservation;
// Reserve's existing project authorization and the caller's Guard remain mandatory.
func walletPayerForReserve(tx *gorm.DB, input application.ReserveInput) (uuid.UUID, error) {
	var owner string
	if err := tx.Raw(`SELECT infra.runtime_owner()`).Scan(&owner).Error; err != nil {
		return uuid.Nil, err
	}
	if owner != "execution" {
		return walletPayer(tx, input.ProjectID)
	}
	if input.SourceScopeID == uuid.Nil || input.SourceScopeID != input.Actor.ScopeID {
		return uuid.Nil, domain.ErrWalletPayerChanged
	}
	return localWalletPayer(tx, input.ProjectID, input.SourceScopeID)
}

// localWalletPayer preserves the old mapping-first/account-fallback behavior using
// only Billing-owned tables. The current project scope comes from Creation's
// accepted source receipt, never a Workspace SQL join or request header.
func localWalletPayer(tx *gorm.DB, project, sourceScope uuid.UUID) (uuid.UUID, error) {
	if sourceScope == uuid.Nil {
		return uuid.Nil, domain.ErrWalletPayerChanged
	}
	var row struct {
		MappedScope  *uuid.UUID
		WalletExists bool
	}
	read := tx.Raw(`SELECT w.scope_id AS mapped_scope,EXISTS(SELECT 1 FROM billing.wallet_account WHERE scope_id=?) AS wallet_exists FROM (SELECT 1) one LEFT JOIN billing.project_wallet w ON w.project_id=?`, sourceScope, project).Scan(&row)
	if read.Error != nil {
		return uuid.Nil, read.Error
	}
	if row.MappedScope != nil && *row.MappedScope != sourceScope {
		return uuid.Nil, domain.ErrWalletPayerChanged
	}
	if !row.WalletExists {
		return uuid.Nil, nil
	}
	return sourceScope, nil
}
