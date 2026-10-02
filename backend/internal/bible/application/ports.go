// Package application coordinates immutable Bible changes with current owning authorization.
package application

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/bible/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
)

var (
	// ErrUnavailable preserves a missing or unproved current owning dependency.
	ErrUnavailable = errors.New("bible context unavailable")
	// ErrNotFound hides absent and foreign identities.
	ErrNotFound = errors.New("bible identity not found")
	// ErrConflict refuses stale content or owner facts.
	ErrConflict = errors.New("bible revision conflict")
	// ErrIdempotencyConflict refuses a permanent key reused for another command.
	ErrIdempotencyConflict = errors.New("bible command key conflict")
)

// ProjectAccess retains the live workspace content lock in the caller's transaction.
type ProjectAccess interface {
	Authorize(context.Context, identityapp.Principal, uuid.UUID, bool) (workspaceapp.ProjectContentAccess, error)
	TouchContent(context.Context, identityapp.Principal, uuid.UUID, int64) (int64, error)
}

// MediaReferences proves current same-project readiness, moderation, consent and actual rendition content.
type MediaReferences interface {
	Reference(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID, string) (domain.MediaFact, error)
	Verify(context.Context, identityapp.Principal, uuid.UUID, domain.MediaFact) error
}

// VoiceSelection contains user choices, not supplier or credential execution facts.
type VoiceSelection struct {
	ModelKey             string             `json:"model_key"`
	ExpectedModelVersion int                `json:"expected_model_version"`
	VoiceKey             string             `json:"voice_key"`
	Params               domain.VoiceParams `json:"params"`
}

// CatalogVoices checks the real configured TTS model and voice schema under current authorization.
type CatalogVoices interface {
	Reference(context.Context, identityapp.Principal, uuid.UUID, VoiceSelection) (domain.CatalogVoice, error)
	List(context.Context, identityapp.Principal, uuid.UUID, int, *VoiceCursor) (VoicePage, error)
}

// ScriptScopes proves that appearance applicability refers to actual formal episode/scene identities.
type ScriptScopes interface {
	ValidateScopes(context.Context, identityapp.Principal, uuid.UUID, []domain.LookScope) error
}

// ImpactInput describes a real downstream-sensitive change, never a guessed zero count.
type ImpactInput struct {
	Kind      domain.Kind
	EntryID   uuid.UUID
	VersionID uuid.UUID
	Action    string
	LookID    *uuid.UUID
}

// ImpactProof contains the exact owning revisions accepted by an impact acknowledgment.
type ImpactProof struct {
	SHA256   string `json:"sha256"`
	Revision int64  `json:"revision"`
}

// Impacts reads and applies actual downstream facts within the content transaction.
type Impacts interface {
	Read(context.Context, identityapp.Principal, uuid.UUID, ImpactInput) (ImpactProof, error)
	Apply(context.Context, identityapp.Principal, uuid.UUID, ImpactInput, ImpactProof) error
}

// GeneratedResults returns only an actual authorized successful extraction result.
type GeneratedResults interface {
	Character(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID, uuid.UUID) (CharacterInput, domain.ResultSource, error)
	Location(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID, uuid.UUID) (domain.LocationContent, domain.ResultSource, error)
	Prop(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID, uuid.UUID) (domain.PropContent, domain.ResultSource, error)
}

// VoiceCursor identifies a real catalog page position.
type VoiceCursor struct {
	ModelKey string `json:"model_key"`
	VoiceKey string `json:"voice_key"`
}

// VoiceChoice presents a configured voice without pretending a sample was generated.
type VoiceChoice struct {
	ModelKey     string `json:"model_key"`
	ModelVersion int    `json:"model_version"`
	VoiceKey     string `json:"voice_key"`
	DisplayName  string `json:"display_name"`
}

// VoicePage is the bounded current configured catalog projection.
type VoicePage struct {
	Voices []VoiceChoice `json:"voices"`
	Next   *VoiceCursor  `json:"next,omitempty"`
}

