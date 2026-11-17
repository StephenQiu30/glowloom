package application

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	// ErrInvalidNotification rejects invalid content, filters or command identities.
	ErrInvalidNotification = errors.New("invalid notification")
	// ErrNotificationConflict rejects stale revisions, mailbox generations or claimed popups.
	ErrNotificationConflict = errors.New("notification revision conflict")
	// ErrNotificationNotFound hides absent and foreign-account records.
	ErrNotificationNotFound = errors.New("notification not found")
)

// AnnouncementContent is safe text and a controlled, same-site destination.
type AnnouncementContent struct {
	Title           string     `json:"title"`
	Body            string     `json:"body"`
	Placement       string     `json:"placement"`
	Audience        string     `json:"audience"`
	TeamID          *uuid.UUID `json:"team_id"`
	StartsAt        time.Time  `json:"starts_at"`
	EndsAt          time.Time  `json:"ends_at"`
	Endless         bool       `json:"endless"`
	ManualClose     bool       `json:"manual_close"`
	Frequency       string     `json:"frequency" enums:"once,daily,login" default:"once"`
	Priority        int        `json:"priority"`
	DurationSeconds int        `json:"duration_seconds"`
	ActionPath      string     `json:"action_path"`
	Inbox           bool       `json:"inbox"`
}

// Announcement combines an editable draft and its current immutable publication.
type Announcement struct {
	ID                uuid.UUID            `json:"id"`
	Version           int64                `json:"version,string" swaggertype:"string"`
	Draft             AnnouncementContent  `json:"draft"`
	Published         *AnnouncementContent `json:"published"`
	PublishedRevision *string              `json:"published_revision"`
	Enabled           bool                 `json:"enabled"`
	Status            string               `json:"status"`
	CreatedAt         time.Time            `json:"created_at"`
	UpdatedAt         time.Time            `json:"updated_at"`
}

// SaveAnnouncementInput binds an editable draft to a stable command and expected version.
type SaveAnnouncementInput struct {
	CommandID       uuid.UUID           `json:"command_id"`
	ExpectedVersion int64               `json:"expected_version,string" swaggertype:"string"`
	Content         AnnouncementContent `json:"content"`
}

// AnnouncementCommand identifies a publication, enable-state or draft deletion command.
type AnnouncementCommand struct {
	CommandID       uuid.UUID `json:"command_id"`
	ExpectedVersion int64     `json:"expected_version,string" swaggertype:"string"`
	Enabled         bool      `json:"enabled"`
}

// NotificationFilter contains the supported server-side paging and filtering conditions.
type NotificationFilter struct {
	Cursor     string     `json:"-"`
	Limit      int        `json:"limit"`
	Search     string     `json:"search"`
	Placement  string     `json:"placement"`
	Audience   string     `json:"audience"`
	Status     string     `json:"status"`
	Category   string     `json:"category"`
	UnreadOnly bool       `json:"unread_only"`
	From       *time.Time `json:"from"`
	To         *time.Time `json:"to"`
}

// AnnouncementPage contains a bounded management page without an invented total.
type AnnouncementPage struct {
	Items      []Announcement `json:"items"`
	NextCursor *string        `json:"next_cursor"`
}

// NotificationSummary is the authoritative account mailbox watermark and unread count.
type NotificationSummary struct {
	DataEpoch      uuid.UUID `json:"data_epoch"`
	MailboxVersion int64     `json:"mailbox_version,string" swaggertype:"string"`
	MaxInboxSeq    int64     `json:"max_inbox_seq,string" swaggertype:"string"`
	UnreadCount    int64     `json:"unread_count"`
}

// InboxNotification is an account-owned immutable content snapshot and current read state.
type InboxNotification struct {
	ID             uuid.UUID  `json:"id"`
	InboxSeq       int64      `json:"inbox_seq,string" swaggertype:"string"`
	AnnouncementID *uuid.UUID `json:"announcement_id"`
	Revision       *int64     `json:"revision,string" swaggertype:"string"`
	SourceKind     string     `json:"source_kind"`
	SourceID       *uuid.UUID `json:"source_id"`
	Category       string     `json:"category"`
	Title          string     `json:"title"`
	Body           string     `json:"body"`
	ActionPath     string     `json:"action_path"`
	Read           bool       `json:"read"`
	OccurredAt     time.Time  `json:"occurred_at"`
	Status         string     `json:"status"`
}

