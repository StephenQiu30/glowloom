package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	app "github.com/StephenQiu30/glowloom-server/backend/identity/application"
)

// Banners dismiss for the announcement lifetime. Popup buckets are derived only
// from server time and the authenticated session; revisions never reset a bucket.
func notificationDisplayScope(c app.AnnouncementContent, a app.Principal, now time.Time) string {
	if c.Placement != "modal" {
		return "once"
	}
	switch c.Frequency {
	case "daily":
		return "daily:" + now.In(time.FixedZone("Asia/Shanghai", 8*60*60)).Format("2006-01-02")
	case "login":
		return "login:" + a.SessionID.String()
	default:
		return "once"
	}
}

func notificationEligible(c app.AnnouncementContent, tx *gorm.DB, a *app.Principal) (bool, error) {
	if a == nil {
		return c.Audience == "public", nil
	}
	if c.Audience != "team" {
		return true, nil
	}
	var n int64
	err := tx.Raw(`SELECT count(*) FROM workspace.team_member tm JOIN workspace.team t ON t.id=tm.team_id WHERE tm.user_id=? AND tm.team_id=? AND tm.status='active' AND t.status='active' AND NOT t.governance_blocked`, a.ID, *c.TeamID).Scan(&n).Error
	return n == 1, err
}

// ValidateDisplay ignores display receipts while checking the current published
// revision, enabled state, effective period and account audience.
func (s *NotificationStore) ValidateDisplay(ctx context.Context, a app.Principal, id uuid.UUID, revision int64, clock func() time.Time) (app.NotificationDisplayValidity, error) {
	var out app.NotificationDisplayValidity
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := notificationActor(tx, a, false, clock); err != nil {
			return err
		}
		r, err := loadAnnouncement(tx, id, false)
		if errors.Is(err, app.ErrNotificationNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if !r.Enabled || r.PublishedRevision == nil || *r.PublishedRevision != revision {
			return nil
		}
		var c app.AnnouncementContent
		if err = json.Unmarshal(r.Published, &c); err != nil {
			return err
		}
		now := clock()
		if c.Placement == "inbox" || app.IsSidebarPlacement(c.Placement) || now.Before(c.StartsAt) || (!c.Endless && !now.Before(c.EndsAt)) {
			return nil
		}
		out.Valid, err = notificationEligible(c, tx, &a)
		return err
	})
	return out, err
}

// Display filters active publications by audience and independent display receipts.
func (s *NotificationStore) Display(ctx context.Context, a *app.Principal, clock func() time.Time) (app.NotificationDisplayView, error) {
	var out app.NotificationDisplayView
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if a != nil {
			if err := notificationActor(tx, *a, false, clock); err != nil {
				return err
			}
			if err := syncNotificationMailbox(tx, a.ID, clock()); err != nil {
				return err
			}
		}
		now := clock()
		var rows []announcementRow
		if err := tx.Raw(announcementSelect+` WHERE a.enabled AND (r.content->>'starts_at')::timestamptz<=? AND (coalesce((r.content->>'endless')::boolean,false) OR (r.content->>'ends_at')::timestamptz>?) AND r.content->>'placement' IN ('banner','modal','sidebar','sidebar_badge') ORDER BY (r.content->>'priority')::int DESC,r.published_at DESC,a.id`, now, now).Scan(&rows).Error; err != nil {
			return err
		}
		for _, r := range rows {
			var c app.AnnouncementContent
			if err := json.Unmarshal(r.Published, &c); err != nil {
				return err
			}
			if (c.Placement == "banner" && out.Banner != nil) ||
				(c.Placement == "modal" && (out.Popup != nil || a == nil)) ||
				(app.IsSidebarPlacement(c.Placement) && out.Sidebar != nil) {
				continue
			}
			ok, err := notificationEligible(c, tx, a)
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
			v := &app.DisplayAnnouncement{ID: r.ID, Revision: *r.PublishedRevision, Title: c.Title, Body: c.Body, Placement: c.Placement, ActionPath: c.ActionPath, DurationSeconds: c.DurationSeconds, StartsAt: c.StartsAt, EndsAt: c.EndsAt, Endless: c.Endless, ManualClose: c.ManualClose, Frequency: c.Frequency}
			if app.IsSidebarPlacement(c.Placement) {
				if out.Sidebar == nil {
					out.Sidebar = v
				}
				continue
			}
			if a != nil {
				scope := notificationDisplayScope(c, *a, now)
				var receipt struct{ ShownAt, DismissedAt *time.Time }
				if err := tx.Raw(`SELECT shown_at,dismissed_at FROM identity.notification_display_receipt WHERE account_id=? AND announcement_id=? AND scope_key=?`, a.ID, r.ID, scope).Scan(&receipt).Error; err != nil {
					return err
				}
				if receipt.DismissedAt != nil || (c.Placement == "modal" && receipt.ShownAt != nil) {
					continue
				}
				var n struct{ ID uuid.UUID }
				res := tx.Raw(`SELECT id FROM identity.notification_inbox WHERE account_id=? AND announcement_id=? AND inbox_seq IS NOT NULL ORDER BY inbox_seq LIMIT 1`, a.ID, r.ID).Scan(&n)
				if res.Error != nil {
					return res.Error
				}
				if res.RowsAffected == 1 {
					v.NotificationID = &n.ID
				}
			}
			if c.Placement == "banner" && out.Banner == nil {
				out.Banner = v
			}
			if c.Placement == "modal" && out.Popup == nil {
				out.Popup = v
			}
		}
		return nil
	})
	return out, err
}

