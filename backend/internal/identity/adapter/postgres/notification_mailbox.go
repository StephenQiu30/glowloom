package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	app "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	pginbox "github.com/StephenQiu30/glowloom-server/backend/internal/infra/inbox/adapter/postgres"
)

func syncNotificationMailbox(tx *gorm.DB, a uuid.UUID, now time.Time) error {
	var rows []announcementRow
	if err := tx.Raw(announcementSelect+` WHERE (a.enabled AND (r.content->>'starts_at')::timestamptz<=? AND (coalesce((r.content->>'endless')::boolean,false) OR (r.content->>'ends_at')::timestamptz>?)) OR EXISTS(SELECT 1 FROM identity.notification_inbox n WHERE n.announcement_id=a.id AND n.account_id=? AND n.inbox_seq IS NULL AND n.cancelled_at IS NULL AND n.occurred_at<=?) ORDER BY a.id FOR SHARE OF a`, now, now, a, now).Scan(&rows).Error; err != nil {
		return err
	}
	for _, r := range rows {
		var c app.AnnouncementContent
		if err := json.Unmarshal(r.Published, &c); err != nil {
			return err
		}
		if !r.Enabled || now.Before(c.StartsAt) || (!c.Endless && !now.Before(c.EndsAt)) {
			continue
		}
		if err := projectAnnouncement(tx, r.ID, *r.PublishedRevision, c, now, &a); err != nil {
			return err
		}
	}
	if err := tx.Exec(`INSERT INTO identity.notification_mailbox(account_id,data_epoch) VALUES(?,?) ON CONFLICT DO NOTHING`, a, uuid.New()).Error; err != nil {
		return err
	}
	return openScheduledNotifications(tx, a, now)
}

// Future deliveries capture their original audience without consuming a sequence.
// Opening under the mailbox lock preserves read watermarks even after expiry.
func openScheduledNotifications(tx *gorm.DB, account uuid.UUID, now time.Time) error {
	var pending []struct {
		ID      uuid.UUID
		Content []byte
	}
	if err := tx.Raw(`SELECT id,content FROM identity.notification_inbox WHERE account_id=? AND inbox_seq IS NULL AND cancelled_at IS NULL AND occurred_at<=? ORDER BY occurred_at,id`, account, now).Scan(&pending).Error; err != nil {
		return err
	}
	if len(pending) == 0 {
		return nil
	}
	var seq int64
	if err := tx.Raw(`SELECT last_seq FROM identity.notification_mailbox WHERE account_id=? FOR UPDATE`, account).Scan(&seq).Error; err != nil {
		return err
	}
	opened := 0
	for _, row := range pending {
		var content app.AnnouncementContent
		if err := json.Unmarshal(row.Content, &content); err != nil {
			return err
		}
		eligible, err := notificationEligible(content, tx, &app.Principal{ID: account})
		if err != nil {
			return err
		}
		if !eligible {
			if err = tx.Exec(`UPDATE identity.notification_inbox SET cancelled_at=? WHERE id=? AND inbox_seq IS NULL`, now, row.ID).Error; err != nil {
				return err
			}
			continue
		}
		seq++
		if err = tx.Exec(`UPDATE identity.notification_inbox SET inbox_seq=? WHERE id=? AND inbox_seq IS NULL`, seq, row.ID).Error; err != nil {
			return err
		}
		opened++
	}
	if opened == 0 {
		return nil
	}
	return tx.Exec(`UPDATE identity.notification_mailbox SET last_seq=?,version=version+? WHERE account_id=?`, seq, opened, account).Error
}
func mailboxSummary(tx *gorm.DB, a uuid.UUID) (app.NotificationSummary, error) {
	var out app.NotificationSummary
	err := tx.Raw(`SELECT m.data_epoch,m.version AS mailbox_version,m.last_seq AS max_inbox_seq,(SELECT count(*) FROM identity.notification_inbox n WHERE n.account_id=m.account_id AND n.read_at IS NULL AND n.inbox_seq IS NOT NULL) AS unread_count FROM identity.notification_mailbox m WHERE account_id=?`, a).Scan(&out).Error
	return out, err
}

// Summary backfills active announcements and reads the current mailbox watermark.
func (s *NotificationStore) Summary(ctx context.Context, a app.Principal, clock func() time.Time) (app.NotificationSummary, error) {
	var out app.NotificationSummary
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := notificationActor(tx, a, false, clock); err != nil {
			return err
		}
		if err := syncNotificationMailbox(tx, a.ID, clock()); err != nil {
			return err
		}
		var err error
		out, err = mailboxSummary(tx, a.ID)
		return err
	})
	return out, err
}

