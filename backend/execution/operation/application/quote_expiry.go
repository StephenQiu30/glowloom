package application

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ErrInvalidQuoteExpiry means a sweep has no valid cutoff or batch size.
var ErrInvalidQuoteExpiry = errors.New("invalid quote expiry sweep")

// ExpiredQuotes reports rows changed by one bounded sweep batch.
type ExpiredQuotes struct {
	Operations int64
	Batches    int64
}

// QuoteExpiryStore changes only unconfirmed quotes and fully expired batches.
type QuoteExpiryStore interface {
	ExpireQuoted(context.Context, time.Time, int) (ExpiredQuotes, error)
}

// QuoteExpiryService drains bounded batches for one schedule execution.
type QuoteExpiryService struct {
	store     QuoteExpiryStore
	batchSize int
}

// NewQuoteExpiryService injects the persistent store and bounded batch size.
func NewQuoteExpiryService(store QuoteExpiryStore, batchSize int) *QuoteExpiryService {
	return &QuoteExpiryService{store: store, batchSize: batchSize}
}

// Run uses a single cutoff for the whole invocation and reports durable progress.
func (s *QuoteExpiryService) Run(ctx context.Context, now time.Time, progress func(ExpiredQuotes)) (ExpiredQuotes, error) {
	if s == nil || s.store == nil || s.batchSize < 1 || s.batchSize > 1000 || now.IsZero() {
		return ExpiredQuotes{}, ErrInvalidQuoteExpiry
	}
	var total ExpiredQuotes
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		changed, err := s.store.ExpireQuoted(ctx, now.UTC(), s.batchSize)
		if err != nil {
			return total, fmt.Errorf("expire quoted operations and batches: %w", err)
		}
		total.Operations += changed.Operations
		total.Batches += changed.Batches
		if progress != nil {
			progress(total)
		}
		if changed.Operations < int64(s.batchSize) && changed.Batches < int64(s.batchSize) {
			return total, nil
		}
	}
}
