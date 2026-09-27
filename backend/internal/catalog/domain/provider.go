// Package domain owns provider and encrypted credential invariants.
package domain

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

var (
	// ErrInvalidProvider means provider settings violate the persisted contract.
	ErrInvalidProvider = errors.New("invalid provider")
	// ErrProviderDisabled means a disabled provider cannot accept new work.
	ErrProviderDisabled = errors.New("provider is disabled")
	// ErrInvalidCredential means an encrypted credential has invalid metadata.
	ErrInvalidCredential = errors.New("invalid provider credential")
	// ErrCredentialDisabled means a disabled credential cannot be tested again.
	ErrCredentialDisabled = errors.New("provider credential is disabled")
	// ErrInvalidTestResult means an unknown provider test result was supplied.
	ErrInvalidTestResult = errors.New("invalid credential test result")
)

// Region identifies the provider's processing region.
type Region string

// Provider regions distinguish domestic and overseas processing.
const (
	RegionDomestic Region = "domestic"
	RegionOverseas Region = "overseas"
)

// ProviderStatus is the provider's availability for new submissions.
type ProviderStatus string

// Provider statuses distinguish usable and disabled providers.
const (
	ProviderActive   ProviderStatus = "active"
	ProviderDisabled ProviderStatus = "disabled"
)

// Provider is the platform-level adapter and rate-limit configuration.
type Provider struct {
	ID               uuid.UUID
	Key              string
	Name             string
	AdapterKey       string
	Region           Region
	Status           ProviderStatus
	ConcurrencyLimit int
	RateLimitPerMin  int
	Revision         int64
	CreateTime       time.Time
	UpdateTime       time.Time
}

// Validate checks fields before a provider is persisted or updated.
func (p Provider) Validate() error {
	if p.ID == uuid.Nil || strings.TrimSpace(p.Key) == "" || strings.TrimSpace(p.Name) == "" ||
		strings.TrimSpace(p.AdapterKey) == "" ||
		(p.Region != RegionDomestic && p.Region != RegionOverseas) ||
		(p.Status != ProviderActive && p.Status != ProviderDisabled) ||
		p.ConcurrencyLimit < 1 || p.RateLimitPerMin < 1 || p.Revision < 1 {
		return ErrInvalidProvider
	}
	return nil
}

// Disable stops new submissions and advances the optimistic revision once.
func (p *Provider) Disable() error {
	if err := p.Validate(); err != nil {
		return err
	}
	if p.Status == ProviderDisabled {
		return ErrProviderDisabled
	}
	p.Status = ProviderDisabled
	p.Revision++
	return nil
}

// CredentialStatus is the availability of a saved encrypted credential.
type CredentialStatus string

// Credential statuses distinguish current and historical records.
const (
	CredentialActive   CredentialStatus = "active"
	CredentialDisabled CredentialStatus = "disabled"
)

// TestResult is a safe category for a free credential connectivity check.
type TestResult string

// Credential test outcomes never contain a provider's raw error response.
const (
	TestOK          TestResult = "ok"
	TestAuthFailed  TestResult = "auth_failed"
	TestUnreachable TestResult = "unreachable"
	TestTimeout     TestResult = "timeout"
	TestUnsupported TestResult = "unsupported"
)

// Credential contains encrypted bytes for the worker and safe display metadata.
// Ciphertext must not appear in an API response or structured log.
type Credential struct {
	ID             uuid.UUID
	ProviderID     uuid.UUID
	Label          string
	Ciphertext     []byte `json:"-"`
	KeyID          string
	Last4          string
	Status         CredentialStatus
	LastTestedAt   time.Time
	LastTestResult TestResult
	CreateTime     time.Time
	UpdateTime     time.Time
}

// Validate checks encrypted credential metadata without inspecting plaintext.
func (c Credential) Validate() error {
	if c.ID == uuid.Nil || c.ProviderID == uuid.Nil || strings.TrimSpace(c.Label) == "" ||
		len(c.Ciphertext) == 0 || strings.TrimSpace(c.KeyID) == "" ||
		!utf8.ValidString(c.Last4) || utf8.RuneCountInString(c.Last4) != 4 ||
		(c.Status != CredentialActive && c.Status != CredentialDisabled) ||
		(c.LastTestResult == "") != c.LastTestedAt.IsZero() ||
		(c.LastTestResult != "" && !validTestResult(c.LastTestResult)) {
		return ErrInvalidCredential
	}
	return nil
}

// RecordTest stores only the category and time of a free connectivity check.
func (c *Credential) RecordTest(result TestResult, testedAt time.Time) error {
	if c.Status != CredentialActive {
		return ErrCredentialDisabled
	}
	if !validTestResult(result) || testedAt.IsZero() {
		return ErrInvalidTestResult
	}
	c.LastTestResult = result
	c.LastTestedAt = testedAt.UTC()
	return nil
}

// Disable retains a credential for history while preventing new submission.
func (c *Credential) Disable() error {
	if err := c.Validate(); err != nil {
		return err
	}
	if c.Status == CredentialDisabled {
		return ErrCredentialDisabled
	}
	c.Status = CredentialDisabled
	return nil
}

func validTestResult(result TestResult) bool {
	switch result {
	case TestOK, TestAuthFailed, TestUnreachable, TestTimeout, TestUnsupported:
		return true
	default:
		return false
	}
}
