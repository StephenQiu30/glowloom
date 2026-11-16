package identity_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	identityevent "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/event"
	pg "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/postgres"
	app "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	pginbox "github.com/StephenQiu30/glowloom-server/backend/internal/infra/inbox/adapter/postgres"
	inbox "github.com/StephenQiu30/glowloom-server/backend/internal/infra/inbox/application"
	realtime "github.com/StephenQiu30/glowloom-server/backend/internal/infra/realtime/application"
)

type notificationEventSink struct {
	calls atomic.Int32
	fail  bool
}

func (s *notificationEventSink) Publish(_ context.Context, _ realtime.Event) error {
	if s.fail {
		return errors.New("test realtime unavailable")
	}
	s.calls.Add(1)
	return nil
}

func taskNotificationRecord(t *testing.T, event, project, operation uuid.UUID, recipient *uuid.UUID, status string, now time.Time) inbox.Record {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"event_id": event, "event_type": realtime.OperationStatusTopic, "occurred_at": now, "project_id": project, "recipient_id": recipient, "aggregate": map[string]any{"type": "operation", "id": operation}, "data": map[string]any{"target_type": "free", "status": status}})
	if err != nil {
		t.Fatal(err)
	}
	return inbox.Record{Topic: realtime.OperationStatusTopic, Key: []byte(project.String()), Value: payload}
}

func TestNotificationsTaskEventProjectionAndRecovery(t *testing.T) {
	tx, _, _, now := governanceFixture(t)
	_, user := verifiedTestActor(t, tx, "notification-task-reader@example.invalid", now)
	_, other := verifiedTestActor(t, tx, "notification-task-other@example.invalid", now)
	svc := app.NewNotifications(pg.NewNotificationStore(tx), func() time.Time { return now })
	var beforeAnnouncements int64
	if err := tx.Table("identity.announcement").Count(&beforeAnnouncements).Error; err != nil {
		t.Fatal(err)
	}
	sink := &notificationEventSink{}
	handler := identityevent.WithOperationNotifications(realtime.NewHandler(pginbox.NewStore(tx), sink), svc)
	event, project, operation := uuid.New(), uuid.New(), uuid.New()
	record := taskNotificationRecord(t, event, project, operation, &user.ID, "completed", now)
	for i := 0; i < 2; i++ {
		if err := handler.Handle(t.Context(), record); err != nil {
			t.Fatal(err)
		}
	}
	page, err := svc.Inbox(t.Context(), user, app.NotificationFilter{Category: "task"})
	if err != nil || len(page.Items) != 1 || page.UnreadCount != 1 || sink.calls.Load() != 1 {
		t.Fatal("task event deduplication", page, sink.calls.Load(), err)
	}
	n := page.Items[0]
	if n.AnnouncementID != nil || n.Revision != nil || n.SourceKind != "operation" || n.SourceID == nil || *n.SourceID != operation || n.ActionPath != "/assets?view=history" || n.Title != "任务已完成" {
		t.Fatal("task source snapshot", n)
	}
	if _, err = svc.Detail(t.Context(), other, n.ID); !errors.Is(err, app.ErrNotificationNotFound) {
		t.Fatal("foreign task detail", err)
	}
	var announcements, markers int64
	if err = tx.Table("identity.announcement").Count(&announcements).Error; err != nil || announcements != beforeAnnouncements {
		t.Fatal("task fabricated an announcement", announcements, err)
	}
	if err = tx.Table("infra.processed_event").Where("consumer=? AND event_id=?", "identity.notification.operation:"+user.ID.String(), event).Count(&markers).Error; err != nil || markers != 1 {
		t.Fatal("event receipt", markers, err)
	}
	for _, status := range []string{"submitting", "cancelled"} {
		if err = handler.Handle(t.Context(), taskNotificationRecord(t, uuid.New(), project, uuid.New(), &user.ID, status, now)); err != nil {
			t.Fatal(err)
		}
	}
	if err = handler.Handle(t.Context(), taskNotificationRecord(t, uuid.New(), project, uuid.New(), nil, "completed", now)); err != nil {
		t.Fatal("historical envelope", err)
	}
	if page, err = svc.Inbox(t.Context(), user, app.NotificationFilter{Category: "task"}); err != nil || len(page.Items) != 1 {
		t.Fatal("unsupported event produced task notification", page, err)
	}

	failed := taskNotificationRecord(t, uuid.New(), project, uuid.New(), &user.ID, "failed", now)
	if err = tx.Exec(`ALTER TABLE identity.notification_inbox ADD CONSTRAINT notification_task_test_failure CHECK(category<>'task') NOT VALID`).Error; err != nil {
		t.Fatal(err)
	}
	if err = handler.Handle(t.Context(), failed); err == nil {
		t.Fatal("inbox write failure accepted")
	}
	if err = tx.Exec(`ALTER TABLE identity.notification_inbox DROP CONSTRAINT notification_task_test_failure`).Error; err != nil {
		t.Fatal(err)
	}
	if err = handler.Handle(t.Context(), failed); err != nil {
		t.Fatal("projection recovery after realtime already committed", err)
	}
	page, err = svc.Inbox(t.Context(), user, app.NotificationFilter{Category: "task"})
	if err != nil || len(page.Items) != 2 || page.Items[0].Title != "任务失败" {
		t.Fatal("failed task recovery", page, err)
	}
	before := page.MaxInboxSeq
	if _, err = svc.Read(t.Context(), user, uuid.Nil, app.ReadAllNotificationsInput{CommandID: uuid.New(), DataEpoch: page.DataEpoch, ThroughSeq: before - 1}); err != nil {
		t.Fatal(err)
	}
	if summary, err := svc.Summary(t.Context(), user); err != nil || summary.UnreadCount != 1 {
		t.Fatal("task watermark", summary, err)
	}
}
