// Package application coordinates space-owned subjects and their frozen media bindings.
package application

import (
	"context"
	"time"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/subject/domain"
)

// ScopeFacts separates the logical owner from its physical organization.
type ScopeFacts struct {
	ScopeID         uuid.UUID
	PhysicalScopeID uuid.UUID
}

// ScopeAccess retains current request authority in the caller's transaction.
type ScopeAccess interface {
	Resolve(context.Context, identity.Principal, bool) (ScopeFacts, error)
}

// MediaSelection names a source; all other facts are supplied by its owner.
type MediaSelection struct {
	BindingID *uuid.UUID `json:"binding_id,omitempty"`
	AssetID   uuid.UUID  `json:"asset_id"`
	ProjectID *uuid.UUID `json:"project_id,omitempty"`
}

// MediaAccess proves source ownership, readiness and actual frozen media facts.
type MediaAccess interface {
	Reference(context.Context, identity.Principal, MediaSelection) (domain.MediaFact, error)
}

// VoiceSelection names an uploaded audio sample, without pretending TTS was run.
type VoiceSelection struct {
	BindingID *uuid.UUID `json:"binding_id,omitempty"`
	Name      string     `json:"name"`
	AssetID   uuid.UUID  `json:"asset_id"`
	ProjectID *uuid.UUID `json:"project_id,omitempty"`
}

// SaveInput atomically replaces the complete subject settings.
type SaveInput struct {
	ExpectedRevision int64            `json:"expected_revision"`
	Name             string           `json:"name"`
	Description      string           `json:"description"`
	Primary          MediaSelection   `json:"primary"`
	References       []MediaSelection `json:"references"`
	Voice            *VoiceSelection  `json:"voice,omitempty"`
}

// Media is the safe public representation of a frozen binding.
type Media struct {
	BindingID     uuid.UUID `json:"binding_id"`
	AssetID       uuid.UUID `json:"asset_id"`
	AssetRevision int64     `json:"asset_revision"`
	Kind          string    `json:"kind"`
	MIMEType      string    `json:"mime_type"`
	FileName      string    `json:"file_name"`
	DurationMS    *int64    `json:"duration_ms,omitempty"`
	Width         *int32    `json:"width,omitempty"`
	Height        *int32    `json:"height,omitempty"`
}

// Voice exposes one optional audio binding.
type Voice struct {
	Name  string `json:"name"`
	Media Media  `json:"media"`
}

// Detail contains everything needed to render a card or reopen its settings.
type Detail struct {
	ID           uuid.UUID `json:"id"`
	OwnerScopeID uuid.UUID `json:"owner_scope_id"`
	Revision     int64     `json:"revision"`
	VersionID    uuid.UUID `json:"version_id"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	Primary      Media     `json:"primary"`
	References   []Media   `json:"references"`
	Voice        *Voice    `json:"voice,omitempty"`
	Favorite     bool      `json:"favorite"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Capability reports an actual execution capability separately from UI bindings.
type Capability struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

// Capabilities never advertises an unimplemented intelligent voice generator.
type Capabilities struct {
	IntelligentVoice Capability `json:"intelligent_voice"`
}

// Page is an offset-paged, fully filtered current-space result.
type Page struct {
	CurrentActorID uuid.UUID    `json:"current_actor_id"`
	CurrentScopeID uuid.UUID    `json:"current_scope_id"`
	Items          []Detail     `json:"items"`
	Page           int          `json:"page"`
	PageSize       int          `json:"page_size"`
	Total          int64        `json:"total"`
	Capabilities   Capabilities `json:"capabilities"`
}

// ListInput filters before stable ordering and pagination.
type ListInput struct {
	Query     string
	Favorite  bool
	From      *time.Time
	To        *time.Time
	TimeField string
	Order     string
	Page      int
	PageSize  int
}

// RevisionInput binds a content mutation to its observed shared revision.
type RevisionInput struct {
	ExpectedRevision int64 `json:"expected_revision"`
}

// FavoriteInput keeps personal preference CAS separate from content revision.
type FavoriteInput struct {
	ExpectedRevision int64 `json:"expected_revision"`
	Favorite         bool  `json:"favorite"`
}

// BatchItem carries one explicitly selected identity and observed revision.
type BatchItem struct {
	ID               uuid.UUID `json:"id"`
	ExpectedRevision int64     `json:"expected_revision"`
}

// BatchInput requests one bounded action with explicit per-item outcomes.
type BatchInput struct {
	Action string      `json:"action"`
	Items  []BatchItem `json:"items"`
}

// ItemResult reports each action independently; failed items are never implicit successes.
type ItemResult struct {
	ID       uuid.UUID `json:"id"`
	Status   string    `json:"status"`
	Code     string    `json:"code,omitempty"`
	Revision int64     `json:"revision,omitempty"`
	Favorite *bool     `json:"favorite,omitempty"`
}

// BatchResult preserves outcomes under a permanent command key.
type BatchResult struct {
	Items []ItemResult `json:"items"`
}

// Recovery reports only committed results. Unrecorded never proves dispatch stopped.
type Recovery struct {
	State  string         `json:"state"`
	Result *CommandResult `json:"result,omitempty"`
}
