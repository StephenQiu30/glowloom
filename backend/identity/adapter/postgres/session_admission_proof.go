package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

var _ application.SessionAdmissionReader = (*SessionAdmissionStore)(nil)

type sessionAdmissionRow struct {
	CommandID          uuid.UUID
	TargetWriter       writefence.Writer
	AccountID          uuid.UUID
	SessionID          uuid.UUID
	CredentialEpoch    int64
	RequestSHA256      string `gorm:"column:request_sha256"`
	AuthoritySetSHA256 string `gorm:"column:authority_set_sha256"`
}

// recordSessionAdmission adds immutable credential evidence to the same existing
// policy transaction as the full admission. An old command without this evidence
// is never upgraded by a read, and an idempotent write must match every field.
func recordSessionAdmission(ctx context.Context, tx *gorm.DB, actor application.Principal, command writefence.Command, target writefence.Writer) error {
	binding, err := command.AuthorityDigest()
	if err != nil {
		return err
	}
	expected := sessionAdmissionRow{CommandID: command.ID, TargetWriter: target, AccountID: actor.ID, SessionID: actor.SessionID, CredentialEpoch: actor.CredentialEpoch, RequestSHA256: command.SHA256, AuthoritySetSHA256: binding}
	if err := tx.WithContext(ctx).Exec(`INSERT INTO identity.session_command_admission
		(command_id,target_writer,account_id,session_id,credential_epoch,request_sha256,authority_set_sha256)
		VALUES (?,?,?,?,?,?,?) ON CONFLICT (command_id,target_writer) DO NOTHING`,
		expected.CommandID, expected.TargetWriter, expected.AccountID, expected.SessionID, expected.CredentialEpoch, expected.RequestSHA256, expected.AuthoritySetSHA256).Error; err != nil {
		return fmt.Errorf("record session admission identity: %w", err)
	}
	actual, err := readSessionAdmissionRow(ctx, tx, command.ID, target)
	if err != nil {
		return err
	}
	if actual != expected {
		return writefence.ErrConflict
	}
	return nil
}

// requireExistingAdmissionProof prevents a historical unbound ledger row from
// acquiring a new credential proof during a later explicit write replay.
func requireExistingAdmissionProof(ctx context.Context, tx *gorm.DB, id uuid.UUID, target writefence.Writer) error {
	var row struct{ TargetWriter writefence.Writer }
	err := tx.WithContext(ctx).Table("infra.authority_admission").Where("service_owner='identity' AND id=?", id).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("check existing session admission: %w", err)
	}
	if row.TargetWriter != target {
		return writefence.ErrConflict
	}
	_, err = readSessionAdmissionRow(ctx, tx, id, target)
	return err
}

func readSessionAdmissionRow(ctx context.Context, tx *gorm.DB, id uuid.UUID, target writefence.Writer) (sessionAdmissionRow, error) {
	var row sessionAdmissionRow
	err := tx.WithContext(ctx).Table("identity.session_command_admission").Where("command_id=? AND target_writer=?", id, target).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return row, writefence.ErrNotFound
	}
	if err != nil {
		return row, fmt.Errorf("read session admission identity: %w", err)
	}
	return row, nil
}

func sessionAdmissionBindsActor(command writefence.Command, actor application.Principal) bool {
	var account, session bool
	for _, a := range command.Authorities {
		if a.Issuer != writefence.Identity {
			continue
		}
		switch {
		case a.Kind == writefence.Account && a.ID == actor.ID && !account:
			account = true
		case a.Kind == writefence.Session && a.ID == actor.SessionID && !session:
			session = true
		default:
			return false
		}
	}
	return account && session
}

