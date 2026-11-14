// Package postgres persists writer-local barriers. It does not authorize actors
// or authenticate issuers; only an issuer-authenticated control adapter may call
// Install. Business effects must call Guard and Complete in their own transaction.
package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

const ownerPredicate = "service_owner = infra.runtime_owner()"

// Store derives its owner only from the restricted database connection.
// No request parameter, GUC or application-supplied owner changes its scope.
type Store struct{ db *gorm.DB }

// New uses the caller-owned, restricted writer connection without changing it.
func New(db *gorm.DB) *Store { return &Store{db: db} }

type fenceRow struct {
	Issuer         writefence.Issuer
	AuthorityKind  writefence.Kind
	AuthorityID    uuid.UUID
	Epoch          int64
	Blocked        bool
	CoordinationID uuid.UUID
}

type commandRow struct {
	ID                 uuid.UUID
	RequestSHA256      string `gorm:"column:request_sha256"`
	Action             string
	AuthoritySetSHA256 string          `gorm:"column:authority_set_sha256"`
	Authorities        json.RawMessage `gorm:"type:jsonb"`
	State              writefence.State
	Receipt            json.RawMessage `gorm:"type:jsonb"`
}

// Owner verifies the authenticated database capability, not a configured label.
// Private control adapters must compare this result with their fixed audience.
func (s *Store) Owner(ctx context.Context) (writefence.Writer, error) {
	if s == nil || s.db == nil {
		return "", writefence.ErrUnavailable
	}
	var owner writefence.Writer
	if err := s.db.WithContext(ctx).Raw("SELECT infra.runtime_owner()").Scan(&owner).Error; err != nil {
		return "", fmt.Errorf("verify writer owner: %w", err)
	}
	if err := owner.Validate(); err != nil {
		return "", err
	}
	return owner, nil
}

// Install serializes a monotonic issuer decision with every final business
// transaction holding the corresponding fence row. An ACK is returned only after
// commit. A repeated epoch must have the exact same coordination ID and decision.
func (s *Store) Install(ctx context.Context, control writefence.Control) error {
	if err := control.Validate(); err != nil {
		return err
	}
	if s == nil || s.db == nil {
		return writefence.ErrUnavailable
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		inserted := tx.Exec(`INSERT INTO infra.writer_fence
			(issuer, authority_kind, authority_id, epoch, blocked, coordination_id)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT (service_owner, issuer, authority_kind, authority_id) DO NOTHING`,
			control.Issuer, control.Kind, control.ID, control.Epoch, control.Blocked, control.CoordinationID)
		if inserted.Error != nil {
			return fmt.Errorf("register writer fence: %w", inserted.Error)
		}
		row, err := lockFence(ctx, tx, control.Authority)
		if err != nil {
			return err
		}
		if control.Epoch < row.Epoch {
			return writefence.ErrStale
		}
		if control.Epoch == row.Epoch {
			if control.Blocked != row.Blocked || control.CoordinationID != row.CoordinationID {
				return writefence.ErrConflict
			}
			return nil
		}
		return tx.Exec(`UPDATE infra.writer_fence
			SET epoch = ?, blocked = ?, coordination_id = ?, updated_at = statement_timestamp()
			WHERE service_owner = infra.runtime_owner() AND issuer = ? AND authority_kind = ? AND authority_id = ?`,
			control.Epoch, control.Blocked, control.CoordinationID, control.Issuer, control.Kind, control.ID).Error
	})
}

