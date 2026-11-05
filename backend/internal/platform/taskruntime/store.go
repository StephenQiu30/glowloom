// Package taskruntime leases execution windows for existing business records.
package taskruntime

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrStopRequested reports a durable request to stop the current business step.
var ErrStopRequested = errors.New("task stop requested")

// ErrLeaseLost rejects operations whose task claim or fence is no longer current.
var ErrLeaseLost = errors.New("task lease lost")

// Job is one claimed task with its durable identity and execution fence.
type Job struct {
	Kind       string
	BusinessID string
	Owner      string
	Epoch      int64
	Attempt    int
	Reference  []byte
	Failures   int
}

// Store persists task claims and enforces their owner and epoch fences.
type Store struct {
	db         *gorm.DB
	businessID string
}

type taskWorkRow struct {
	Kind            string     `gorm:"column:kind;primaryKey"`
	BusinessID      string     `gorm:"column:business_id;primaryKey"`
	NextRunAt       time.Time  `gorm:"column:next_run_at"`
	Owner           *string    `gorm:"column:owner"`
	Epoch           int64      `gorm:"column:epoch"`
	LeaseUntil      *time.Time `gorm:"column:lease_until"`
	Attempt         int        `gorm:"column:attempt"`
	Reference       taskJSON   `gorm:"column:reference"`
	Failures        int        `gorm:"column:failures"`
	FailedAt        *time.Time `gorm:"column:failed_at"`
	StopRequestedAt *time.Time `gorm:"column:stop_requested_at"`
	CompletedAt     *time.Time `gorm:"column:completed_at"`
}

func (taskWorkRow) TableName() string { return "infra.task_work" }

type taskJSON []byte

func (j *taskJSON) Scan(value any) error {
	switch value := value.(type) {
	case nil:
		*j = nil
	case []byte:
		*j = append((*j)[:0], value...)
	case string:
		*j = append((*j)[:0], value...)
	default:
		return fmt.Errorf("scan task reference from %T", value)
	}
	return nil
}

func (j taskJSON) Value() (driver.Value, error) {
	if len(j) == 0 {
		return "null", nil
	}
	if !json.Valid(j) {
		return nil, errors.New("invalid task reference")
	}
	return string(j), nil
}

func (taskJSON) GormDataType() string { return "jsonb" }

// NewStore injects the database used for durable task claims.
func NewStore(db *gorm.DB) *Store { return &Store{db: db} }

// ForBusiness restricts a directed wakeup to one already admitted business ID.
func (s *Store) ForBusiness(id string) *Store { return &Store{db: s.db, businessID: id} }

// Enqueue inserts a single wakeup in the caller's transaction. Repeated admission
// cannot reopen completed work or move a delayed task back to the head of the queue.
func Enqueue(ctx context.Context, tx *gorm.DB, kind, id string, at time.Time) error {
	return EnqueueReference(ctx, tx, kind, id, at, nil)
}

// EnqueueReference keeps only the immutable identifiers needed to find the domain
// work. Callers must keep provider inputs, outputs and secrets in their own stores.
func EnqueueReference(ctx context.Context, tx *gorm.DB, kind, id string, at time.Time, ref any) error {
	if tx == nil || kind == "" || id == "" || at.IsZero() {
		return errors.New("invalid task admission")
	}
	raw, err := json.Marshal(ref)
	if err != nil {
		return err
	}
	row := taskWorkRow{Kind: kind, BusinessID: id, NextRunAt: at.UTC(), Reference: taskJSON(raw)}
	return tx.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "kind"}, {Name: "business_id"}},
		DoNothing: true,
	}).Create(&row).Error
}

// Claim selects one due task and assigns an owner and a new execution fence.
func (s *Store) Claim(ctx context.Context, kinds []string, owner string, ttl time.Duration) (*Job, error) {
	if s == nil || s.db == nil || owner == "" || len(kinds) == 0 || ttl <= 0 {
		return nil, errors.New("invalid task claim")
	}
	now := time.Now().UTC()
	var claimed *Job
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Where("kind IN ? AND next_run_at <= ?", kinds, now).
			Where("completed_at IS NULL AND failed_at IS NULL").
			Where("lease_until IS NULL OR lease_until <= ?", now).
			Order("next_run_at").Order("kind").Order("business_id").
			Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"})
		if s.businessID != "" {
			query = query.Where("business_id = ?", s.businessID)
		}
		var row taskWorkRow
		if err := query.Take(&row).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		} else if err != nil {
			return err
		}
		leaseUntil := now.Add(ttl)
		result := tx.Model(&taskWorkRow{}).
			Where("kind = ? AND business_id = ?", row.Kind, row.BusinessID).
			Updates(map[string]any{
				"owner":       owner,
				"epoch":       row.Epoch + 1,
				"attempt":     row.Attempt + 1,
				"lease_until": leaseUntil,
			})
		if err := result.Error; err != nil {
			return err
		}
		if result.RowsAffected != 1 {
			return ErrLeaseLost
		}
		claimed = &Job{
			Kind: row.Kind, BusinessID: row.BusinessID, Owner: owner,
			Epoch: row.Epoch + 1, Attempt: row.Attempt + 1,
			Reference: append([]byte(nil), row.Reference...), Failures: row.Failures,
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("claim task: %w", err)
	}
	return claimed, nil
}

