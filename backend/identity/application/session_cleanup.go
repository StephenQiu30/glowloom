package application

import (
	"context"
	"fmt"
	"time"
)

// SessionReceiptCleanup clears short recovery ciphertext through the existing store.
type SessionReceiptCleanup struct {
	store EmailStore
	now   func() time.Time
}

// NewSessionReceiptCleanup retains the existing identity transaction boundary.
func NewSessionReceiptCleanup(store EmailStore, now func() time.Time) *SessionReceiptCleanup {
	if now == nil {
		now = time.Now
	}
	return &SessionReceiptCleanup{store: store, now: now}
}

// RunOnce bounds work; digest retention is decided by immutable session deadlines.
func (c *SessionReceiptCleanup) RunOnce(ctx context.Context, limit int) (int, error) {
	if c == nil || c.store == nil || limit < 1 || limit > 1000 {
		return 0, fmt.Errorf("receipt cleanup requires store and limit between 1 and 1000")
	}
	var count int
	err := c.store.EmailTransaction(ctx, func(tx EmailTx) error {
		var err error
		count, err = tx.CleanupExpiredRefresh(c.now().UTC(), limit)
		return err
	})
	if err != nil {
		return 0, err
	}
	return count, nil
}
