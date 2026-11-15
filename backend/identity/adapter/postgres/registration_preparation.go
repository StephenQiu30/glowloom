package postgres

import (
	"context"
	"crypto/hmac"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/identity/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

// RegistrationStore owns accepted registrations outside the public account table.
// The database must be an Identity connection, never an existing transaction.
type RegistrationStore struct{ *Store }

// NewRegistrationStore enables the durable mailbox reservation in every email flow.
func NewRegistrationStore(db *gorm.DB) *RegistrationStore {
	return &RegistrationStore{Store: NewStore(db)}
}

// EmailTransaction retains the original policy and proof locks while admitting
// registration preparations. No participant call can occur in this callback.
func (s *RegistrationStore) EmailTransaction(ctx context.Context, run func(application.EmailTx) error) error {
	return identityWriterTransaction(ctx, s.db, func(tx *gorm.DB) error {
		return run(&registrationAuthTx{emailAuthTx: &emailAuthTx{authTx: &authTx{db: tx}}})
	})
}

type registrationAuthTx struct{ *emailAuthTx }

// CheckCurrentSessionSources is called only while the original Auth transaction
// holds current user/session policy locks, before refresh or protocol conversion.
func (t *registrationAuthTx) CheckCurrentSessionSources(actor application.Principal) error {
	_, err := readOpenSessionAuthorities(t.db, actor)
	return err
}

func (t *registrationAuthTx) EmailReserved(email string) (bool, error) {
	return t.authTx.EmailReserved(email)
}

type registrationPreparationRow struct {
	OperationID, AccountID, OrganizationID, ScopeID uuid.UUID
	InputDigest                                     []byte
	UserSnapshot                                    string
	CreationReady, WalletReady                      bool
	AcceptedAt                                      time.Time
	ActivatedAt                                     *time.Time
}

func (r registrationPreparationRow) preparation() application.RegistrationPreparation {
	return application.RegistrationPreparation{OperationID: r.OperationID, AccountID: r.AccountID, OrganizationID: r.OrganizationID, ScopeID: r.ScopeID, InputSHA256: hex.EncodeToString(r.InputDigest), CreationReady: r.CreationReady, WalletReady: r.WalletReady, Activated: r.ActivatedAt != nil}
}
func readRegistrationPreparation(tx *gorm.DB, id uuid.UUID) (registrationPreparationRow, bool, error) {
	var row registrationPreparationRow
	result := tx.Raw(`SELECT operation_id,account_id,organization_id,scope_id,input_digest,user_snapshot::text,creation_ready,wallet_ready,accepted_at,activated_at FROM identity.registration_preparation WHERE operation_id=? FOR UPDATE`, id).Scan(&row)
	if result.Error != nil {
		return row, false, fmt.Errorf("read accepted registration: %w", result.Error)
	}
	return row, result.RowsAffected == 1, nil
}
func (t *registrationAuthTx) FindRegistrationPreparation(id uuid.UUID, proof []byte) (application.RegistrationPreparation, bool, error) {
	r, found, err := readRegistrationPreparation(t.db, id)
	if err != nil || !found {
		return application.RegistrationPreparation{}, found, err
	}
	if len(proof) != 32 || !hmac.Equal(proof, r.InputDigest) {
		return application.RegistrationPreparation{}, true, application.ErrProofInvalid
	}
	return r.preparation(), true, nil
}
func (t *registrationAuthTx) InsertRegistrationPreparation(p application.RegistrationPreparation, u domain.User, proof []byte, now time.Time) error {
	if p.OperationID == uuid.Nil || p.AccountID != u.ID || p.OrganizationID != u.OrgID || p.ScopeID == uuid.Nil || p.InputSHA256 != hex.EncodeToString(proof) || len(proof) != 32 || now.IsZero() {
		return application.ErrRegistrationReceipt
	}
	candidate, err := json.Marshal(u)
	if err != nil {
		return err
	}
	defer clear(candidate)
	if len(candidate) > 16384 {
		return application.ErrRegistrationUnavailable
	}
	if err := t.db.Exec(`INSERT INTO identity.registration_preparation(operation_id,account_id,organization_id,scope_id,email_key,input_digest,user_snapshot,accepted_at) VALUES(?,?,?,?,?,?,?::jsonb,?)`, p.OperationID, p.AccountID, p.OrganizationID, p.ScopeID, u.Email, proof, string(candidate), now).Error; err != nil {
		return fmt.Errorf("reserve registration identity: %w", err)
	}
	return taskruntime.Enqueue(t.db.Statement.Context, t.db, application.RegistrationRecoveryTask, p.OperationID.String(), now)
}

// LoadRegistrationPreparation accepts only the original HMAC input proof. This
// lookup never authorizes a session or exposes a pending account to a directory.
func (s *RegistrationStore) LoadRegistrationPreparation(ctx context.Context, id uuid.UUID, proof []byte) (application.RegistrationPreparation, error) {
	var result application.RegistrationPreparation
	err := identityWriterTransaction(ctx, s.db, func(tx *gorm.DB) error {
		r, found, err := readRegistrationPreparation(tx, id)
		if err != nil {
			return err
		}
		if !found || len(proof) != 32 || !hmac.Equal(proof, r.InputDigest) {
			return application.ErrProofInvalid
		}
		result = r.preparation()
		return nil
	})
	return result, err
}

// LoadAcceptedRegistration is an owner-only worker capability. It returns the
// already accepted command and no credential, and cannot admit a new registration.
func (s *RegistrationStore) LoadAcceptedRegistration(ctx context.Context, id uuid.UUID) (application.RegistrationPreparation, error) {
	var result application.RegistrationPreparation
	err := identityWriterTransaction(ctx, s.db, func(tx *gorm.DB) error {
		r, found, err := readRegistrationPreparation(tx, id)
		if err != nil {
			return err
		}
		if !found {
			return application.ErrProofInvalid
		}
		result = r.preparation()
		return nil
	})
	return result, err
}

// RecordRegistrationReady saves only an exact authenticated participant receipt.
func (s *RegistrationStore) RecordRegistrationReady(ctx context.Context, p application.RegistrationPreparation, receipt application.RegistrationReady) error {
	if (receipt.Owner != "creation" && receipt.Owner != "execution") || receipt != p.ExpectedReady(receipt.Owner) {
		return application.ErrRegistrationReceipt
	}
	return identityWriterTransaction(ctx, s.db, func(tx *gorm.DB) error {
		r, found, err := readRegistrationPreparation(tx, p.OperationID)
		if err != nil {
			return err
		}
		if !found || r.preparation().ExpectedReady(receipt.Owner) != receipt {
			return application.ErrRegistrationReceipt
		}
		if receipt.Owner == "creation" {
			return tx.Exec(`UPDATE identity.registration_preparation SET creation_ready=true WHERE operation_id=?`, p.OperationID).Error
		}
		return tx.Exec(`UPDATE identity.registration_preparation SET wallet_ready=true WHERE operation_id=?`, p.OperationID).Error
	})
}

// PrepareRegistrationAccount creates only this original registration's initial
// account head. A later closed or differently coordinated head cannot be reopened.
func (s *RegistrationStore) PrepareRegistrationAccount(ctx context.Context, p application.RegistrationPreparation) (writefence.Control, error) {
	var control writefence.Control
	err := identityWriterTransaction(ctx, s.db, func(tx *gorm.DB) error {
		t := &authTx{db: tx}
		if _, err := t.RegistrationEnabled(); err != nil {
			return err
		}
		r, found, err := readRegistrationPreparation(tx, p.OperationID)
		if err != nil {
			return err
		}
		if !found || r.preparation().ExpectedReady("creation") != p.ExpectedReady("creation") || !r.CreationReady || !r.WalletReady || r.ActivatedAt != nil {
			return application.ErrRegistrationReceipt
		}
		expected := p.AccountControl()
		authority := expected.Authority
		authority.Epoch = 0
		state, err := pgfence.NewAuthorityStore(tx).BeginTx(ctx, tx, authority, expected.CoordinationID, false)
		if err != nil {
			return err
		}
		if state.Control != expected {
			return application.ErrRegistrationReceipt
		}
		control = state.Control
		return nil
	})
	return control, err
}

// ActivateRegistration publishes the account and audit exactly once after both
// owner receipts. It does not create a session or touch participant tables.
func (s *RegistrationStore) ActivateRegistration(ctx context.Context, p application.RegistrationPreparation) (uuid.UUID, error) {
	var account uuid.UUID
	err := identityWriterTransaction(ctx, s.db, func(tx *gorm.DB) error {
		t := &emailAuthTx{authTx: &authTx{db: tx}}
		if _, err := t.RegistrationEnabled(); err != nil {
			return err
		}
		r, found, err := readRegistrationPreparation(tx, p.OperationID)
		if err != nil {
			return err
		}
		if !found || r.preparation().ExpectedReady("creation") != p.ExpectedReady("creation") || !r.CreationReady || !r.WalletReady {
			return application.ErrRegistrationReceipt
		}
		account = r.AccountID
		if r.ActivatedAt != nil {
			return nil
		}
		control := p.AccountControl()
		state, err := pgfence.NewAuthorityStore(tx).LoadCoordination(ctx, control.CoordinationID)
		if err != nil {
			return err
		}
		if !state.Complete || state.Control != control || len(state.Acknowledged) != len(writefence.Writers()) {
			return application.ErrRegistrationReceipt
		}
		var head struct {
			Epoch             int64
			CoordinationID    uuid.UUID
			Blocked, Complete bool
		}
		if err := tx.Raw(`SELECT epoch,coordination_id,blocked,complete FROM infra.authority_head WHERE service_owner=infra.runtime_owner() AND issuer='identity' AND authority_kind='account' AND authority_id=? FOR UPDATE`, p.AccountID).Scan(&head).Error; err != nil {
			return err
		}
		if head.Epoch != control.Epoch || head.CoordinationID != control.CoordinationID || head.Blocked || !head.Complete {
			return application.ErrRegistrationReceipt
		}
		var u domain.User
		if err := json.Unmarshal([]byte(r.UserSnapshot), &u); err != nil {
			return application.ErrRegistrationReceipt
		}
		if u.ID != r.AccountID || u.OrgID != r.OrganizationID || u.Status != domain.StatusActive || u.Email == "" || u.PasswordHash == "" || u.PlatformRole != "creator" || u.SessionEpoch != 1 {
			return application.ErrRegistrationReceipt
		}
		if err := t.insertVerifiedAccount(u); err != nil {
			return err
		}
		now := time.Now().UTC().Truncate(time.Microsecond)
		if now.Before(r.AcceptedAt) {
			now = r.AcceptedAt
		}
		if err := t.AppendAuthEvent(u.ID, "registered", now); err != nil {
			return err
		}
		return tx.Exec(`UPDATE identity.registration_preparation SET activated_at=? WHERE operation_id=? AND activated_at IS NULL`, now, p.OperationID).Error
	})
	if err != nil {
		return uuid.Nil, err
	}
	return account, nil
}

var _ application.RegistrationPreparationTx = (*registrationAuthTx)(nil)
var _ application.RegistrationPreparationStore = (*RegistrationStore)(nil)
