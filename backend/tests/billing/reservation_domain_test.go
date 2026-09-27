package billing_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/billing/domain"
)

func heldReservation() domain.Reservation {
	return domain.Reservation{
		ID: uuid.New(), ProjectID: uuid.New(), OperationID: uuid.New(),
		AmountMicros: 100, Status: domain.ReservationHeld,
		CreateTime: time.Date(2026, time.September, 27, 12, 0, 0, 0, time.UTC),
	}
}

func TestReservationSettlementAndRelease(t *testing.T) {
	for _, tc := range []struct {
		name       string
		actual     int64
		capAtQuote bool
		want       domain.Settlement
		status     domain.ReservationStatus
	}{
		{"under quote", 70, false, domain.Settlement{ChargeMicros: 70, ReleasedMicros: 30}, domain.ReservationSettled},
		{"non-token over quote", 130, false, domain.Settlement{ChargeMicros: 130}, domain.ReservationSettled},
		{"token capped", 130, true, domain.Settlement{ChargeMicros: 100, ProviderOverageMicros: 30}, domain.ReservationSettled},
		{"zero cost", 0, false, domain.Settlement{ReleasedMicros: 100}, domain.ReservationReleased},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reservation := heldReservation()
			closed := reservation.CreateTime.Add(time.Minute)
			got, err := reservation.Settle(tc.actual, tc.capAtQuote, closed)
			if err != nil || got != tc.want || reservation.Status != tc.status ||
				reservation.ClosedAt == nil || !reservation.ClosedAt.Equal(closed) {
				t.Fatalf("Settle = %+v, %v; reservation=%+v", got, err, reservation)
			}
			if err := reservation.Validate(); err != nil {
				t.Fatalf("closed reservation: %v", err)
			}
			if _, err := reservation.Settle(tc.actual, tc.capAtQuote, closed); !errors.Is(err, domain.ErrReservationClosed) {
				t.Fatalf("repeated settlement = %v", err)
			}
		})
	}
}

func TestReservationRejectsInvalidTimeAndAmountWithoutMutation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		actual int64
		closed time.Time
	}{
		{"negative actual", -1, heldReservation().CreateTime.Add(time.Minute)},
		{"zero close time", 10, time.Time{}},
		{"before creation", 10, heldReservation().CreateTime.Add(-time.Second)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reservation := heldReservation()
			before := reservation
			if _, err := reservation.Settle(tc.actual, false, tc.closed); err == nil || reservation != before {
				t.Fatalf("Settle invalid = %v, reservation=%+v", err, reservation)
			}
		})
	}
	invalid := heldReservation()
	invalid.ProjectID = uuid.Nil
	if err := invalid.Validate(); !errors.Is(err, domain.ErrInvalidReservation) {
		t.Fatalf("missing project = %v", err)
	}
	invalid = heldReservation()
	invalid.ClosedAt = &invalid.CreateTime
	if err := invalid.Validate(); !errors.Is(err, domain.ErrInvalidReservation) {
		t.Fatalf("held with close time = %v", err)
	}
}
