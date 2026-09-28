package postgres

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	billingdomain "github.com/StephenQiu30/lanverse/backend/internal/billing/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
)

type confirmationKey struct {
	orgID       uuid.UUID
	actorID     uuid.UUID
	requestID   uuid.UUID
	fingerprint [sha256.Size]byte
}

type confirmationOutcome struct {
	Result    json.RawMessage `json:"result"`
	ErrorCode string          `json:"error_code,omitempty"`
	Reasons   []string        `json:"reasons,omitempty"`
}

func singleConfirmationKey(actor identityapp.Principal, input application.ConfirmSingleQuoteInput) confirmationKey {
	return confirmationKey{
		orgID: actor.OrgID, actorID: actor.ID, requestID: uuid.MustParse(input.RequestID),
		fingerprint: sha256.Sum256([]byte("operation.confirm:" + input.ProjectID.String() + ":" + input.OperationID.String())),
	}
}

func batchConfirmationKey(actor identityapp.Principal, input application.ConfirmBatchQuoteInput) confirmationKey {
	excluded := make([]string, 0, len(input.ExcludeOperationIDs))
	for _, id := range input.ExcludeOperationIDs {
		excluded = append(excluded, id.String())
	}
	sort.Strings(excluded)
	return confirmationKey{
		orgID: actor.OrgID, actorID: actor.ID, requestID: uuid.MustParse(input.RequestID),
		fingerprint: sha256.Sum256([]byte("batch.confirm:" + input.ProjectID.String() + ":" +
			input.BatchID.String() + ":" + strings.Join(excluded, ","))),
	}
}

func (key confirmationKey) advisoryParts() (int32, int32) {
	sum := sha256.Sum256([]byte(key.actorID.String() + ":" + key.requestID.String()))
	return int32(binary.BigEndian.Uint32(sum[:4])), int32(binary.BigEndian.Uint32(sum[4:8]))
}

// withConfirmationKey pins one PostgreSQL session across a batch's separate
// stale-item and reservation transactions. The session lock also serializes
// concurrent retries on other API processes. A successful outcome is inserted
// by the reservation transaction before this callback returns.
func (s *Store) withConfirmationKey(ctx context.Context, actor identityapp.Principal,
	key confirmationKey, run func(*Store, confirmationKey) (confirmationOutcome, error),
) (outcome confirmationOutcome, err error) {
	if s == nil || s.db == nil {
		return confirmationOutcome{}, ErrUnavailable
	}
	if key.actorID == uuid.Nil || key.orgID == uuid.Nil {
		return confirmationOutcome{}, identityapp.ErrForbidden
	}
	// A caller-managed transaction is used by ACL and rollback probes. The
	// production API injects a pool-backed store, where transactions A and B
	// can commit independently on one pinned session.
	if _, inTx := s.db.Statement.ConnPool.(gorm.TxCommitter); inTx {
		return run(s, confirmationKey{})
	}
	if _, dbErr := s.db.DB(); dbErr != nil {
		return run(s, confirmationKey{})
	}
	err = s.db.WithContext(ctx).Connection(func(conn *gorm.DB) (callbackErr error) {
		first, second := key.advisoryParts()
		if lockErr := conn.Exec(`SELECT pg_advisory_lock(?, ?)`, first, second).Error; lockErr != nil {
			return fmt.Errorf("lock confirmation request: %w", lockErr)
		}
		defer func() {
			unlockCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			var released bool
			unlockErr := conn.WithContext(unlockCtx).Raw(`SELECT pg_advisory_unlock(?, ?)`, first, second).Scan(&released).Error
			if unlockErr != nil {
				callbackErr = errors.Join(callbackErr, fmt.Errorf("unlock confirmation request: %w", unlockErr))
			} else if !released {
				callbackErr = errors.Join(callbackErr, errors.New("confirmation request lock was not held"))
			}
		}()
		if actorErr := conn.Transaction(func(tx *gorm.DB) error { return requireCurrentActor(tx, actor) }); actorErr != nil {
			return actorErr
		}
		stored, found, readErr := readConfirmationOutcome(conn, key)
		if readErr != nil {
			return readErr
		}
		if found {
			outcome = stored
			return replayConfirmationError(stored)
		}
		outcome, callbackErr = run(NewStore(conn), key)
		if callbackErr != nil && outcome.ErrorCode != "" {
			if writeErr := conn.Transaction(func(tx *gorm.DB) error {
				return insertConfirmationOutcome(tx, key, outcome)
			}); writeErr != nil {
				return errors.Join(callbackErr, fmt.Errorf("record rejected confirmation: %w", writeErr))
			}
		}
		return callbackErr
	})
	return outcome, err
}

