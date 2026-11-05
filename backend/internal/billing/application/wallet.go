package application

import (
	"time"

	"github.com/google/uuid"
)

// WalletView is the authenticated creation scope's balance and movement page.
type WalletView struct {
	Enabled           bool          `json:"enabled"`
	Payer             string        `json:"payer"`
	GeneralCredits    string        `json:"general_credits"`
	RestrictedCredits string        `json:"restricted_credits"`
	HeldCredits       string        `json:"held_credits"`
	RedeemedToday     string        `json:"redeemed_today"`
	HistoryAvailable  bool          `json:"history_available"`
	Entries           []WalletEntry `json:"entries"`
	NextCursor        *uuid.UUID    `json:"next_cursor,omitempty"`
}

// WalletEntry is one immutable movement attributed to a funding lot.
type WalletEntry struct {
	ID          uuid.UUID  `json:"id"`
	Kind        string     `json:"kind"`
	Credits     string     `json:"credits"`
	Source      string     `json:"source"`
	ModelKey    *string    `json:"model_key,omitempty"`
	OperationID *uuid.UUID `json:"operation_id,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// WalletDemand is a server-priced item in confirmation order.
type WalletDemand struct {
	Micros               int64
	ModelKey             string
	CommercialPriceReady bool
	ReusedResult         bool
}