// NotificationPage combines inbox items with the current mailbox summary.
type NotificationPage struct {
	NotificationSummary
	Items      []InboxNotification `json:"items"`
	NextCursor *string             `json:"next_cursor"`
}

// ReadNotificationInput identifies a stable single-notification read command.
type ReadNotificationInput struct {
	CommandID uuid.UUID `json:"command_id"`
}

// ReadAllNotificationsInput binds a read command to an account mailbox generation and watermark.
type ReadAllNotificationsInput struct {
	CommandID  uuid.UUID `json:"command_id"`
	DataEpoch  uuid.UUID `json:"data_epoch"`
	ThroughSeq int64     `json:"through_seq,string" swaggertype:"string"`
}

// DisplayAnnouncement contains the active, audience-authorized display projection.
type DisplayAnnouncement struct {
	ID              uuid.UUID  `json:"id"`
	Revision        int64      `json:"revision,string" swaggertype:"string"`
	NotificationID  *uuid.UUID `json:"notification_id"`
	Title           string     `json:"title"`
	Body            string     `json:"body"`
	Placement       string     `json:"placement"`
	ActionPath      string     `json:"action_path"`
	DurationSeconds int        `json:"duration_seconds"`
	StartsAt        time.Time  `json:"starts_at"`
	EndsAt          time.Time  `json:"ends_at"`
	Endless         bool       `json:"endless"`
	ManualClose     bool       `json:"manual_close"`
	Frequency       string     `json:"frequency" enums:"once,daily,login" default:"once"`
}

// NotificationDisplayView contains at most one banner and one popup candidate.
type NotificationDisplayView struct {
	Banner *DisplayAnnouncement `json:"banner"`
	Popup  *DisplayAnnouncement `json:"popup"`
}

// NotificationDisplayValidity rechecks current publication independently of read
// state and any previously claimed display receipt.
type NotificationDisplayValidity struct {
	Valid bool `json:"valid" binding:"required"`
}

// NotificationDisplayAck records display separately from mailbox read state.
type NotificationDisplayAck struct {
	AnnouncementID uuid.UUID `json:"announcement_id"`
	Revision       int64     `json:"revision,string" swaggertype:"string"`
	Kind           string    `json:"kind"`
}

// NotificationAudit records an authorized management mutation.
type NotificationAudit struct {
	ID        uuid.UUID `json:"id"`
	ActorID   uuid.UUID `json:"actor_id"`
	Action    string    `json:"action"`
	Version   int64     `json:"version,string" swaggertype:"string"`
	CreatedAt time.Time `json:"created_at"`
}

// NotificationAuditPage contains the most recent management records.
type NotificationAuditPage struct {
	Items []NotificationAudit `json:"items"`
}

// TaskNotificationInput is the source-authorized safe terminal event projection.
// RecipientID must come from Execution's persisted confirmation owner.
type TaskNotificationInput struct {
	EventID     uuid.UUID
	RecipientID uuid.UUID
	OperationID uuid.UUID
	Status      string
	OccurredAt  time.Time
}

// NotificationStore owns current authority and atomic persistence boundaries.
type NotificationStore interface {
	SaveAnnouncement(context.Context, Principal, uuid.UUID, SaveAnnouncementInput, func() time.Time) (Announcement, error)
	CommandAnnouncement(context.Context, Principal, uuid.UUID, string, AnnouncementCommand, func() time.Time) (Announcement, error)
	GetAnnouncement(context.Context, Principal, uuid.UUID, func() time.Time) (Announcement, error)
	ListAnnouncements(context.Context, Principal, NotificationFilter, func() time.Time) (AnnouncementPage, error)
	AnnouncementAudit(context.Context, Principal, uuid.UUID, func() time.Time) (NotificationAuditPage, error)
	Summary(context.Context, Principal, func() time.Time) (NotificationSummary, error)
	ListNotifications(context.Context, Principal, NotificationFilter, func() time.Time) (NotificationPage, error)
	GetNotification(context.Context, Principal, uuid.UUID, func() time.Time) (InboxNotification, error)
	ReadNotification(context.Context, Principal, uuid.UUID, ReadAllNotificationsInput, func() time.Time) (NotificationSummary, error)
	Display(context.Context, *Principal, func() time.Time) (NotificationDisplayView, error)
	ValidateDisplay(context.Context, Principal, uuid.UUID, int64, func() time.Time) (NotificationDisplayValidity, error)
	AckDisplay(context.Context, Principal, NotificationDisplayAck, func() time.Time) error
	ProjectTaskNotification(context.Context, TaskNotificationInput) error
}