func readConfirmationOutcome(db *gorm.DB, key confirmationKey) (confirmationOutcome, bool, error) {
	var row struct {
		Fingerprint []byte
		Outcome     []byte
	}
	read := db.Raw(`SELECT fingerprint, outcome FROM operation.confirmation_request
		WHERE org_id = ?::uuid AND actor_id = ?::uuid AND request_id = ?::uuid`,
		key.orgID.String(), key.actorID.String(), key.requestID.String()).Scan(&row)
	if read.Error != nil {
		return confirmationOutcome{}, false, fmt.Errorf("read confirmation request: %w", read.Error)
	}
	if read.RowsAffected == 0 {
		return confirmationOutcome{}, false, nil
	}
	if !bytes.Equal(row.Fingerprint, key.fingerprint[:]) {
		return confirmationOutcome{}, false, application.ErrConfirmationKeyReused
	}
	var outcome confirmationOutcome
	if err := json.Unmarshal(row.Outcome, &outcome); err != nil {
		return confirmationOutcome{}, false, fmt.Errorf("decode confirmation outcome: %w", err)
	}
	return outcome, true, nil
}

func insertConfirmationOutcome(tx *gorm.DB, key confirmationKey, outcome confirmationOutcome) error {
	payload, err := json.Marshal(outcome)
	if err != nil {
		return fmt.Errorf("encode confirmation outcome: %w", err)
	}
	write := tx.Exec(`INSERT INTO operation.confirmation_request
		(org_id, actor_id, request_id, fingerprint, outcome)
		VALUES (?::uuid, ?::uuid, ?::uuid, ?, ?::jsonb)`,
		key.orgID.String(), key.actorID.String(), key.requestID.String(), key.fingerprint[:], string(payload))
	if write.Error != nil {
		return fmt.Errorf("insert confirmation request: %w", write.Error)
	}
	if write.RowsAffected != 1 {
		return fmt.Errorf("insert confirmation request: wrote %d rows", write.RowsAffected)
	}
	return nil
}

func makeConfirmationOutcome(result any, err error) (confirmationOutcome, error) {
	payload, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		return confirmationOutcome{}, fmt.Errorf("encode confirmation result: %w", marshalErr)
	}
	outcome := confirmationOutcome{Result: payload}
	switch {
	case err == nil:
	case errors.Is(err, application.ErrQuoteStale):
		outcome.ErrorCode = "quote_expired"
		var stale *QuoteStaleError
		if errors.As(err, &stale) {
			outcome.Reasons = stale.Reasons
		}
	case errors.Is(err, billingdomain.ErrBudgetInsufficient):
		outcome.ErrorCode = "budget_insufficient"
	case errors.Is(err, domain.ErrQuoteNotConfirmable):
		outcome.ErrorCode = "state_conflict"
	case errors.Is(err, application.ErrConfirmationTargetUnavailable):
		outcome.ErrorCode = "target_unavailable"
	default:
		// Dependency failures and access checks are not stable outcomes.
	}
	return outcome, nil
}

func replayConfirmationError(outcome confirmationOutcome) error {
	switch outcome.ErrorCode {
	case "":
		return nil
	case "quote_expired":
		if len(outcome.Reasons) > 0 {
			return &QuoteStaleError{Reasons: outcome.Reasons}
		}
		return application.ErrQuoteStale
	case "budget_insufficient":
		return billingdomain.ErrBudgetInsufficient
	case "state_conflict":
		return domain.ErrQuoteNotConfirmable
	case "target_unavailable":
		return application.ErrConfirmationTargetUnavailable
	default:
		return fmt.Errorf("unknown confirmation outcome %q", outcome.ErrorCode)
	}
}
