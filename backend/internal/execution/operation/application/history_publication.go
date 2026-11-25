package application

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// ErrPublicationConflict means an idempotency key was reused for different public facts.
var ErrPublicationConflict = errors.New("publication idempotency conflict")

// HistoryPublicationTarget identifies one actual candidate, never an entire project.
type HistoryPublicationTarget struct {
	OperationID uuid.UUID `json:"operation_id"`
	OutputID    uuid.UUID `json:"output_id"`
}

// HistoryPublicationInput explicitly chooses the facts made public.
type HistoryPublicationInput struct {
	Targets       []HistoryPublicationTarget `json:"targets"`
	Title         string                     `json:"title"`
	Description   string                     `json:"description"`
	PublishPrompt bool                       `json:"publish_prompt"`
}

// HistoryPublicationResult preserves per-candidate success in a partial batch.
type HistoryPublicationResult struct {
	OperationID uuid.UUID  `json:"operation_id"`
	OutputID    uuid.UUID  `json:"output_id"`
	Status      string     `json:"status"`
	WorkID      *uuid.UUID `json:"work_id,omitempty"`
	URL         string     `json:"url,omitempty"`
	ErrorCode   string     `json:"error_code,omitempty"`
}

// HistoryPublicationReceipt is safe to persist as an idempotency result.
type HistoryPublicationReceipt struct {
	Items []HistoryPublicationResult `json:"items"`
}

// HistoryPublicationStore retains authorization, candidate locks and the receipt together.
type HistoryPublicationStore interface {
	PublishHistory(context.Context, identity.Principal, uuid.UUID, HistoryPublicationInput) (HistoryPublicationReceipt, error)
}

// HistoryPublisher validates publication independently of HTTP or storage.
type HistoryPublisher struct{ store HistoryPublicationStore }

// NewHistoryPublisher injects the authorized publication writer.
func NewHistoryPublisher(store HistoryPublicationStore) *HistoryPublisher {
	return &HistoryPublisher{store: store}
}

// Publish validates a deliberate, bounded public selection.
func (p *HistoryPublisher) Publish(ctx context.Context, actor identity.Principal, key uuid.UUID, in HistoryPublicationInput) (HistoryPublicationReceipt, error) {
	if actor.ID == uuid.Nil || actor.OrgID == uuid.Nil || actor.ScopeID == uuid.Nil || actor.SessionID == uuid.Nil || actor.CredentialEpoch < 1 {
		return HistoryPublicationReceipt{}, identity.ErrForbidden
	}
	in.Title = strings.TrimSpace(in.Title)
	in.Description = strings.TrimSpace(in.Description)
	if strings.ContainsRune(in.Description, 0) || key == uuid.Nil || len(in.Targets) == 0 || len(in.Targets) > 16 || !utf8.ValidString(in.Title) || !utf8.ValidString(in.Description) || utf8.RuneCountInString(in.Title) < 1 || utf8.RuneCountInString(in.Title) > 120 || utf8.RuneCountInString(in.Description) > 2000 {
		return HistoryPublicationReceipt{}, ErrInvalidPublicQuery
	}
	for _, r := range in.Title {
		if unicode.IsControl(r) {
			return HistoryPublicationReceipt{}, ErrInvalidPublicQuery
		}
	}
	seen := make(map[uuid.UUID]bool, len(in.Targets))
	for _, target := range in.Targets {
		if target.OperationID == uuid.Nil || target.OutputID == uuid.Nil || seen[target.OutputID] {
			return HistoryPublicationReceipt{}, ErrInvalidPublicQuery
		}
		seen[target.OutputID] = true
	}
	if p == nil || p.store == nil {
		return HistoryPublicationReceipt{}, ErrInvalidPublicQuery
	}
	return p.store.PublishHistory(ctx, actor, key, in)
}

// PublishedWork contains only a deliberately public projection.
type PublishedWork struct {
	ID             uuid.UUID `json:"id"`
	Title          string    `json:"title"`
	Description    string    `json:"description"`
	Kind           string    `json:"kind"`
	Prompt         *string   `json:"prompt,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	MediaURL       string    `json:"media_url"`
	MediaExpiresAt time.Time `json:"media_expires_at"`
}

// PublishedWorkFile is an internal media identity; never serialize it as an API response.
type PublishedWorkFile struct {
	Work       PublishedWork
	ProfileID  string
	Bucket     string
	ObjectKey  string
	LocationID uuid.UUID
	AssetID    uuid.UUID
	Revision   int64
}

// PublishedWorkStore rechecks current deletion and moderation before issuing a lease.
type PublishedWorkStore interface {
	FindPublishedWork(context.Context, uuid.UUID) (PublishedWorkFile, error)
}

// PublicationMedia signs exactly the persisted physical object selected by the store.
type PublicationMedia interface {
	SignPublishedMedia(context.Context, PublishedWorkFile) (string, time.Time, error)
}

// PublishedWorkQuery resolves only currently eligible public works.
type PublishedWorkQuery struct {
	store PublishedWorkStore
	media PublicationMedia
}

// NewPublishedWorkQuery injects persisted eligibility and physical signing.
func NewPublishedWorkQuery(store PublishedWorkStore, media PublicationMedia) *PublishedWorkQuery {
	return &PublishedWorkQuery{store: store, media: media}
}

// Get rechecks live eligibility after issuing the bounded media lease.
func (q *PublishedWorkQuery) Get(ctx context.Context, id uuid.UUID) (PublishedWork, error) {
	if id == uuid.Nil {
		return PublishedWork{}, ErrPublicNotFound
	}
	if q == nil || q.store == nil || q.media == nil {
		return PublishedWork{}, ErrInvalidPublicQuery
	}
	file, err := q.store.FindPublishedWork(ctx, id)
	if err != nil {
		return PublishedWork{}, err
	}
	url, expiry, err := q.media.SignPublishedMedia(ctx, file)
	if err != nil {
		return PublishedWork{}, err
	}
	current, err := q.store.FindPublishedWork(ctx, id)
	if err != nil {
		return PublishedWork{}, err
	}
	if url == "" || current.AssetID != file.AssetID || current.Revision != file.Revision || current.LocationID != file.LocationID || current.ObjectKey != file.ObjectKey || current.ProfileID != file.ProfileID || current.Bucket != file.Bucket || !expiry.After(time.Now()) {
		return PublishedWork{}, ErrPublicNotFound
	}
	current.Work.MediaURL = url
	current.Work.MediaExpiresAt = expiry
	return current.Work, nil
}
