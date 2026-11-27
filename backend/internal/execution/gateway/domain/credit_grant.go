package domain

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// MaxCreditGrant is the maximum integer award accepted in one order.
const MaxCreditGrant = 1_000_000_000

// CreditGrantInput describes a proposal to issue general-purpose promotional credits.
type CreditGrantInput struct {
	ScopeID uuid.UUID `json:"scope_id" binding:"required"`
	Key     string    `json:"key" binding:"required"`
	Credits int64     `json:"credits" binding:"required"`
	Reason  string    `json:"reason" binding:"required"`
}

// Normalize makes retries independent of unintentional surrounding whitespace.
func (in CreditGrantInput) Normalize() (CreditGrantInput, error) {
	in.Key = strings.TrimSpace(in.Key)
	in.Reason = strings.TrimSpace(in.Reason)
	if in.ScopeID == uuid.Nil || in.Credits < 1 || in.Credits > MaxCreditGrant ||
		in.Key == "" || utf8.RuneCountInString(in.Key) > 120 || strings.ContainsRune(in.Key, 0) ||
		in.Reason == "" || utf8.RuneCountInString(in.Reason) > 500 || strings.ContainsRune(in.Reason, 0) {
		return CreditGrantInput{}, ErrInvalid
	}
	return in, nil
}

// CreditGrantOrder includes the supervisor's decision and the wallet receipt after issuance.
type CreditGrantOrder struct {
	CreditGrantInput
	ID             uuid.UUID  `json:"id" binding:"required"`
	ProposerID     uuid.UUID  `json:"proposer_id" binding:"required"`
	RecipientKind  string     `json:"recipient_kind" binding:"required"`
	RecipientLabel string     `json:"recipient_label" binding:"required"`
	State          string     `json:"state" binding:"required" enums:"pending,issued,rejected"`
	CreatedAt      time.Time  `json:"created_at" binding:"required"`
	ReviewerID     *uuid.UUID `json:"reviewer_id,omitempty"`
	ReviewedAt     *time.Time `json:"reviewed_at,omitempty"`
	EventID        *uuid.UUID `json:"event_id,omitempty"`
}

// CreditGrantReview explicitly approves or rejects a submitted grant order.
type CreditGrantReview struct {
	Approve *bool `json:"approve" binding:"required"`
}
