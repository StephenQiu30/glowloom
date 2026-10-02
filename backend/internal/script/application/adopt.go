package application

import (
	"context"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
)

// AdoptCommand binds the selected formal version to both current heads and explicit impact intent.
type AdoptCommand struct {
	ProjectID             uuid.UUID `json:"project_id"`
	VersionID             uuid.UUID `json:"version_id"`
	Key                   uuid.UUID `json:"key"`
	RequestID             uuid.UUID `json:"request_id"`
	ExpectedRevision      int64     `json:"expected_revision"`
	ExpectedSplitRevision int64     `json:"expected_split_revision"`
	AckInvalidate         bool      `json:"ack_invalidate"`
}

// AdoptReceipt preserves original publication facts and explicit inheritance decisions.
type AdoptReceipt struct {
	ScriptRevision    int64            `json:"script_revision"`
	ProjectRevision   int64            `json:"project_revision"`
	VersionID         uuid.UUID        `json:"version_id"`
	PreviousVersionID *uuid.UUID       `json:"previous_version_id,omitempty"`
	SplitSetID        uuid.UUID        `json:"split_set_id"`
	Mappings          []EpisodeMapping `json:"episode_mappings"`
	Changed           bool             `json:"changed"`
	Duplicate         bool             `json:"duplicate"`
}

// AdoptStore owns publication, current authority, CAS and permanent command replay.
type AdoptStore interface {
	Adopt(context.Context, identityapp.Principal, AdoptCommand, time.Time) (AdoptReceipt, error)
}

// AdoptService admits confirmed content without pretending missing downstream owners are empty.
type AdoptService struct {
	store AdoptStore
	now   func() time.Time
}

// NewAdoptService injects the owning persistence and the current clock.
func NewAdoptService(store AdoptStore, now func() time.Time) *AdoptService {
	return &AdoptService{store: store, now: now}
}

// Adopt preserves old adopted/episode/structure facts when necessary evidence is missing.
func (s *AdoptService) Adopt(ctx context.Context, actor identityapp.Principal, input AdoptCommand) (AdoptReceipt, error) {
	if s == nil || s.store == nil || s.now == nil {
		return AdoptReceipt{}, ErrUnavailable
	}
	if input.ProjectID == uuid.Nil || input.VersionID == uuid.Nil || input.Key == uuid.Nil || input.RequestID == uuid.Nil || input.ExpectedRevision < 1 || input.ExpectedSplitRevision < 1 {
		return AdoptReceipt{}, domain.ErrInvalidSource
	}
	return s.store.Adopt(ctx, actor, input, s.now().UTC().Truncate(time.Microsecond))
}
