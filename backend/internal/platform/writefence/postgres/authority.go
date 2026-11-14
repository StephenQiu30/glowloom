package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// AuthorityStore is the issuer-local coordination and admission repository. It
// never checks actor/member policy. Callers must perform those checks and Register
// in the same existing local business transaction before sending any command.
type AuthorityStore struct{ db *gorm.DB }

// NewAuthorityStore uses a restricted Identity or Creation connection unchanged.
func NewAuthorityStore(db *gorm.DB) *AuthorityStore { return &AuthorityStore{db: db} }

var _ writefence.AuthorityRepository = (*AuthorityStore)(nil)

type authorityHead struct {
	Issuer         writefence.Issuer
	AuthorityKind  writefence.Kind
	AuthorityID    uuid.UUID
	Epoch          int64
	CoordinationID uuid.UUID
	Blocked        bool
	Complete       bool
}

type coordinationRow struct {
	Issuer         writefence.Issuer
	AuthorityKind  writefence.Kind
	AuthorityID    uuid.UUID
	Epoch          int64
	ExpectedEpoch  int64
	CoordinationID uuid.UUID
	Blocked        bool
}

func (r coordinationRow) control() writefence.Control {
	return writefence.Control{Authority: writefence.Authority{Issuer: r.Issuer, Kind: r.AuthorityKind, ID: r.AuthorityID, Epoch: r.Epoch}, CoordinationID: r.CoordinationID, Blocked: r.Blocked}
}

// Begin commits source closure before returning to an RPC coordinator.
func (s *AuthorityStore) Begin(ctx context.Context, authority writefence.Authority, id uuid.UUID, blocked bool) (writefence.Coordination, error) {
	if s == nil || s.db == nil || s.db.Statement == nil {
		return writefence.Coordination{}, writefence.ErrUnavailable
	}
	if _, open := s.db.Statement.ConnPool.(gorm.TxCommitter); open {
		return writefence.Coordination{}, writefence.ErrTransaction
	}
	var state writefence.Coordination
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		state, err = s.BeginTx(ctx, tx, authority, id, blocked)
		return err
	})
	return state, err
}