// AckDisplay atomically claims popup display or records a dismissal without marking read.
func (s *NotificationStore) AckDisplay(ctx context.Context, a app.Principal, in app.NotificationDisplayAck, clock func() time.Time) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := notificationActor(tx, a, false, clock); err != nil {
			return err
		}
		// Claim under the publication lock so a committed stop or replacement
		// cannot be overtaken by a new popup receipt.
		r, err := loadAnnouncement(tx, in.AnnouncementID, true)
		if err != nil {
			return err
		}
		if !r.Enabled || r.PublishedRevision == nil || *r.PublishedRevision != in.Revision {
			return app.ErrNotificationConflict
		}
		var c app.AnnouncementContent
		if err = json.Unmarshal(r.Published, &c); err != nil {
			return err
		}
		now := clock()
		if app.IsSidebarPlacement(c.Placement) || now.Before(c.StartsAt) || (!c.Endless && !now.Before(c.EndsAt)) {
			return app.ErrNotificationConflict
		}
		eligible, err := notificationEligible(c, tx, &a)
		if err != nil {
			return err
		}
		if !eligible {
			return app.ErrForbidden
		}
		scope := notificationDisplayScope(c, a, now)
		if c.Placement == "modal" && in.Kind == "dismissed" {
			// A close acknowledges an existing popup claim; it must never claim a
			// new bucket. A previous-day popup may close after Beijing midnight,
			// when no current-day claim exists. That close is a harmless no-op.
			return tx.Exec(`UPDATE identity.notification_display_receipt SET dismissed_at=coalesce(dismissed_at,?) WHERE account_id=? AND announcement_id=? AND scope_key=? AND shown_at IS NOT NULL`, now, a.ID, in.AnnouncementID, scope).Error
		}
		var shown, dismissed *time.Time
		if in.Kind == "shown" {
			shown = &now
		} else {
			dismissed = &now
		}
		q := `INSERT INTO identity.notification_display_receipt(account_id,announcement_id,revision,scope_key,shown_at,dismissed_at) VALUES(?,?,?,?,?,?) ON CONFLICT(account_id,announcement_id,scope_key) DO UPDATE SET shown_at=coalesce(identity.notification_display_receipt.shown_at,EXCLUDED.shown_at),dismissed_at=coalesce(identity.notification_display_receipt.dismissed_at,EXCLUDED.dismissed_at)`
		if in.Kind == "shown" {
			q += ` WHERE identity.notification_display_receipt.shown_at IS NULL AND identity.notification_display_receipt.dismissed_at IS NULL`
		}
		result := tx.Exec(q, a.ID, in.AnnouncementID, in.Revision, scope, shown, dismissed)
		if result.Error != nil {
			return result.Error
		}
		if in.Kind == "shown" && result.RowsAffected != 1 {
			return app.ErrNotificationConflict
		}
		return nil
	})
}

var _ app.NotificationStore = (*NotificationStore)(nil)