type inboxRow struct {
	ID             uuid.UUID
	InboxSeq       int64
	AnnouncementID *uuid.UUID
	Revision       *int64
	SourceKind     string
	SourceID       *uuid.UUID
	Category       string
	Content        []byte
	ReadAt         *time.Time
	OccurredAt     time.Time
	Enabled        bool
}

func inboxView(r inboxRow, now time.Time) (app.InboxNotification, error) {
	var c struct {
		app.AnnouncementContent
		Status string `json:"status"`
	}
	err := json.Unmarshal(r.Content, &c)
	status := "active"
	switch {
	case r.SourceKind == "operation":
		status = c.Status
	case !r.Enabled:
		status = "disabled"
	case !c.Endless && !now.Before(c.EndsAt):
		status = "expired"
	}
	return app.InboxNotification{ID: r.ID, InboxSeq: r.InboxSeq, AnnouncementID: r.AnnouncementID, Revision: r.Revision, SourceKind: r.SourceKind, SourceID: r.SourceID, Category: r.Category, Title: c.Title, Body: c.Body, ActionPath: c.ActionPath, Read: r.ReadAt != nil, OccurredAt: r.OccurredAt, Status: status}, err
}

// ListNotifications reads only the authenticated account after current-announcement backfill.
func (s *NotificationStore) ListNotifications(ctx context.Context, a app.Principal, f app.NotificationFilter, clock func() time.Time) (app.NotificationPage, error) {
	out := app.NotificationPage{Items: []app.InboxNotification{}}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := notificationActor(tx, a, false, clock); err != nil {
			return err
		}
		if err := syncNotificationMailbox(tx, a.ID, clock()); err != nil {
			return err
		}
		p, err := notificationPosition(a.ID, f, clock())
		if err != nil {
			return err
		}
		q := tx.Table("identity.notification_inbox n").Joins("LEFT JOIN identity.announcement a ON a.id=n.announcement_id").Select("n.*,coalesce(a.enabled,false) AS enabled").Where("n.account_id=? AND n.inbox_seq IS NOT NULL AND n.occurred_at<=?", a.ID, p.AsOf)
		switch f.Category {
		case "interaction":
			q = q.Where("n.category=?", "collaboration")
		case "official":
			q = q.Where("n.category IN ?", []string{"system", "task", "account"})
		case "":
		default:
			q = q.Where("n.category=?", f.Category)
		}
		if f.UnreadOnly {
			q = q.Where("n.read_at IS NULL")
		}
		if f.From != nil {
			q = q.Where("n.occurred_at>=?", *f.From)
		}
		if f.To != nil {
			q = q.Where("n.occurred_at<?", *f.To)
		}
		if p.Seq > 0 {
			q = q.Where("n.inbox_seq<?", p.Seq)
		}
		var rows []inboxRow
		if err = q.Order("n.inbox_seq DESC").Limit(f.Limit + 1).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) > f.Limit {
			rows = rows[:f.Limit]
			p.Seq = rows[len(rows)-1].InboxSeq
			out.NextCursor = encodeNotificationCursor(p)
		}
		for _, r := range rows {
			v, err := inboxView(r, clock())
			if err != nil {
				return err
			}
			out.Items = append(out.Items, v)
		}
		out.NotificationSummary, err = mailboxSummary(tx, a.ID)
		return err
	})
	return out, err
}

// GetNotification reads only an already-delivered account-owned snapshot.
func (s *NotificationStore) GetNotification(ctx context.Context, a app.Principal, id uuid.UUID, clock func() time.Time) (app.InboxNotification, error) {
	var out app.InboxNotification
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := notificationActor(tx, a, false, clock); err != nil {
			return err
		}
		if err := syncNotificationMailbox(tx, a.ID, clock()); err != nil {
			return err
		}
		var r inboxRow
		res := tx.Raw(`SELECT n.*,coalesce(a.enabled,false) AS enabled FROM identity.notification_inbox n LEFT JOIN identity.announcement a ON a.id=n.announcement_id WHERE n.id=? AND n.account_id=? AND n.inbox_seq IS NOT NULL`, id, a.ID).Scan(&r)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return app.ErrNotificationNotFound
		}
		var err error
		out, err = inboxView(r, clock())
		return err
	})
	return out, err
}