// Notifications validates notification use cases before the transactional persistence boundary.
type Notifications struct {
	store NotificationStore
	clock func() time.Time
}

// NewNotifications constructs notification use cases with explicit persistence and clock dependencies.
func NewNotifications(store NotificationStore, clock func() time.Time) *Notifications {
	return &Notifications{store: store, clock: clock}
}

func validContent(c *AnnouncementContent) bool {
	c.Title = strings.TrimSpace(c.Title)
	c.Body = strings.TrimSpace(c.Body)
	if c.Frequency == "" {
		c.Frequency = "once"
	}
	if c.Endless {
		c.EndsAt = time.Time{}
	}
	if c.DurationSeconds == 0 {
		c.DurationSeconds = 5
	}
	if c.Title == "" || len([]rune(c.Title)) > 120 || c.Body == "" || len([]rune(c.Body)) > 10000 || c.StartsAt.IsZero() || (!c.Endless && !c.EndsAt.After(c.StartsAt)) || c.Priority < 0 || c.Priority > 100 || c.DurationSeconds < 5 || c.DurationSeconds > 60 {
		return false
	}
	if c.Frequency != "once" && c.Frequency != "daily" && c.Frequency != "login" {
		return false
	}
	if c.Placement != "banner" && c.Placement != "modal" && c.Placement != "inbox" {
		return false
	}
	if c.Audience != "public" && c.Audience != "authenticated" && c.Audience != "team" {
		return false
	}
	if (c.Audience == "team" && (c.TeamID == nil || *c.TeamID == uuid.Nil)) || (c.Audience != "team" && c.TeamID != nil) {
		return false
	}
	if c.Audience == "public" && c.Placement == "modal" {
		return false
	}
	if c.ActionPath == "" {
		return true
	}
	u, err := url.Parse(c.ActionPath)
	if err != nil || u.IsAbs() || u.Host != "" || strings.ContainsAny(c.ActionPath, "\\\r\n") {
		return false
	}
	for _, p := range []string{"/assets", "/tasks", "/teams", "/notifications", "/billing", "/account", "/short-drama", "/ai-tools"} {
		if u.Path == p || strings.HasPrefix(u.Path, p+"/") {
			return true
		}
	}
	return false
}

// Save validates content before saving a draft with compare-and-swap semantics.
func (s *Notifications) Save(ctx context.Context, a Principal, id uuid.UUID, in SaveAnnouncementInput) (Announcement, error) {
	if in.CommandID == uuid.Nil || (id == uuid.Nil && in.ExpectedVersion != 0) || (id != uuid.Nil && in.ExpectedVersion < 1) || !validContent(&in.Content) {
		return Announcement{}, ErrInvalidNotification
	}
	return s.store.SaveAnnouncement(ctx, a, id, in, s.clock)
}

// Command validates publication, enable-state and draft deletion command identities.
func (s *Notifications) Command(ctx context.Context, a Principal, id uuid.UUID, action string, in AnnouncementCommand) (Announcement, error) {
	if id == uuid.Nil || in.CommandID == uuid.Nil || in.ExpectedVersion < 1 || (action != "publish" && action != "enabled" && action != "delete") {
		return Announcement{}, ErrInvalidNotification
	}
	return s.store.CommandAnnouncement(ctx, a, id, action, in, s.clock)
}

