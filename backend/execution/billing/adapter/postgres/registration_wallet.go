package postgres

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/execution/billing/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

// RegistrationWalletStore is the Execution-local registration participant.
// It creates the existing wallet-account fact without inventing any credit lots.
type RegistrationWalletStore struct{ db *gorm.DB }

// NewRegistrationWalletStore injects only the restricted Execution connection.
func NewRegistrationWalletStore(db *gorm.DB) *RegistrationWalletStore {
	return &RegistrationWalletStore{db: db}
}
func (s *RegistrationWalletStore) transaction(ctx context.Context, run func(*gorm.DB) error) error {
	if s == nil || s.db == nil || s.db.Statement == nil {
		return application.ErrRegistrationWallet
	}
	if _, open := s.db.Statement.ConnPool.(gorm.TxCommitter); open {
		return writefence.ErrTransaction
	}
	return taskruntime.Transaction(ctx, s.db, func(tx *gorm.DB) error {
		owner, err := pgfence.New(tx).Owner(ctx)
		if err != nil {
			return err
		}
		if owner != writefence.WriterExecution {
			return application.ErrRegistrationWallet
		}
		return run(tx)
	})
}
func readRegistrationWallet(tx *gorm.DB, p application.RegistrationWallet) (application.RegistrationWallet, bool, error) {
	var r application.RegistrationWallet
	result := tx.Raw(`SELECT operation_id,account_id,organization_id,scope_id,input_sha256,true AS creation_ready FROM billing.registration_wallet_preparation WHERE operation_id=? AND creation_receipt_sha256=?`, p.OperationID, p.CreationReceiptSHA256()).Scan(&r)
	if result.Error != nil {
		return r, false, result.Error
	}
	if result.RowsAffected == 1 && r != p {
		return r, true, application.ErrRegistrationWallet
	}
	return r, result.RowsAffected == 1, nil
}

// PrepareRegistrationWallet commits the wallet and immutable ready receipt together.
func (s *RegistrationWalletStore) PrepareRegistrationWallet(ctx context.Context, p application.RegistrationWallet) (application.RegistrationWallet, error) {
	if err := p.Validate(); err != nil {
		return application.RegistrationWallet{}, err
	}
	var receipt application.RegistrationWallet
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?,0))`, p.OperationID.String()).Error; err != nil {
			return err
		}
		var found bool
		var err error
		receipt, found, err = readRegistrationWallet(tx, p)
		if err != nil || found {
			return err
		}
		// A pre-existing account is not adopted by a new source command. The command
		// and account insert roll back together on any conflicting resource identity.
		if err := tx.Exec(`INSERT INTO billing.wallet_account(scope_id) VALUES(?)`, p.ScopeID).Error; err != nil {
			return fmt.Errorf("prepare registration wallet: %w", err)
		}
		if err := tx.Exec(`INSERT INTO billing.registration_wallet_preparation(operation_id,account_id,organization_id,scope_id,input_sha256,creation_receipt_sha256)VALUES(?,?,?,?,?,?)`, p.OperationID, p.AccountID, p.OrganizationID, p.ScopeID, p.InputSHA256, p.CreationReceiptSHA256()).Error; err != nil {
			return fmt.Errorf("record registration wallet: %w", err)
		}
		receipt, found, err = readRegistrationWallet(tx, p)
		if err != nil {
			return err
		}
		if !found {
			return application.ErrRegistrationWallet
		}
		return nil
	})
	if err != nil {
		return application.RegistrationWallet{}, err
	}
	return receipt, nil
}

// ResolveRegistrationWallet reads the exact committed original wallet receipt.
func (s *RegistrationWalletStore) ResolveRegistrationWallet(ctx context.Context, p application.RegistrationWallet) (application.RegistrationWallet, error) {
	if err := p.Validate(); err != nil {
		return application.RegistrationWallet{}, err
	}
	var receipt application.RegistrationWallet
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		var found bool
		var err error
		receipt, found, err = readRegistrationWallet(tx, p)
		if err != nil {
			return err
		}
		if !found {
			return application.ErrRegistrationWallet
		}
		return nil
	})
	if err != nil {
		return application.RegistrationWallet{}, err
	}
	return receipt, nil
}
