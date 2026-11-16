package postgres

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	app "github.com/StephenQiu30/glowloom-server/backend/identity/application"
)

// NotificationStore serializes mailbox writes and commits publication, projection,
// command receipt and audit together. No process-local state is authoritative.
type NotificationStore struct{ db *gorm.DB }

// NewNotificationStore constructs the PostgreSQL notification authority.
func NewNotificationStore(db *gorm.DB) *NotificationStore { return &NotificationStore{db: db} }

type announcementRow struct {
	ID                   uuid.UUID
	Version              int64
	Draft                []byte
	PublishedRevision    *int64
	Published            []byte
	Enabled              bool
	CreatedAt, UpdatedAt time.Time
}

const announcementSelect = `SELECT a.id,a.version,a.draft,a.published_revision,r.content AS published,a.enabled,a.created_at,a.updated_at FROM identity.announcement a LEFT JOIN identity.announcement_revision r ON r.announcement_id=a.id AND r.revision=a.published_revision`

func announcementView(r announcementRow, now time.Time) (app.Announcement, error) {
	a := app.Announcement{ID: r.ID, Version: r.Version, Enabled: r.Enabled, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Status: "draft"}
	if err := json.Unmarshal(r.Draft, &a.Draft); err != nil {
		return a, err
	}
	if r.PublishedRevision != nil {
		var c app.AnnouncementContent
		if err := json.Unmarshal(r.Published, &c); err != nil {
			return a, err
		}
		a.Published = &c
		v := strconv.FormatInt(*r.PublishedRevision, 10)
		a.PublishedRevision = &v
		a.Status = "active"
		switch {
		case !r.Enabled:
			a.Status = "disabled"
		case !now.Before(c.EndsAt):
			a.Status = "expired"
		case now.Before(c.StartsAt):
			a.Status = "scheduled"
		}
	}
	return a, nil
}

func notificationActor(tx *gorm.DB, a app.Principal, admin bool, clock func() time.Time) error {
	u, err := lockSelfActorWithKeyCompatibility(tx, a, clock, true)
	if err != nil {
		return err
	}
	if admin && (u.PlatformRole != "platform_admin" || u.PlatformEpoch != a.PlatformEpoch) {
		return app.ErrForbidden
	}
	return nil
}
func notificationHash(v any) string {
	b, _ := json.Marshal(v)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func notificationReplay(tx *gorm.DB, a, cmd uuid.UUID, fingerprint string, out any) (bool, error) {
	var row struct {
		Fingerprint string
		Response    []byte
	}
	r := tx.Raw(`SELECT fingerprint,response FROM identity.notification_command WHERE actor_id=? AND command_id=?`, a, cmd).Scan(&row)
	if r.Error != nil || r.RowsAffected == 0 {
		return false, r.Error
	}
	if row.Fingerprint != fingerprint {
		return false, app.ErrNotificationConflict
	}
	return true, json.Unmarshal(row.Response, out)
}
func notificationReceipt(tx *gorm.DB, a, cmd uuid.UUID, fingerprint string, out any, now time.Time) error {
	b, err := json.Marshal(out)
	if err != nil {
		return err
	}
	return tx.Exec(`INSERT INTO identity.notification_command(actor_id,command_id,fingerprint,response,created_at) VALUES(?,?,?,?::jsonb,?)`, a, cmd, fingerprint, string(b), now).Error
}
func notificationAudit(tx *gorm.DB, a app.Principal, id, cmd uuid.UUID, action string, v int64, now time.Time) error {
	if err := tx.Exec(`INSERT INTO identity.notification_audit(id,announcement_id,actor_id,command_id,action,version,created_at) VALUES(?,?,?,?,?,?,?)`, uuid.New(), id, a.ID, cmd, action, v, now).Error; err != nil {
		return err
	}
	return tx.Exec(`INSERT INTO identity.security_event(id,actor_id,scope_kind,action,result,request_id,safe_diff,created_at) VALUES(?,?,'platform',?,'success',?,?::jsonb,?)`, uuid.New(), a.ID, "notification."+action, cmd.String(), fmt.Sprintf(`{"announcement_id":%q,"version":%q}`, id.String(), strconv.FormatInt(v, 10)), now).Error
}
func loadAnnouncement(tx *gorm.DB, id uuid.UUID, lock bool) (announcementRow, error) {
	var r announcementRow
	q := announcementSelect + ` WHERE a.id=?`
	if lock {
		q += ` FOR UPDATE OF a`
	}
	res := tx.Raw(q, id).Scan(&r)
	if res.Error != nil {
		return r, res.Error
	}
	if res.RowsAffected != 1 {
		return r, app.ErrNotificationNotFound
	}
	return r, nil
}
func validateAnnouncementTeam(tx *gorm.DB, c app.AnnouncementContent) error {
	if c.TeamID == nil {
		return nil
	}
	var n int64
	if err := tx.Raw(`SELECT count(*) FROM workspace.team WHERE id=? AND status='active' AND NOT governance_blocked`, *c.TeamID).Scan(&n).Error; err != nil {
		return err
	}
	if n != 1 {
		return app.ErrInvalidNotification
	}
	return nil
}

// SaveAnnouncement commits a draft, command receipt and audit atomically.
func (s *NotificationStore) SaveAnnouncement(ctx context.Context, a app.Principal, id uuid.UUID, in app.SaveAnnouncementInput, clock func() time.Time) (app.Announcement, error) {
	var out app.Announcement
	fingerprint := notificationHash([]any{"save", id, in})
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := notificationActor(tx, a, true, clock); err != nil {
			return err
		}
		if ok, err := notificationReplay(tx, a.ID, in.CommandID, fingerprint, &out); ok || err != nil {
			return err
		}
		if err := validateAnnouncementTeam(tx, in.Content); err != nil {
			return err
		}
		now := clock().UTC()
		b, err := json.Marshal(in.Content)
		if err != nil {
			return err
		}
		action := "saved"
		if id == uuid.Nil {
			id = uuid.New()
			action = "created"
			if err := tx.Exec(`INSERT INTO identity.announcement(id,version,draft,created_by,updated_by,created_at,updated_at) VALUES(?,1,?::jsonb,?,?,?,?)`, id, string(b), a.ID, a.ID, now, now).Error; err != nil {
				return err
			}
		} else {
			r, err := loadAnnouncement(tx, id, true)
			if err != nil {
				return err
			}
			if r.Version != in.ExpectedVersion {
				return app.ErrNotificationConflict
			}
			if err := tx.Exec(`UPDATE identity.announcement SET version=version+1,draft=?::jsonb,updated_by=?,updated_at=? WHERE id=?`, string(b), a.ID, now, id).Error; err != nil {
				return err
			}
		}
		r, err := loadAnnouncement(tx, id, false)
		if err != nil {
			return err
		}
		out, err = announcementView(r, now)
		if err != nil {
			return err
		}
		if err = notificationAudit(tx, a, id, in.CommandID, action, out.Version, now); err != nil {
			return err
		}
		return notificationReceipt(tx, a.ID, in.CommandID, fingerprint, out, now)
	})
	return out, err
}

