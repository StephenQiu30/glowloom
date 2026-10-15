package application

import (
	"context"
	"crypto/hmac"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// ErrInvalidEventFilter rejects unsupported or inconsistent event filters.
var ErrInvalidEventFilter = errors.New("invalid security event filter")

// EventFilter fixes the authority scope as well as the page selection.
type EventFilter struct {
	ScopeKind, Action, Cursor     string
	TeamID, TargetUserID, ActorID *uuid.UUID
	From, To                      *time.Time
	Limit                         int
}

// SecurityEvent contains no credentials or free-form persistence metadata.
type SecurityEvent struct {
	ID                                uuid.UUID
	ActorID, TargetUserID, TeamID     *uuid.UUID
	Action, Result, Reason, RequestID string
	SafeDiff                          map[string]string
	CreatedAt                         time.Time
}

// EventPage contains redacted events and an authenticated continuation cursor.
type EventPage struct {
	Items      []SecurityEvent
	NextCursor *string
	AsOf       time.Time
}

// EventPosition is the exclusive keyset boundary within a bounded snapshot.
type EventPosition struct {
	AsOf      time.Time `json:"as_of"`
	CreatedAt time.Time `json:"created_at"`
	ID        uuid.UUID `json:"id"`
}

// EventStore supplies authorized security event reads.
type EventStore interface {
	ReadEvents(context.Context, Principal, EventFilter, EventPosition, func() time.Time) ([]SecurityEvent, error)
}

// EventSigner authenticates continuation cursors without exposing key material.
type EventSigner interface {
	Digest(string, uuid.UUID, string) []byte
}

// EventQuery validates filters and binds event cursors to current authority.
type EventQuery struct {
	store  EventStore
	signer EventSigner
	now    func() time.Time
}

// NewEventQuery constructs EventQuery with explicit caller-supplied dependencies.
func NewEventQuery(store EventStore, signer EventSigner, now func() time.Time) *EventQuery {
	return &EventQuery{store: store, signer: signer, now: now}
}

func normalizeEventFilter(actor Principal, f EventFilter) (EventFilter, error) {
	if f.Limit == 0 {
		f.Limit = 20
	}
	if f.Limit < 1 || f.Limit > 100 || utf8.RuneCountInString(f.Action) > 100 || len(f.Cursor) > 4096 {
		return f, ErrInvalidEventFilter
	}
	for _, id := range []*uuid.UUID{f.TeamID, f.TargetUserID, f.ActorID} {
		if id != nil && *id == uuid.Nil {
			return f, ErrInvalidEventFilter
		}
	}
	if f.From != nil {
		v := f.From.UTC()
		f.From = &v
	}
	if f.To != nil {
		v := f.To.UTC()
		f.To = &v
	}
	if f.From != nil && f.To != nil && !f.From.Before(*f.To) {
		return f, ErrInvalidEventFilter
	}
	switch f.ScopeKind {
	case "self":
		if f.TeamID != nil || f.ActorID != nil || (f.TargetUserID != nil && *f.TargetUserID != actor.ID) {
			return f, ErrInvalidEventFilter
		}
		id := actor.ID
		f.TargetUserID = &id
	case "team":
		if f.TeamID == nil {
			return f, ErrInvalidEventFilter
		}
	case "platform":
	default:
		return f, ErrInvalidEventFilter
	}
	return f, nil
}

// cursorDigest binds a cursor to its actor, session, epochs and normalized filters.
func (q *EventQuery) cursorDigest(actor Principal, f EventFilter, payload string) []byte {
	f.Cursor = ""
	bound, _ := json.Marshal(struct {
		Session                        uuid.UUID
		CredentialEpoch, PlatformEpoch int64
		Filter                         EventFilter
		Payload                        string
	}{actor.SessionID, actor.CredentialEpoch, actor.PlatformEpoch, f, payload})
	return q.signer.Digest("security_event_cursor", actor.ID, string(bound))
}

// List validates current authority, filters and cursor before redaction.
func (q *EventQuery) List(ctx context.Context, actor Principal, filter EventFilter) (EventPage, error) {
	if actor.ID == uuid.Nil || actor.SessionID == uuid.Nil {
		return EventPage{}, ErrSessionInvalid
	}
	f, err := normalizeEventFilter(actor, filter)
	if err != nil || q.signer == nil {
		return EventPage{}, ErrInvalidEventFilter
	}
	now := q.now().UTC()
	position := EventPosition{AsOf: now}
	if f.Cursor != "" {
		parts := strings.Split(f.Cursor, ".")
		if len(parts) != 2 {
			return EventPage{}, ErrInvalidEventFilter
		}
		sig, err := base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil || !hmac.Equal(sig, q.cursorDigest(actor, f, parts[0])) {
			return EventPage{}, ErrInvalidEventFilter
		}
		raw, err := base64.RawURLEncoding.DecodeString(parts[0])
		if err != nil || json.Unmarshal(raw, &position) != nil || position.ID == uuid.Nil || position.AsOf.After(now) || now.Sub(position.AsOf) >= 10*time.Minute || position.CreatedAt.IsZero() || position.CreatedAt.After(position.AsOf) {
			return EventPage{}, ErrInvalidEventFilter
		}
	}
	items, err := q.store.ReadEvents(ctx, actor, f, position, q.now)
	if err != nil {
		return EventPage{}, err
	}
	page := EventPage{Items: make([]SecurityEvent, 0, len(items)), AsOf: position.AsOf}
	if len(items) > f.Limit {
		items = items[:f.Limit]
		last := items[len(items)-1]
		raw, err := json.Marshal(EventPosition{AsOf: position.AsOf, CreatedAt: last.CreatedAt, ID: last.ID})
		if err != nil {
			return EventPage{}, err
		}
		payload := base64.RawURLEncoding.EncodeToString(raw)
		cursor := payload + "." + base64.RawURLEncoding.EncodeToString(q.cursorDigest(actor, f, payload))
		page.NextCursor = &cursor
	}
	for _, e := range items {
		diff := make(map[string]string)
		for _, key := range []string{"status", "platform_role", "role", "governance_blocked"} {
			if value, ok := e.SafeDiff[key]; ok {
				diff[key] = value
			}
		}
		e.SafeDiff = diff
		switch f.ScopeKind {
		case "self":
			e.Reason = ""
			e.RequestID = ""
			if e.ActorID != nil && *e.ActorID != actor.ID {
				e.ActorID = nil
			}
		case "team":
			e.RequestID = ""
		}
		page.Items = append(page.Items, e)
	}
	return page, nil
}