// Get reads one management-authorized announcement.
func (s *Notifications) Get(ctx context.Context, a Principal, id uuid.UUID) (Announcement, error) {
	return s.store.GetAnnouncement(ctx, a, id, s.clock)
}
func normalizeNotificationFilter(f *NotificationFilter, admin bool) error {
	if f.Limit == 0 {
		f.Limit = 20
	}
	if f.Limit < 1 || f.Limit > 100 || len(f.Search) > 200 || (f.From != nil && f.To != nil && !f.To.After(*f.From)) {
		return ErrInvalidNotification
	}
	if admin {
		if f.Placement != "" && f.Placement != "banner" && f.Placement != "modal" && f.Placement != "inbox" {
			return ErrInvalidNotification
		}
		if f.Audience != "" && f.Audience != "public" && f.Audience != "authenticated" && f.Audience != "team" {
			return ErrInvalidNotification
		}
		if f.Status != "" && f.Status != "draft" && f.Status != "scheduled" && f.Status != "active" && f.Status != "expired" && f.Status != "disabled" {
			return ErrInvalidNotification
		}
	} else if f.Category != "" && f.Category != "system" && f.Category != "task" && f.Category != "collaboration" && f.Category != "account" && f.Category != "interaction" && f.Category != "official" {
		return ErrInvalidNotification
	}
	return nil
}

// List validates and reads the management query.
func (s *Notifications) List(ctx context.Context, a Principal, f NotificationFilter) (AnnouncementPage, error) {
	if err := normalizeNotificationFilter(&f, true); err != nil {
		return AnnouncementPage{}, err
	}
	return s.store.ListAnnouncements(ctx, a, f, s.clock)
}

// Audit reads the latest management audit for an announcement.
func (s *Notifications) Audit(ctx context.Context, a Principal, id uuid.UUID) (NotificationAuditPage, error) {
	return s.store.AnnouncementAudit(ctx, a, id, s.clock)
}

// Summary reads the current account mailbox authority.
func (s *Notifications) Summary(ctx context.Context, a Principal) (NotificationSummary, error) {
	return s.store.Summary(ctx, a, s.clock)
}

// Inbox validates and reads the account-owned inbox query.
func (s *Notifications) Inbox(ctx context.Context, a Principal, f NotificationFilter) (NotificationPage, error) {
	if err := normalizeNotificationFilter(&f, false); err != nil {
		return NotificationPage{}, err
	}
	return s.store.ListNotifications(ctx, a, f, s.clock)
}

// Detail reads one account-owned content snapshot without marking it read.
func (s *Notifications) Detail(ctx context.Context, a Principal, id uuid.UUID) (InboxNotification, error) {
	return s.store.GetNotification(ctx, a, id, s.clock)
}

// Read marks a single item or a bounded mailbox watermark read.
func (s *Notifications) Read(ctx context.Context, a Principal, id uuid.UUID, in ReadAllNotificationsInput) (NotificationSummary, error) {
	if in.CommandID == uuid.Nil || (id == uuid.Nil && (in.DataEpoch == uuid.Nil || in.ThroughSeq < 0)) {
		return NotificationSummary{}, ErrInvalidNotification
	}
	return s.store.ReadNotification(ctx, a, id, in, s.clock)
}

// Display reads current display candidates for a visitor or account.
func (s *Notifications) Display(ctx context.Context, a *Principal) (NotificationDisplayView, error) {
	return s.store.Display(ctx, a, s.clock)
}

// ValidateDisplay rechecks a previously claimed candidate before presentation.
func (s *Notifications) ValidateDisplay(ctx context.Context, a Principal, id uuid.UUID, revision int64) (NotificationDisplayValidity, error) {
	if id == uuid.Nil || revision < 1 {
		return NotificationDisplayValidity{}, ErrInvalidNotification
	}
	return s.store.ValidateDisplay(ctx, a, id, revision, s.clock)
}

// Ack validates an independent display receipt.
func (s *Notifications) Ack(ctx context.Context, a Principal, in NotificationDisplayAck) error {
	if in.AnnouncementID == uuid.Nil || in.Revision < 1 || (in.Kind != "shown" && in.Kind != "dismissed") {
		return ErrInvalidNotification
	}
	return s.store.AckDisplay(ctx, a, in, s.clock)
}

// ProjectTask validates a trusted source event; there is no public recipient API.
func (s *Notifications) ProjectTask(ctx context.Context, in TaskNotificationInput) error {
	if in.EventID == uuid.Nil || in.RecipientID == uuid.Nil || in.OperationID == uuid.Nil || in.OccurredAt.IsZero() || (in.Status != "completed" && in.Status != "failed") {
		return ErrInvalidNotification
	}
	return s.store.ProjectTaskNotification(ctx, in)
}