// CommandAnnouncement serializes publication or enable-state changes against the expected version.
func (s *NotificationStore) CommandAnnouncement(ctx context.Context, a app.Principal, id uuid.UUID, action string, in app.AnnouncementCommand, clock func() time.Time) (app.Announcement, error) {
	var out app.Announcement
	fingerprint := notificationHash([]any{action, id, in})
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := notificationActor(tx, a, true, clock); err != nil {
			return err
		}
		if ok, err := notificationReplay(tx, a.ID, in.CommandID, fingerprint, &out); ok || err != nil {
			return err
		}
		r, err := loadAnnouncement(tx, id, true)
		if err != nil {
			return err
		}
		if r.Version != in.ExpectedVersion {
			return app.ErrNotificationConflict
		}
		now := clock().UTC()
		var content app.AnnouncementContent
		if action == "publish" {
			if r.PublishedRevision != nil && bytes.Equal(r.Draft, r.Published) {
				return app.ErrNotificationConflict
			}
			if err = json.Unmarshal(r.Draft, &content); err != nil {
				return err
			}
			if !content.EndsAt.After(now) {
				return app.ErrInvalidNotification
			}
			if err = validateAnnouncementTeam(tx, content); err != nil {
				return err
			}
			v := r.Version + 1
			if r.PublishedRevision != nil {
				if err = cancelFutureAnnouncement(tx, id, *r.PublishedRevision, now); err != nil {
					return err
				}
			}
			if err = tx.Exec(`INSERT INTO identity.announcement_revision(announcement_id,revision,content,published_at) VALUES(?,?,?::jsonb,?)`, id, v, string(r.Draft), now).Error; err != nil {
				return err
			}
			if err = tx.Exec(`UPDATE identity.announcement SET version=?,published_revision=?,enabled=true,updated_by=?,updated_at=? WHERE id=?`, v, v, a.ID, now, id).Error; err != nil {
				return err
			}
			action = "published"
			if err = projectAnnouncement(tx, id, v, content, now, nil); err != nil {
				return err
			}
		} else {
			if r.PublishedRevision == nil {
				return app.ErrInvalidNotification
			}
			if err = json.Unmarshal(r.Published, &content); err != nil {
				return err
			}
			if !content.EndsAt.After(now) {
				return app.ErrInvalidNotification
			}
			if err = validateAnnouncementTeam(tx, content); err != nil {
				return err
			}
			if err = tx.Exec(`UPDATE identity.announcement SET version=version+1,enabled=?,updated_by=?,updated_at=? WHERE id=?`, in.Enabled, a.ID, now, id).Error; err != nil {
				return err
			}
			action = "disabled"
			if in.Enabled {
				action = "enabled"
				if err = tx.Exec(`UPDATE identity.notification_inbox SET cancelled_at=NULL WHERE announcement_id=? AND revision=? AND inbox_seq IS NULL`, id, *r.PublishedRevision).Error; err != nil {
					return err
				}
				if err = projectAnnouncement(tx, id, *r.PublishedRevision, content, now, nil); err != nil {
					return err
				}
			} else if err = cancelFutureAnnouncement(tx, id, *r.PublishedRevision, now); err != nil {
				return err
			}
		}
		r, err = loadAnnouncement(tx, id, false)
		if err != nil {
			return err
		}
		out, err = announcementView(r, now)
		if err != nil {
			return err
		}
		if err = notificationAudit(tx, a, id, in.CommandID, action, out.Version, now); err != nil {
			return err
		}
		return notificationReceipt(tx, a.ID, in.CommandID, fingerprint, out, now)
	})
	return out, err
}

