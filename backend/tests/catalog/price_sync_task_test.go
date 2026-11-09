package catalog_test

import (
	"context"
	"errors"
	"testing"
	"time"

	catalogtask "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/task"
	catalogapp "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

type priceSyncStub struct {
	called bool
	err    error
}

func (s *priceSyncStub) Synchronize(context.Context) (catalogapp.PriceCandidateSyncResult, error) {
	s.called = true
	return catalogapp.PriceCandidateSyncResult{}, s.err
}

func TestPriceSyncHandlerSchedulesNextUTCDateAndRejectsNonPeriodicWork(t *testing.T) {
	syncer := &priceSyncStub{}
	handler := catalogtask.PriceSyncHandler(syncer)
	next, err := handler(t.Context(), taskruntime.Job{Kind: catalogtask.PriceSync, BusinessID: "periodic"})
	if err != nil || !syncer.called || next.Location() != time.UTC || next.Hour() != 3 || !next.After(time.Now().UTC()) {
		t.Fatalf("periodic sync result: next=%s called=%v err=%v", next, syncer.called, err)
	}
	if _, err := handler(t.Context(), taskruntime.Job{Kind: catalogtask.PriceSync, BusinessID: "arbitrary"}); err == nil {
		t.Fatal("handler accepted non-periodic sync identity")
	}
}

func TestPriceSyncHandlerReturnsErrorsForDurableRetry(t *testing.T) {
	want := errors.New("temporary source failure")
	syncer := &priceSyncStub{err: want}
	next, err := catalogtask.PriceSyncHandler(syncer)(t.Context(), taskruntime.Job{Kind: catalogtask.PriceSync, BusinessID: "periodic"})
	if !errors.Is(err, want) || !next.IsZero() {
		t.Fatalf("sync failure result: next=%s err=%v", next, err)
	}
}
