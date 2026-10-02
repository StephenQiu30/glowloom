package application

import (
	"context"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
)

// VersionText is a bounded canonical slice available before formal episode identities exist.
type VersionText struct {
	VersionID   uuid.UUID `json:"script_version_id"`
	Start       int       `json:"span_start"`
	End         int       `json:"span_end"`
	ContentHash string    `json:"content_hash"`
	Text        string    `json:"text"`
}

// ConfirmationSummary pages immutable complete partition decisions independently of heads.
type ConfirmationSummary struct {
	ID             uuid.UUID `json:"id"`
	Revision       int64     `json:"revision"`
	CandidateSetID uuid.UUID `json:"candidate_set_id"`
	FormalSetID    uuid.UUID `json:"formal_set_id"`
	EpisodeCount   int       `json:"episode_count"`
	CreatedAt      time.Time `json:"created_at"`
}

// ConfirmationPage keeps historical reads bounded without discarding the full snapshot.
type ConfirmationPage struct {
	Items        []ConfirmationSummary `json:"items"`
	NextRevision *int64                `json:"next_revision,omitempty"`
}

// VersionText proves the immutable object's SHA before returning a Unicode-scalar slice.
func (s *HistoryService) VersionText(ctx context.Context, actor identityapp.Principal, project, version uuid.UUID, from, to int) (VersionText, error) {
	if s == nil || s.sources == nil {
		return VersionText{}, ErrUnavailable
	}
	if project == uuid.Nil || version == uuid.Nil || from < 0 || to <= from || to-from > 65536 {
		return VersionText{}, domain.ErrInvalidSpan
	}
	text, err := s.sources.VersionText(ctx, actor, project, version)
	if err != nil {
		return VersionText{}, err
	}
	part, err := domain.ScalarSlice(text, from, to)
	if err != nil {
		return VersionText{}, err
	}
	return VersionText{VersionID: version, Start: from, End: to, ContentHash: domain.ContentSHA([]byte(text)), Text: part}, nil
}

// Confirmations pages every past partition result rather than reconstructing current heads.
func (s *HistoryService) Confirmations(ctx context.Context, actor identityapp.Principal, project, version uuid.UUID, before int64, limit int) (ConfirmationPage, error) {
	if s == nil || s.store == nil {
		return ConfirmationPage{}, ErrUnavailable
	}
	if project == uuid.Nil || version == uuid.Nil || before < 0 || limit < 1 || limit > 100 {
		return ConfirmationPage{}, domain.ErrInvalidSpan
	}
	return s.store.Confirmations(ctx, actor, project, version, before, limit)
}

// Confirmation returns only the exact immutable version/confirmation relationship.
func (s *HistoryService) Confirmation(ctx context.Context, actor identityapp.Principal, project, version, id uuid.UUID) (domain.SplitConfirmation, error) {
	if s == nil || s.store == nil {
		return domain.SplitConfirmation{}, ErrUnavailable
	}
	if project == uuid.Nil || version == uuid.Nil || id == uuid.Nil {
		return domain.SplitConfirmation{}, domain.ErrInvalidSpan
	}
	return s.store.Confirmation(ctx, actor, project, version, id)
}