// Admit durably records a previously authorized command, including every policy
// epoch needed after restart. Missing fences fail closed; a grant never bootstraps
// an unknown local authority. Completed/frozen identities remain queryable.
func (s *Store) Admit(ctx context.Context, command writefence.Command) (writefence.Record, error) {
	binding, err := command.AuthorityDigest()
	if err != nil {
		return writefence.Record{}, err
	}
	if s == nil || s.db == nil {
		return writefence.Record{}, writefence.ErrUnavailable
	}
	var saved writefence.Record
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		open, err := lockAuthorities(ctx, tx, command.Authorities)
		if err != nil {
			return err
		}
		row, err := loadRow(ctx, tx, command.ID, true)
		if err == nil {
			if err := sameCommand(row, command, binding); err != nil {
				return err
			}
			if row.State != writefence.Admitted {
				saved, err = record(row)
				return err
			}
		} else if !errors.Is(err, writefence.ErrNotFound) {
			return err
		}
		if !open {
			return writefence.ErrFenced
		}
		if errors.Is(err, writefence.ErrNotFound) {
			authorities, err := json.Marshal(command.Authorities)
			if err != nil {
				return writefence.ErrInvalid
			}
			result := tx.Exec(`INSERT INTO infra.writer_command
				(id, request_sha256, action, authority_set_sha256, authorities, state)
				VALUES (?, ?, ?, ?, ?::jsonb, 'admitted')
				ON CONFLICT (service_owner, id) DO NOTHING`, command.ID, command.SHA256, command.Action, binding, string(authorities))
			if result.Error != nil {
				return fmt.Errorf("admit writer command: %w", result.Error)
			}
			row, err = loadRow(ctx, tx, command.ID, true)
			if err != nil {
				return err
			}
			if err := sameCommand(row, command, binding); err != nil {
				return err
			}
		}
		saved, err = record(row)
		return err
	})
	return saved, err
}

// Guard must run before the effect in its existing local transaction. Fence and
// command locks remain held until that transaction commits or rolls back. Returning
// ErrCommitted means the caller must return the saved receipt, not replay effects.
func (s *Store) Guard(ctx context.Context, tx *gorm.DB, command writefence.Command) error {
	_, err := guardedRow(ctx, tx, command)
	return err
}

// Complete records the exact receipt in the same transaction as the effect. It
// repeats the guard rather than trusting that a caller previously called Guard.
func (s *Store) Complete(ctx context.Context, tx *gorm.DB, command writefence.Command, receipt json.RawMessage) error {
	canonical, err := writefence.CanonicalReceipt(receipt)
	if err != nil {
		return err
	}
	row, err := guardedRow(ctx, tx, command)
	if errors.Is(err, writefence.ErrCommitted) {
		// JSONB canonicalizes nested object ordering when read from PostgreSQL.
		var equal bool
		if err := tx.WithContext(ctx).Raw("SELECT ?::jsonb = ?::jsonb", string(row.Receipt), string(canonical)).Scan(&equal).Error; err != nil {
			return err
		}
		if !equal {
			return writefence.ErrConflict
		}
		return nil
	}
	if err != nil {
		return err
	}
	updated := tx.WithContext(ctx).Exec(`UPDATE infra.writer_command
		SET state = 'committed', receipt = ?::jsonb, updated_at = statement_timestamp()
		WHERE service_owner = infra.runtime_owner() AND id = ? AND state = 'admitted'`, string(canonical), command.ID)
	if updated.Error != nil {
		return fmt.Errorf("commit writer receipt: %w", updated.Error)
	}
	if updated.RowsAffected != 1 {
		return writefence.ErrConflict
	}
	return nil
}

// Load returns durable evidence only; it never checks or grants fresh access.
// Transport/application adapters must authorize who can query each original ID.
func (s *Store) Load(ctx context.Context, id uuid.UUID) (writefence.Record, error) {
	if id == uuid.Nil {
		return writefence.Record{}, writefence.ErrInvalid
	}
	if s == nil || s.db == nil {
		return writefence.Record{}, writefence.ErrUnavailable
	}
	row, err := loadRow(ctx, s.db, id, false)
	if err != nil {
		return writefence.Record{}, err
	}
	return record(row)
}

// Freeze permanently forbids a still-admitted effect without discarding its
// admission or external/financial responsibility. Committed evidence is retained.
func (s *Store) Freeze(ctx context.Context, id uuid.UUID) (writefence.Record, error) {
	if id == uuid.Nil {
		return writefence.Record{}, writefence.ErrInvalid
	}
	if s == nil || s.db == nil {
		return writefence.Record{}, writefence.ErrUnavailable
	}
	var saved writefence.Record
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, err := loadRow(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if row.State == writefence.Admitted {
			if err := tx.Exec(`UPDATE infra.writer_command SET state = 'frozen', updated_at = statement_timestamp()
				WHERE service_owner = infra.runtime_owner() AND id = ?`, id).Error; err != nil {
				return err
			}
			row.State = writefence.Frozen
		}
		saved, err = record(row)
		return err
	})
	return saved, err
}

