package domain

import (
	"encoding/hex"
	"encoding/json"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

// PriceCandidateStatus tracks public supplier data before it becomes a price version.
type PriceCandidateStatus string

const (
	// PriceCandidatePendingReview is a normalized public quote awaiting approval.
	PriceCandidatePendingReview PriceCandidateStatus = "pending_review"
	// PriceCandidateNeedsReview cannot be normalized to a supported billing rule.
	PriceCandidateNeedsReview PriceCandidateStatus = "needs_review"
	// PriceCandidatePublished has been atomically linked to a reviewed price version.
	PriceCandidatePublished PriceCandidateStatus = "published"
	// PriceCandidateRejected was reviewed and explicitly declined.
	PriceCandidateRejected PriceCandidateStatus = "rejected"
)

// PriceCandidate is a public-source snapshot; it is never a billing input.
type PriceCandidate struct {
	ID                    uuid.UUID
	AdapterKey            string
	ProviderModelID       string
	SourceURL             string
	SourceHash            string
	ParserVersion         string
	SourceFetchedAt       time.Time
	Currency              string
	Unit                  *PriceUnit
	Rule                  json.RawMessage
	SourcePricing         json.RawMessage
	Status                PriceCandidateStatus
	PublishedPriceVersion *uuid.UUID
	ReviewedBy            *uuid.UUID
	ReviewedAt            *time.Time
	CreateTime            time.Time
}

// Validate checks a source snapshot before it is persisted or reviewed.
func (c PriceCandidate) Validate() error {
	if c.ID == uuid.Nil || !validPriceCandidateIdentifier(c.AdapterKey, 64) ||
		strings.TrimSpace(c.ProviderModelID) == "" || len(c.ProviderModelID) > 256 ||
		strings.ContainsAny(c.ProviderModelID, "\x00\r\n") || c.ParserVersion == "" || len(c.ParserVersion) > 64 ||
		c.SourceFetchedAt.IsZero() || len(c.Currency) != 3 || !json.Valid(c.SourcePricing) ||
		!json.Valid(c.Rule) && len(c.Rule) != 0 {
		return ErrInvalidPriceRule
	}
	for _, character := range c.Currency {
		if character < 'A' || character > 'Z' {
			return ErrInvalidPriceRule
		}
	}
	parsed, err := url.Parse(c.SourceURL)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" || len(c.SourceURL) > 2048 {
		return ErrInvalidPriceRule
	}
	hash, err := hex.DecodeString(c.SourceHash)
	if err != nil || len(hash) != 32 {
		return ErrInvalidPriceRule
	}
	var pricing map[string]json.RawMessage
	if json.Unmarshal(c.SourcePricing, &pricing) != nil || pricing == nil {
		return ErrInvalidPriceRule
	}
	switch c.Status {
	case PriceCandidatePendingReview:
		if c.Unit == nil || !jsonShape(c.Rule, '{') || c.ReviewedBy != nil || c.ReviewedAt != nil || c.PublishedPriceVersion != nil {
			return ErrInvalidPriceRule
		}
	case PriceCandidateNeedsReview:
		if c.PublishedPriceVersion != nil || c.ReviewedBy != nil || c.ReviewedAt != nil ||
			c.Unit == nil && len(c.Rule) != 0 {
			return ErrInvalidPriceRule
		}
	case PriceCandidatePublished:
		if c.Unit == nil || !jsonShape(c.Rule, '{') || c.PublishedPriceVersion == nil || *c.PublishedPriceVersion == uuid.Nil ||
			c.ReviewedBy == nil || *c.ReviewedBy == uuid.Nil || c.ReviewedAt == nil || c.ReviewedAt.IsZero() {
			return ErrInvalidPriceRule
		}
	case PriceCandidateRejected:
		if c.PublishedPriceVersion != nil || c.ReviewedBy == nil || *c.ReviewedBy == uuid.Nil || c.ReviewedAt == nil || c.ReviewedAt.IsZero() {
			return ErrInvalidPriceRule
		}
	default:
		return ErrInvalidPriceRule
	}
	if c.Unit != nil {
		switch *c.Unit {
		case PricePerImage, PricePerSecond, PricePerRequest, PricePer1KTokens, PricePer1KChars:
		default:
			return ErrInvalidPriceRule
		}
	}
	return nil
}

func validPriceCandidateIdentifier(value string, limit int) bool {
	if value == "" || len(value) > limit {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '_' || character == '-' {
			continue
		}
		return false
	}
	return true
}