// BeginTx closes admission in the caller's existing policy transaction. It never
// calls a remote writer. The caller must roll back on error and commit before
// propagating this control. The expected epoch is a CAS; an exact historical
// replay retains its original ID.
func (s *AuthorityStore) BeginTx(ctx context.Context, tx *gorm.DB, authority writefence.Authority, id uuid.UUID, blocked bool) (writefence.Coordination, error) {
	if authority.Epoch < 0 || authority.Epoch == math.MaxInt64 || id == uuid.Nil {
		return writefence.Coordination{}, writefence.ErrInvalid
	}
	validated := authority
	validated.Epoch = 1
	if err := validated.Validate(); err != nil {
		return writefence.Coordination{}, err
	}
	if s == nil || s.db == nil {
		return writefence.Coordination{}, writefence.ErrUnavailable
	}
	if tx == nil || tx.Statement == nil {
		return writefence.Coordination{}, writefence.ErrTransaction
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return writefence.Coordination{}, writefence.ErrTransaction
	}
	var state writefence.Coordination
	err := func() error {
		owner, err := authorityOwner(ctx, tx)
		if err != nil {
			return err
		}
		if owner != authority.Issuer {
			return writefence.ErrInvalid
		}
		if saved, err := loadCoordination(ctx, tx, id); err == nil {
			state = saved
			return matchCoordination(saved, authority, id, blocked)
		} else if !errors.Is(err, writefence.ErrNotFound) {
			return err
		}
		inserted := tx.Exec(`INSERT INTO infra.authority_head
			(issuer,authority_kind,authority_id,epoch,coordination_id,blocked,complete)
			VALUES (?,?,?,1,?,true,false)
			ON CONFLICT (service_owner,issuer,authority_kind,authority_id) DO NOTHING`, authority.Issuer, authority.Kind, authority.ID, id)
		if inserted.Error != nil {
			return fmt.Errorf("initialize authority head: %w", inserted.Error)
		}
		head, err := lockAuthorityHead(ctx, tx, authority)
		if err != nil {
			return err
		}
		// Another Begin may have committed this original ID while we waited.
		if saved, err := loadCoordination(ctx, tx, id); err == nil {
			state = saved
			return matchCoordination(saved, authority, id, blocked)
		} else if !errors.Is(err, writefence.ErrNotFound) {
			return err
		}
		if inserted.RowsAffected == 1 {
			if authority.Epoch != 0 {
				return writefence.ErrStale
			}
		} else {
			if !head.Complete {
				return writefence.ErrConflict
			}
			if head.Epoch != authority.Epoch {
				return writefence.ErrStale
			}
		}
		epoch := authority.Epoch + 1
		result := tx.Exec(`INSERT INTO infra.authority_coordination
			(coordination_id,issuer,authority_kind,authority_id,expected_epoch,epoch,blocked)
			VALUES (?,?,?,?,?,?,?) ON CONFLICT (service_owner,coordination_id) DO NOTHING`, id, authority.Issuer, authority.Kind, authority.ID, authority.Epoch, epoch, blocked)
		if result.Error != nil {
			return fmt.Errorf("persist authority coordination: %w", result.Error)
		}
		state, err = loadCoordination(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := matchCoordination(state, authority, id, blocked); err != nil {
			return err
		}
		return tx.Exec(`UPDATE infra.authority_head SET epoch=?,coordination_id=?,blocked=true,complete=false,updated_at=statement_timestamp()
			WHERE service_owner=infra.runtime_owner() AND issuer=? AND authority_kind=? AND authority_id=?`, epoch, id, authority.Issuer, authority.Kind, authority.ID).Error
	}()
	return state, err
}

// Acknowledge accepts only the current exact control. ACKs are append-only, and
// completion/opening of the head shares the transaction with the fourth ACK.
func (s *AuthorityStore) Acknowledge(ctx context.Context, control writefence.Control, writer writefence.Writer) (writefence.Coordination, error) {
	if err := control.Validate(); err != nil {
		return writefence.Coordination{}, err
	}
	if err := writer.Validate(); err != nil {
		return writefence.Coordination{}, err
	}
	if s == nil || s.db == nil {
		return writefence.Coordination{}, writefence.ErrUnavailable
	}
	var state writefence.Coordination
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		owner, err := authorityOwner(ctx, tx)
		if err != nil {
			return err
		}
		if owner != control.Issuer {
			return writefence.ErrInvalid
		}
		head, err := lockAuthorityHead(ctx, tx, control.Authority)
		if err != nil {
			return err
		}
		state, err = loadCoordination(ctx, tx, control.CoordinationID)
		if err != nil {
			return err
		}
		if state.Control != control {
			return writefence.ErrConflict
		}
		if head.Epoch != control.Epoch || head.CoordinationID != control.CoordinationID {
			return writefence.ErrStale
		}
		if err := tx.Exec(`INSERT INTO infra.authority_acknowledgement(coordination_id,writer) VALUES (?,?)
			ON CONFLICT (service_owner,coordination_id,writer) DO NOTHING`, control.CoordinationID, writer).Error; err != nil {
			return fmt.Errorf("persist authority writer ACK: %w", err)
		}
		state, err = loadCoordination(ctx, tx, control.CoordinationID)
		if err != nil {
			return err
		}
		if state.Complete {
			return tx.Exec(`UPDATE infra.authority_head SET complete=true,blocked=?,updated_at=statement_timestamp()
				WHERE service_owner=infra.runtime_owner() AND issuer=? AND authority_kind=? AND authority_id=? AND epoch=? AND coordination_id=?`,
				control.Blocked, control.Issuer, control.Kind, control.ID, control.Epoch, control.CoordinationID).Error
		}
		return nil
	})
	return state, err
}

// LoadCoordination reads original evidence only and cannot grant fresh access.
func (s *AuthorityStore) LoadCoordination(ctx context.Context, id uuid.UUID) (writefence.Coordination, error) {
	if id == uuid.Nil {
		return writefence.Coordination{}, writefence.ErrInvalid
	}
	if s == nil || s.db == nil {
		return writefence.Coordination{}, writefence.ErrUnavailable
	}
	if _, err := authorityOwner(ctx, s.db); err != nil {
		return writefence.Coordination{}, err
	}
	return loadCoordination(ctx, s.db, id)
}