func guardedRow(ctx context.Context, tx *gorm.DB, command writefence.Command) (commandRow, error) {
	binding, err := command.AuthorityDigest()
	if err != nil {
		return commandRow{}, err
	}
	if tx == nil || tx.Statement == nil {
		return commandRow{}, writefence.ErrTransaction
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return commandRow{}, writefence.ErrTransaction
	}
	open, err := lockAuthorities(ctx, tx, command.Authorities)
	if err != nil {
		return commandRow{}, err
	}
	row, err := loadRow(ctx, tx, command.ID, true)
	if err != nil {
		return commandRow{}, err
	}
	if err := sameCommand(row, command, binding); err != nil {
		return commandRow{}, err
	}
	switch row.State {
	case writefence.Committed:
		return row, writefence.ErrCommitted
	case writefence.Frozen:
		return row, writefence.ErrFrozen
	case writefence.Admitted:
		if !open {
			return row, writefence.ErrFenced
		}
		return row, nil
	default:
		return row, writefence.ErrConflict
	}
}

func lockAuthorities(ctx context.Context, tx *gorm.DB, authorities []writefence.Authority) (bool, error) {
	ordered := append([]writefence.Authority(nil), authorities...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Key() < ordered[j].Key() })
	open := true
	for _, authority := range ordered {
		row, err := lockFence(ctx, tx, authority)
		if err != nil {
			return false, err
		}
		open = open && !row.Blocked && row.Epoch == authority.Epoch
	}
	return open, nil
}

func lockFence(ctx context.Context, tx *gorm.DB, authority writefence.Authority) (fenceRow, error) {
	var row fenceRow
	err := tx.WithContext(ctx).Table("infra.writer_fence").Clauses(clause.Locking{Strength: "UPDATE"}).
		Where(ownerPredicate).Where("issuer = ? AND authority_kind = ? AND authority_id = ?", authority.Issuer, authority.Kind, authority.ID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return row, writefence.ErrFenced
	}
	if err != nil {
		return row, fmt.Errorf("lock writer authority: %w", err)
	}
	return row, nil
}

func loadRow(ctx context.Context, db *gorm.DB, id uuid.UUID, lock bool) (commandRow, error) {
	var row commandRow
	query := db.WithContext(ctx).Table("infra.writer_command").Where(ownerPredicate).Where("id = ?", id)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	err := query.Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return row, writefence.ErrNotFound
	}
	if err != nil {
		return row, fmt.Errorf("load writer command: %w", err)
	}
	return row, nil
}

func sameCommand(row commandRow, command writefence.Command, binding string) error {
	if row.RequestSHA256 != command.SHA256 || row.Action != command.Action || row.AuthoritySetSHA256 != binding {
		return writefence.ErrConflict
	}
	return nil
}

func record(row commandRow) (writefence.Record, error) {
	var authorities []writefence.Authority
	if err := json.Unmarshal(row.Authorities, &authorities); err != nil {
		return writefence.Record{}, writefence.ErrConflict
	}
	command := writefence.Command{ID: row.ID, SHA256: row.RequestSHA256, Action: row.Action, Authorities: authorities}
	binding, err := command.AuthorityDigest()
	if err != nil || binding != row.AuthoritySetSHA256 {
		return writefence.Record{}, writefence.ErrConflict
	}
	if row.State != writefence.Admitted && row.State != writefence.Committed && row.State != writefence.Frozen {
		return writefence.Record{}, writefence.ErrConflict
	}
	var receipt json.RawMessage
	if row.State == writefence.Committed {
		// JSONB inserts display whitespace. The SQL storage cap accounts for that;
		// transport and application receipts retain the canonical 1MiB bound.
		var compact bytes.Buffer
		if len(row.Receipt) > 2*writefence.MaxReceiptBytes || json.Compact(&compact, row.Receipt) != nil {
			return writefence.Record{}, writefence.ErrConflict
		}
		receipt, err = writefence.CanonicalReceipt(compact.Bytes())
		if err != nil {
			return writefence.Record{}, writefence.ErrConflict
		}
	} else if len(row.Receipt) != 0 && !bytes.Equal(row.Receipt, []byte("null")) {
		return writefence.Record{}, writefence.ErrConflict
	}
	return writefence.Record{Command: command, State: row.State, Receipt: receipt}, nil
}