// projectAnnouncement runs only while the announcement row is locked. Mailbox
// rows are locked in account order before sequence allocation, so a read watermark
// cannot overtake an uncommitted earlier arrival.
func projectAnnouncement(tx *gorm.DB, id uuid.UUID, rev int64, c app.AnnouncementContent, now time.Time, target *uuid.UUID) error {
	if !c.Inbox && c.Placement != "inbox" {
		return nil
	}
	eligible := `u.status='active' AND NOT u.is_delete AND u.email_verified_at IS NOT NULL`
	args := []any{}
	if c.Audience == "team" {
		eligible += ` AND EXISTS(SELECT 1 FROM workspace.team_member tm JOIN workspace.team t ON t.id=tm.team_id WHERE tm.user_id=u.id AND tm.team_id=? AND tm.status='active' AND t.status='active' AND NOT t.governance_blocked)`
		args = append(args, *c.TeamID)
	}
	if target != nil {
		eligible += ` AND u.id=?`
		args = append(args, *target)
	}
	if err := tx.Exec(`INSERT INTO identity.notification_mailbox(account_id,data_epoch) SELECT u.id,gen_random_uuid() FROM identity."user" u WHERE `+eligible+` ON CONFLICT DO NOTHING`, args...).Error; err != nil {
		return err
	}
	lockArgs := append(append([]any{}, args...), id, rev)
	var accounts []uuid.UUID
	if err := tx.Raw(`SELECT m.account_id FROM identity.notification_mailbox m JOIN identity."user" u ON u.id=m.account_id WHERE `+eligible+` AND NOT EXISTS(SELECT 1 FROM identity.notification_inbox n WHERE n.account_id=m.account_id AND n.announcement_id=? AND n.revision=?) ORDER BY m.account_id FOR UPDATE OF m`, lockArgs...).Scan(&accounts).Error; err != nil {
		return err
	}
	if len(accounts) == 0 {
		return nil
	}
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	if now.Before(c.StartsAt) {
		return tx.Exec(`INSERT INTO identity.notification_inbox(id,account_id,announcement_id,revision,category,content,occurred_at) SELECT gen_random_uuid(),account_id,?,?,'system',?::jsonb,? FROM identity.notification_mailbox WHERE account_id IN ? ON CONFLICT(account_id,announcement_id,revision) DO NOTHING`, id, rev, string(b), c.StartsAt, accounts).Error
	}
	return tx.Exec(`WITH advanced AS (UPDATE identity.notification_mailbox m SET last_seq=last_seq+1,version=version+1 WHERE account_id IN ? AND NOT EXISTS(SELECT 1 FROM identity.notification_inbox n WHERE n.account_id=m.account_id AND n.announcement_id=? AND n.revision=?) RETURNING account_id,last_seq) INSERT INTO identity.notification_inbox(id,account_id,inbox_seq,announcement_id,revision,category,content,occurred_at) SELECT gen_random_uuid(),account_id,last_seq,?,?,'system',?::jsonb,? FROM advanced`, accounts, id, rev, id, rev, string(b), now).Error
}

func cancelFutureAnnouncement(tx *gorm.DB, id uuid.UUID, revision int64, now time.Time) error {
	return tx.Exec(`UPDATE identity.notification_inbox SET cancelled_at=? WHERE announcement_id=? AND revision=? AND inbox_seq IS NULL AND occurred_at>?`, now, id, revision, now).Error
}

