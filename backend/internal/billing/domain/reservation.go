package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	// ErrInvalidReservation means a persisted reservation violates its lifecycle.
	ErrInvalidReservation = errors.New("invalid reservation")
	// ErrReservationClosed means a reservation can no longer be settled again.
	ErrReservationClosed = errors.New("reservation already closed")
)

// ReservationStatus is the persisted lifecycle of a held budget amount.
type ReservationStatus string

// Reservation states match billing.reservation.status.
const (
	ReservationHeld     ReservationStatus = "held"
	ReservationSettled  ReservationStatus = "settled"
	ReservationReleased ReservationStatus = "released"
)

// Reservation freezes the confirmed quote for exactly one operation.
type Reservation struct {
	ID           uuid.UUID
	ProjectID    uuid.UUID
	OperationID  uuid.UUID
	AmountMicros int64
	Status       ReservationStatus
	CreateTime   time.Time
	ClosedAt     *time.Time
}

// Validate checks project scope, amount, and closure timestamps.
func (r Reservation) Validate() error {
	if r.ID == uuid.Nil || r.ProjectID == uuid.Nil || r.OperationID == uuid.Nil ||
		r.AmountMicros < 0 || r.CreateTime.IsZero() {
		return ErrInvalidReservation
	}
	switch r.Status {
	case ReservationHeld:
		if r.ClosedAt != nil {
			return ErrInvalidReservation
		}
	case ReservationSettled, ReservationReleased:
		if r.ClosedAt == nil || r.ClosedAt.IsZero() || r.ClosedAt.Before(r.CreateTime) {
			return ErrInvalidReservation
		}
	default:
		return ErrInvalidReservation
	}
	return nil
}

// Settlement separates a customer's charge, returned reservation, and any
// token-metered excess paid by the platform. Budget and ledger writes must
// commit in the same transaction as the reservation status change.
type Settlement struct {
	ChargeMicros          int64
	ReleasedMicros        int64
	ProviderOverageMicros int64
}

// Settle closes a held reservation using the supplier's actual nonnegative
// cost. capAtReservation applies only to token-metered charges.
func (r *Reservation) Settle(actualMicros int64, capAtReservation bool, closedAt time.Time) (Settlement, error) {
	if r == nil || r.Validate() != nil {
		return Settlement{}, ErrInvalidReservation
	}
	if r.Status != ReservationHeld {
		return Settlement{}, ErrReservationClosed
	}
	if actualMicros < 0 || closedAt.IsZero() || closedAt.Before(r.CreateTime) {
		return Settlement{}, ErrInvalidReservation
	}
	result := Settlement{ChargeMicros: actualMicros}
	if actualMicros > r.AmountMicros && capAtReservation {
		result.ChargeMicros = r.AmountMicros
		result.ProviderOverageMicros = actualMicros - r.AmountMicros
	}
	if result.ChargeMicros < r.AmountMicros {
		result.ReleasedMicros = r.AmountMicros - result.ChargeMicros
	}
	if result.ChargeMicros == 0 {
		r.Status = ReservationReleased
	} else {
		r.Status = ReservationSettled
	}
	r.ClosedAt = &closedAt
	return result, nil
}