func fenced(query *gorm.DB) error {
	if query.Error != nil {
		return query.Error
	}
	if query.RowsAffected != 1 {
		return ErrLeaseLost
	}
	return nil
}

func leaseQuery(tx *gorm.DB, j Job, now time.Time) *gorm.DB {
	return tx.Model(&taskWorkRow{}).
		Where("kind = ? AND business_id = ? AND owner = ? AND epoch = ?", j.Kind, j.BusinessID, j.Owner, j.Epoch).
		Where("lease_until > ? AND completed_at IS NULL", now)
}

func (s *Store) update(ctx context.Context, j Job, values map[string]any) error {
	if s == nil || s.db == nil {
		return ErrLeaseLost
	}
	return fenced(leaseQuery(s.db.WithContext(ctx), j, time.Now().UTC()).Updates(values))
}

// Renew extends a current claim and reports a durable stop request.
func (s *Store) Renew(ctx context.Context, j Job, ttl time.Duration) error {
	if s == nil || s.db == nil || ttl <= 0 {
		return errors.New("invalid lease duration")
	}
	now := time.Now().UTC()
	var stopped bool
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row taskWorkRow
		if err := leaseQuery(tx, j, now).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrLeaseLost
			}
			return err
		}
		result := tx.Model(&taskWorkRow{}).
			Where("kind = ? AND business_id = ? AND owner = ? AND epoch = ?", j.Kind, j.BusinessID, j.Owner, j.Epoch).
			Where("lease_until > ? AND completed_at IS NULL", now).
			Update("lease_until", now.Add(ttl))
		if err := fenced(result); err != nil {
			return err
		}
		stopped = row.StopRequestedAt != nil
		return nil
	})
	if err != nil {
		return err
	}
	if stopped {
		return ErrStopRequested
	}
	return nil
}

// Complete closes a current execution window after the business step finishes.
func (s *Store) Complete(ctx context.Context, j Job) error {
	now := time.Now().UTC()
	return s.update(ctx, j, map[string]any{"completed_at": now, "owner": nil, "lease_until": nil})
}

// Reschedule releases the claim and durably waits until the requested time.
func (s *Store) Reschedule(ctx context.Context, j Job, at time.Time) error {
	if at.IsZero() {
		return errors.New("missing next run")
	}
	return s.update(ctx, j, map[string]any{"next_run_at": at.UTC(), "failures": 0, "owner": nil, "lease_until": nil})
}

// Fence checks and locks this execution window inside the short business transaction
// that publishes progress or results. No external IO may run in this transaction.
func Fence(ctx context.Context, tx *gorm.DB, j Job) error {
	if tx == nil {
		return ErrLeaseLost
	}
	var row taskWorkRow
	err := leaseQuery(tx.WithContext(ctx), j, time.Now().UTC()).
		Clauses(clause.Locking{Strength: "UPDATE"}).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrLeaseLost
	}
	return err
}

// retry closes a failing execution window and eventually parks the work for
// operator investigation. It never changes a domain job's outcome.
func (s *Store) retry(ctx context.Context, j Job) error {
	now := time.Now().UTC()
	values := map[string]any{"failures": j.Failures + 1, "owner": nil, "lease_until": nil}
	if j.Failures >= 5 {
		values["failed_at"] = now
	} else {
		values["next_run_at"] = now.Add(time.Duration(1<<j.Failures) * time.Second)
	}
	return s.update(ctx, j, values)
}

type claimKey struct{}

// Transaction checks the lease before committing domain progress. Side effects
// keep their own idempotency keys. The lease row is locked only at the end of
// the transaction to avoid blocking renewal during domain work.
func Transaction(ctx context.Context, db *gorm.DB, fn func(*gorm.DB) error) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := fn(tx); err != nil {
			return err
		}
		j, ok := ctx.Value(claimKey{}).(Job)
		if !ok {
			return nil
		}
		return Fence(ctx, tx, j)
	})
}

// RequestStop asks active windows matching an immutable business reference to
// cancel owned IO. Only the domain's actual cleanup can mark a job stopped.
func RequestStop(ctx context.Context, tx *gorm.DB, kind string, match any) error {
	if tx == nil || kind == "" {
		return errors.New("invalid task stop request")
	}
	raw, err := json.Marshal(match)
	if err != nil {
		return err
	}
	return tx.WithContext(ctx).Model(&taskWorkRow{}).
		Where("kind = ? AND owner IS NOT NULL AND completed_at IS NULL AND reference @> ?", kind, taskJSON(raw)).
		Where("stop_requested_at IS NULL").
		Update("stop_requested_at", time.Now().UTC()).Error
}