// Register records admission in the caller's already-open policy transaction.
// Every authority owned by this issuer is locked in canonical order through commit;
// other issuers remain part of the immutable full-command binding. Register does
// not authorize policy, call RPC, expire a ledger, or turn unknown into uncommitted.
func (s *AuthorityStore) Register(ctx context.Context, tx *gorm.DB, command writefence.Command, target writefence.Writer) error {
	binding, err := command.AuthorityDigest()
	if err != nil {
		return err
	}
	if err := target.Validate(); err != nil {
		return err
	}
	if tx == nil || tx.Statement == nil {
		return writefence.ErrTransaction
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return writefence.ErrTransaction
	}
	if s == nil || s.db == nil {
		return writefence.ErrUnavailable
	}
	owner, err := authorityOwner(ctx, tx)
	if err != nil {
		return err
	}
	ordered := append([]writefence.Authority(nil), command.Authorities...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Key() < ordered[j].Key() })
	owned := false
	for _, authority := range ordered {
		if authority.Issuer != owner {
			continue
		}
		owned = true
		head, err := lockAuthorityHead(ctx, tx, authority)
		if err != nil {
			return err
		}
		if !head.Complete || head.Blocked || head.Epoch != authority.Epoch {
			return writefence.ErrFenced
		}
	}
	if !owned {
		return writefence.ErrInvalid
	}
	authorities, err := json.Marshal(command.Authorities)
	if err != nil {
		return writefence.ErrInvalid
	}
	if err := tx.WithContext(ctx).Exec(`INSERT INTO infra.authority_admission
		(id,request_sha256,action,authority_set_sha256,authorities,target_writer) VALUES (?,?,?,?,?::jsonb,?)
		ON CONFLICT (service_owner,id) DO NOTHING`, command.ID, command.SHA256, command.Action, binding, string(authorities), target).Error; err != nil {
		return fmt.Errorf("register authority admission: %w", err)
	}
	var row struct {
		RequestSHA256      string `gorm:"column:request_sha256"`
		Action             string
		AuthoritySetSHA256 string `gorm:"column:authority_set_sha256"`
		TargetWriter       writefence.Writer
	}
	if err := tx.WithContext(ctx).Table("infra.authority_admission").Where(ownerPredicate).Where("id = ?", command.ID).Take(&row).Error; err != nil {
		return err
	}
	if row.TargetWriter != target {
		return writefence.ErrConflict
	}
	return sameCommand(commandRow{RequestSHA256: row.RequestSHA256, Action: row.Action, AuthoritySetSHA256: row.AuthoritySetSHA256}, command, binding)
}

func authorityOwner(ctx context.Context, db *gorm.DB) (writefence.Issuer, error) {
	var owner writefence.Issuer
	if err := db.WithContext(ctx).Raw("SELECT infra.runtime_owner()").Scan(&owner).Error; err != nil {
		return "", fmt.Errorf("resolve authority issuer: %w", err)
	}
	if owner != writefence.Identity && owner != writefence.Creation {
		return "", writefence.ErrInvalid
	}
	return owner, nil
}

func lockAuthorityHead(ctx context.Context, tx *gorm.DB, authority writefence.Authority) (authorityHead, error) {
	var head authorityHead
	err := tx.WithContext(ctx).Table("infra.authority_head").Clauses(clause.Locking{Strength: "UPDATE"}).Where(ownerPredicate).
		Where("issuer=? AND authority_kind=? AND authority_id=?", authority.Issuer, authority.Kind, authority.ID).Take(&head).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return head, writefence.ErrFenced
	}
	if err != nil {
		return head, fmt.Errorf("lock authority head: %w", err)
	}
	return head, nil
}

func matchCoordination(state writefence.Coordination, expected writefence.Authority, id uuid.UUID, blocked bool) error {
	if state.Control.Key() != expected.Key() || state.Control.Epoch != expected.Epoch+1 || state.Control.CoordinationID != id || state.Control.Blocked != blocked {
		return writefence.ErrConflict
	}
	return nil
}

func loadCoordination(ctx context.Context, db *gorm.DB, id uuid.UUID) (writefence.Coordination, error) {
	var row coordinationRow
	err := db.WithContext(ctx).Table("infra.authority_coordination").Where(ownerPredicate).Where("coordination_id=?", id).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return writefence.Coordination{}, writefence.ErrNotFound
	}
	if err != nil {
		return writefence.Coordination{}, fmt.Errorf("load authority coordination: %w", err)
	}
	if err := row.control().Validate(); err != nil || row.ExpectedEpoch < 0 || row.Epoch-1 != row.ExpectedEpoch {
		return writefence.Coordination{}, writefence.ErrConflict
	}
	var acks []struct{ Writer writefence.Writer }
	if err := db.WithContext(ctx).Table("infra.authority_acknowledgement").Select("writer").Where(ownerPredicate).Where("coordination_id=?", id).Find(&acks).Error; err != nil {
		return writefence.Coordination{}, fmt.Errorf("load authority writer ACKs: %w", err)
	}
	seen := make(map[writefence.Writer]bool, len(acks))
	for _, ack := range acks {
		if ack.Writer.Validate() != nil || seen[ack.Writer] {
			return writefence.Coordination{}, writefence.ErrConflict
		}
		seen[ack.Writer] = true
	}
	state := writefence.Coordination{Control: row.control(), Complete: len(seen) == len(writefence.Writers())}
	for _, writer := range writefence.Writers() {
		if seen[writer] {
			state.Acknowledged = append(state.Acknowledged, writer)
		}
	}
	return state, nil
}