// ProjectTaskNotification commits the event marker, immutable safe snapshot and
// mailbox sequence in one transaction; the source event identifies its recipient.
func (s *NotificationStore) ProjectTaskNotification(ctx context.Context, in app.TaskNotificationInput) error {
	_, err := pginbox.NewStore(s.db).ProcessOnce(ctx, "identity.notification.operation:"+in.RecipientID.String(), in.EventID.String(), func(_ context.Context, tx *gorm.DB) error {
		var recipient struct{ ID uuid.UUID }
		result := tx.Raw(`SELECT id FROM identity."user" WHERE id=? AND status='active' AND NOT is_delete AND email_verified_at IS NOT NULL FOR SHARE`, in.RecipientID).Scan(&recipient)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		if err := tx.Exec(`INSERT INTO identity.notification_mailbox(account_id,data_epoch) VALUES(?,?) ON CONFLICT DO NOTHING`, in.RecipientID, uuid.New()).Error; err != nil {
			return err
		}
		var seq int64
		if err := tx.Raw(`SELECT last_seq FROM identity.notification_mailbox WHERE account_id=? FOR UPDATE`, in.RecipientID).Scan(&seq).Error; err != nil {
			return err
		}
		var exists bool
		if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM identity.notification_inbox WHERE account_id=? AND source_event_id=?)`, in.RecipientID, in.EventID).Scan(&exists).Error; err != nil {
			return err
		}
		if exists {
			return nil
		}
		if err := tx.Raw(`UPDATE identity.notification_mailbox SET last_seq=last_seq+1,version=version+1 WHERE account_id=? RETURNING last_seq`, in.RecipientID).Scan(&seq).Error; err != nil {
			return err
		}
		title, body := "任务已完成", "生成任务已完成，可前往资产查看。"
		if in.Status == "failed" {
			title, body = "任务失败", "生成任务未能完成，可前往资产查看记录。"
		}
		content, err := json.Marshal(struct {
			Title      string `json:"title"`
			Body       string `json:"body"`
			ActionPath string `json:"action_path"`
			Status     string `json:"status"`
		}{title, body, "/assets?view=history", in.Status})
		if err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO identity.notification_inbox(id,account_id,inbox_seq,source_kind,source_id,source_event_id,category,content,occurred_at) VALUES(?,?,?,'operation',?,?,'task',?::jsonb,?)`, uuid.New(), in.RecipientID, seq, in.OperationID, in.EventID, string(content), in.OccurredAt).Error
	})
	return err
}

// ReadNotification serializes watermark reads with concurrent mailbox arrivals.
func (s *NotificationStore) ReadNotification(ctx context.Context, a app.Principal, id uuid.UUID, in app.ReadAllNotificationsInput, clock func() time.Time) (app.NotificationSummary, error) {
	var out app.NotificationSummary
	fingerprint := notificationHash([]any{"read", id, in})
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := notificationActor(tx, a, false, clock); err != nil {
			return err
		}
		if err := syncNotificationMailbox(tx, a.ID, clock()); err != nil {
			return err
		}
		if ok, err := notificationReplay(tx, a.ID, in.CommandID, fingerprint, &out); ok || err != nil {
			if err != nil {
				return err
			}
			out, err = mailboxSummary(tx, a.ID)
			return err
		}
		var m struct {
			DataEpoch uuid.UUID
			LastSeq   int64
		}
		if err := tx.Raw(`SELECT data_epoch,last_seq FROM identity.notification_mailbox WHERE account_id=? FOR UPDATE`, a.ID).Scan(&m).Error; err != nil {
			return err
		}
		q := tx.Table("identity.notification_inbox").Where("account_id=? AND read_at IS NULL AND inbox_seq IS NOT NULL", a.ID)
		if id != uuid.Nil {
			var count int64
			if err := tx.Table("identity.notification_inbox").Where("account_id=? AND id=? AND inbox_seq IS NOT NULL", a.ID, id).Count(&count).Error; err != nil {
				return err
			}
			if count != 1 {
				return app.ErrNotificationNotFound
			}
			q = q.Where("id=?", id)
		} else {
			if m.DataEpoch != in.DataEpoch || in.ThroughSeq > m.LastSeq {
				return app.ErrNotificationConflict
			}
			q = q.Where("inbox_seq<=?", in.ThroughSeq)
		}
		r := q.Update("read_at", clock())
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected > 0 {
			if err := tx.Exec(`UPDATE identity.notification_mailbox SET version=version+1 WHERE account_id=?`, a.ID).Error; err != nil {
				return err
			}
		}
		var err error
		out, err = mailboxSummary(tx, a.ID)
		if err != nil {
			return err
		}
		return notificationReceipt(tx, a.ID, in.CommandID, fingerprint, out, clock())
	})
	return out, err
}
