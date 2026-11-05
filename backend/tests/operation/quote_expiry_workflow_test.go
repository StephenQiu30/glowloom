package operation_test

import (
	"context"
	"testing"
	"time"

	operationapp "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

type expiryPageStore struct {
	pages  []operationapp.ExpiredQuotes
	cutoff []time.Time
}

func (s *expiryPageStore) ExpireQuoted(_ context.Context, cutoff time.Time, _ int) (operationapp.ExpiredQuotes, error) {
	s.cutoff = append(s.cutoff, cutoff)
	page := s.pages[0]
	s.pages = s.pages[1:]
	return page, nil
}

func TestQuoteExpiryServiceDrainsOperationsAndBatchesAtOneCutoff(t *testing.T) {
	store := &expiryPageStore{pages: []operationapp.ExpiredQuotes{
		{Operations: 1, Batches: 1},
		{Operations: 1},
		{},
	}}
	cutoff := time.Now().UTC()
	total, err := operationapp.NewQuoteExpiryService(store, 1).Run(t.Context(), cutoff, nil)
	if err != nil || total.Operations != 2 || total.Batches != 1 || len(store.cutoff) != 3 {
		t.Fatalf("drain = %+v, %v, cutoffs=%v", total, err, store.cutoff)
	}
	for _, got := range store.cutoff {
		if !got.Equal(cutoff) {
			t.Fatalf("quote expiry cutoff moved: %v, want %v", got, cutoff)
		}
	}
}