// Receipt is a small permanent successful command result, not the full immutable content.
type Receipt struct {
	EntryID            uuid.UUID   `json:"entry_id"`
	Kind               domain.Kind `json:"kind"`
	Revision           int64       `json:"revision"`
	VersionID          uuid.UUID   `json:"version_id"`
	VersionNumber      int64       `json:"version_number"`
	ConfirmedVersionID *uuid.UUID  `json:"confirmed_version_id,omitempty"`
	ProjectRevision    int64       `json:"project_revision"`
	ContentSHA256      string      `json:"content_sha256"`
	CreatedEntryID     *uuid.UUID  `json:"created_entry_id,omitempty"`
	CreatedVersionID   *uuid.UUID  `json:"created_version_id,omitempty"`
	RedirectID         *uuid.UUID  `json:"redirect_id,omitempty"`
}

// Transaction exposes only the own records and exact transaction-bound owning ports.
type Transaction interface {
	Project() workspaceapp.ProjectContentAccess
	Access() ProjectAccess
	Media() MediaReferences
	Voices() CatalogVoices
	Scopes() ScriptScopes
	Impacts() Impacts
	Results() GeneratedResults
	Replay(context.Context, identityapp.Principal, Command, string) (*Receipt, error)
	Load(context.Context, domain.Kind, uuid.UUID) (domain.Head, domain.Version, error)
	LoadVersion(context.Context, domain.Kind, uuid.UUID) (domain.Version, error)
	InsertHead(context.Context, domain.Head) error
	InsertVersion(context.Context, domain.Version) error
	UpdateHead(context.Context, domain.Head, int64) error
	Confirm(context.Context, domain.Kind, domain.Confirmation) error
	Redirect(context.Context, domain.Redirect) error
	Split(context.Context, domain.Split) error
	Record(context.Context, identityapp.Principal, Command, string, Receipt, time.Time) error
}

// Store owns the transaction and current project/actor authorization before every replay or write.
type Store interface {
	Write(context.Context, identityapp.Principal, uuid.UUID, func(Transaction) (Receipt, error)) (Receipt, error)
	Find(context.Context, identityapp.Principal, uuid.UUID, domain.Kind, uuid.UUID) (Detail, error)
	Version(context.Context, identityapp.Principal, uuid.UUID, domain.Kind, uuid.UUID, uuid.UUID) (domain.Version, error)
	List(context.Context, identityapp.Principal, ListInput) (Page, error)
	History(context.Context, identityapp.Principal, uuid.UUID, domain.Kind, uuid.UUID, int, *HistoryCursor) (HistoryPage, error)
	Voices(context.Context, identityapp.Principal, uuid.UUID, int, *VoiceCursor) (VoicePage, error)
}

// Detail includes stable pointers and the requested identity's complete current content.
type Detail struct {
	Head       domain.Head    `json:"head"`
	Current    domain.Version `json:"current"`
	ResolvedID uuid.UUID      `json:"resolved_id"`
}

// ListCursor is a stable entry page position.
type ListCursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        uuid.UUID `json:"id"`
}

// ListInput is a project-scoped bounded query.
type ListInput struct {
	ProjectID uuid.UUID
	Kind      domain.Kind
	Limit     int
	After     *ListCursor
}

// Summary contains only safe current list fields and actual version pointers.
type Summary struct {
	Head          domain.Head `json:"head"`
	Name          string      `json:"name"`
	ContentSHA256 string      `json:"content_sha256"`
}

// Page provides current scope for safe permanent intent recovery.
type Page struct {
	Entries        []Summary   `json:"entries"`
	Next           *ListCursor `json:"next,omitempty"`
	CurrentActorID uuid.UUID   `json:"current_actor_id"`
	CurrentOrgID   uuid.UUID   `json:"current_org_id"`
}

// HistoryCursor identifies an immutable history page position.
type HistoryCursor struct {
	Number int64     `json:"number"`
	ID     uuid.UUID `json:"id"`
}

// HistoryPage bounds immutable versions without collapsing complete historical content.
type HistoryPage struct {
	Versions []domain.Version `json:"versions"`
	Next     *HistoryCursor   `json:"next,omitempty"`
}