// ReadSessionAdmission returns only an existing exact admission whose original
// credential and Identity heads are still live. It uses only the restricted
// Identity pool, retains no SQL locks across RPC, and never calls Register.
func (s *SessionAdmissionStore) ReadSessionAdmission(ctx context.Context, command writefence.Command, target writefence.Writer, clock func() time.Time) (application.SessionAdmissionProof, error) {
	if s == nil || clock == nil {
		return application.SessionAdmissionProof{}, writefence.ErrUnavailable
	}
	binding, err := command.AuthorityDigest()
	if err != nil {
		return application.SessionAdmissionProof{}, err
	}
	if err := target.Validate(); err != nil {
		return application.SessionAdmissionProof{}, err
	}
	var proof application.SessionAdmissionProof
	err = identityWriterTransaction(ctx, s.db, func(tx *gorm.DB) error {
		row, err := readSessionAdmissionRow(ctx, tx, command.ID, target)
		if err != nil {
			return err
		}
		if row.RequestSHA256 != command.SHA256 || row.AuthoritySetSHA256 != binding {
			return writefence.ErrConflict
		}
		actor := application.Principal{ID: row.AccountID, SessionID: row.SessionID, CredentialEpoch: row.CredentialEpoch}
		if !sessionAdmissionBindsActor(command, actor) {
			return writefence.ErrConflict
		}
		// The actor is derived from immutable persisted proof, not the request.
		// Keep the same account -> session -> ordered head policy lock sequence.
		if _, err := lockSelfActor(tx, actor, clock); err != nil {
			return err
		}
		original, err := readOriginalAdmission(ctx, tx, command, target, binding)
		if err != nil {
			return err
		}
		ordered := append([]writefence.Authority(nil), original.Authorities...)
		sort.Slice(ordered, func(i, j int) bool { return ordered[i].Key() < ordered[j].Key() })
		for _, a := range ordered {
			if a.Issuer != writefence.Identity {
				continue
			}
			var head struct {
				Epoch    int64
				Blocked  bool
				Complete bool
			}
			result := tx.WithContext(ctx).Raw(`SELECT epoch,blocked,complete FROM infra.authority_head
				WHERE service_owner='identity' AND issuer='identity' AND authority_kind=? AND authority_id=? FOR SHARE`, a.Kind, a.ID).Scan(&head)
			if result.Error != nil {
				return fmt.Errorf("read session admission authority head: %w", result.Error)
			}
			if result.RowsAffected != 1 || head.Blocked || !head.Complete || head.Epoch != a.Epoch {
				return writefence.ErrFenced
			}
		}
		proof = application.SessionAdmissionProof{AccountID: row.AccountID, SessionID: row.SessionID, CredentialEpoch: row.CredentialEpoch, Command: original, Target: target}
		return nil
	})
	if err != nil {
		return application.SessionAdmissionProof{}, err
	}
	return proof, nil
}

func readOriginalAdmission(ctx context.Context, tx *gorm.DB, command writefence.Command, target writefence.Writer, binding string) (writefence.Command, error) {
	var row struct {
		ID                 uuid.UUID
		RequestSHA256      string `gorm:"column:request_sha256"`
		Action             string
		AuthoritySetSHA256 string `gorm:"column:authority_set_sha256"`
		Authorities        json.RawMessage
		TargetWriter       writefence.Writer
	}
	err := tx.WithContext(ctx).Table("infra.authority_admission").Where("service_owner='identity' AND id=?", command.ID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return writefence.Command{}, writefence.ErrNotFound
	}
	if err != nil {
		return writefence.Command{}, fmt.Errorf("read original session admission: %w", err)
	}
	if row.TargetWriter != target || row.RequestSHA256 != command.SHA256 || row.Action != command.Action || row.AuthoritySetSHA256 != binding {
		return writefence.Command{}, writefence.ErrConflict
	}
	original := writefence.Command{ID: row.ID, SHA256: row.RequestSHA256, Action: row.Action}
	decoder := json.NewDecoder(bytes.NewReader(row.Authorities))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&original.Authorities); err != nil {
		return writefence.Command{}, writefence.ErrConflict
	}
	actual, err := original.AuthorityDigest()
	if err != nil || actual != binding {
		return writefence.Command{}, writefence.ErrConflict
	}
	return original, nil
}