// GetAnnouncement rechecks management authority and reads one announcement.
func (s *NotificationStore) GetAnnouncement(ctx context.Context, a app.Principal, id uuid.UUID, clock func() time.Time) (app.Announcement, error) {
	var out app.Announcement
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := notificationActor(tx, a, true, clock); err != nil {
			return err
		}
		r, err := loadAnnouncement(tx, id, false)
		if err != nil {
			return err
		}
		out, err = announcementView(r, clock())
		return err
	})
	return out, err
}

type notificationCursor struct {
	Hash string    `json:"h"`
	At   time.Time `json:"at"`
	ID   uuid.UUID `json:"id"`
	Seq  int64     `json:"seq"`
	AsOf time.Time `json:"as_of"`
}

func notificationPosition(a uuid.UUID, f app.NotificationFilter, now time.Time) (notificationCursor, error) {
	conditions := f
	conditions.Cursor = ""
	hash := notificationHash([]any{a, conditions})
	p := notificationCursor{Hash: hash, AsOf: now}
	if f.Cursor != "" {
		if len(f.Cursor) > 2048 {
			return p, app.ErrInvalidNotification
		}
		b, err := base64.RawURLEncoding.DecodeString(f.Cursor)
		if err != nil || json.Unmarshal(b, &p) != nil || p.Hash != hash || p.AsOf.IsZero() || p.AsOf.After(now.Add(time.Second)) {
			return p, app.ErrInvalidNotification
		}
	}
	return p, nil
}
func encodeNotificationCursor(p notificationCursor) *string {
	b, _ := json.Marshal(p)
	v := base64.RawURLEncoding.EncodeToString(b)
	return &v
}

const announcementStatus = `(CASE WHEN a.published_revision IS NULL THEN 'draft' WHEN NOT a.enabled THEN 'disabled' WHEN (r.content->>'ends_at')::timestamptz<=? THEN 'expired' WHEN (r.content->>'starts_at')::timestamptz>? THEN 'scheduled' ELSE 'active' END)`

// ListAnnouncements applies validated conditions before cursor pagination.
func (s *NotificationStore) ListAnnouncements(ctx context.Context, a app.Principal, f app.NotificationFilter, clock func() time.Time) (app.AnnouncementPage, error) {
	out := app.AnnouncementPage{Items: []app.Announcement{}}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := notificationActor(tx, a, true, clock); err != nil {
			return err
		}
		p, err := notificationPosition(a.ID, f, clock())
		if err != nil {
			return err
		}
		q := tx.Table("identity.announcement a").Joins(`LEFT JOIN identity.announcement_revision r ON r.announcement_id=a.id AND r.revision=a.published_revision`).Select("a.id,a.version,a.draft,a.published_revision,r.content AS published,a.enabled,a.created_at,a.updated_at").Where("a.updated_at<=?", p.AsOf)
		if f.Search != "" {
			q = q.Where(`a.draft->>'title' ILIKE ?`, "%"+f.Search+"%")
		}
		if f.Placement != "" {
			q = q.Where(`a.draft->>'placement'=?`, f.Placement)
		}
		if f.Audience != "" {
			q = q.Where(`a.draft->>'audience'=?`, f.Audience)
		}
		if f.Status != "" {
			q = q.Where(announcementStatus+`=?`, p.AsOf, p.AsOf, f.Status)
		}
		if f.From != nil {
			q = q.Where(`(a.draft->>'starts_at')::timestamptz>=?`, *f.From)
		}
		if f.To != nil {
			q = q.Where(`(a.draft->>'starts_at')::timestamptz<?`, *f.To)
		}
		if p.ID != uuid.Nil {
			q = q.Where(`(a.updated_at,a.id)<(?,?)`, p.At, p.ID)
		}
		var rows []announcementRow
		if err = q.Order("a.updated_at DESC,a.id DESC").Limit(f.Limit + 1).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) > f.Limit {
			rows = rows[:f.Limit]
			last := rows[len(rows)-1]
			p.ID = last.ID
			p.At = last.UpdatedAt
			out.NextCursor = encodeNotificationCursor(p)
		}
		for _, r := range rows {
			v, err := announcementView(r, p.AsOf)
			if err != nil {
				return err
			}
			out.Items = append(out.Items, v)
		}
		return nil
	})
	return out, err
}

// AnnouncementAudit reads at most the latest one hundred management records.
func (s *NotificationStore) AnnouncementAudit(ctx context.Context, a app.Principal, id uuid.UUID, clock func() time.Time) (app.NotificationAuditPage, error) {
	out := app.NotificationAuditPage{Items: []app.NotificationAudit{}}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := notificationActor(tx, a, true, clock); err != nil {
			return err
		}
		if _, err := loadAnnouncement(tx, id, false); err != nil {
			return err
		}
		return tx.Raw(`SELECT id,actor_id,action,version,created_at FROM identity.notification_audit WHERE announcement_id=? ORDER BY created_at DESC,id DESC LIMIT 100`, id).Scan(&out.Items).Error
	})
	return out, err
}
