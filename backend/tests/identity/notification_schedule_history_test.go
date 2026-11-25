package identity_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	pg "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

func TestNotificationsScheduledHistoryOpensAfterExpiry(t *testing.T) {
	tx, _, admin, now := governanceFixture(t)
	_, user := verifiedTestActor(t, tx, "notification-offline-window@example.invalid", now)
	svc := app.NewNotifications(pg.NewNotificationStore(tx), func() time.Time { return now })
	content := notificationContent(now)
	content.StartsAt = now.Add(5 * time.Minute)
	content.EndsAt = now.Add(10 * time.Minute)
	a, err := svc.Save(t.Context(), admin, uuid.Nil, app.SaveAnnouncementInput{CommandID: uuid.New(), Content: content})
	if err != nil {
		t.Fatal(err)
	}
	a, err = svc.Command(t.Context(), admin, a.ID, "publish", app.AnnouncementCommand{CommandID: uuid.New(), ExpectedVersion: a.Version})
	if err != nil {
		t.Fatal(err)
	}
	before, err := svc.Summary(t.Context(), user)
	if err != nil || before.UnreadCount != 0 || before.MaxInboxSeq != 0 {
		t.Fatal("future message consumed mailbox watermark", before, err)
	}
	if _, err = svc.Read(t.Context(), user, uuid.Nil, app.ReadAllNotificationsInput{CommandID: uuid.New(), DataEpoch: before.DataEpoch, ThroughSeq: before.MaxInboxSeq}); err != nil {
		t.Fatal(err)
	}
	now = now.Add(11 * time.Minute)
	after, err := svc.Inbox(t.Context(), user, app.NotificationFilter{})
	if err != nil || len(after.Items) != 1 || after.UnreadCount != 1 || after.Items[0].Status != "expired" {
		t.Fatal("offline recipient lost scheduled history or was marked read before arrival", after, err)
	}
	_, late := verifiedTestActor(t, tx, "notification-born-after-window@example.invalid", now)
	lateInbox, err := svc.Inbox(t.Context(), late, app.NotificationFilter{})
	if err != nil || len(lateInbox.Items) != 0 {
		t.Fatal("new account inherited expired audience", lateInbox, err)
	}
}

func TestNotificationsScheduledStopReplaceAndWatermark(t *testing.T) {
	tx, _, admin, now := governanceFixture(t)
	_, user := verifiedTestActor(t, tx, "notification-scheduled-controls@example.invalid", now)
	svc := app.NewNotifications(pg.NewNotificationStore(tx), func() time.Time { return now })
	content := notificationContent(now)
	content.StartsAt = now.Add(5 * time.Minute)
	content.EndsAt = now.Add(10 * time.Minute)
	a, err := svc.Save(t.Context(), admin, uuid.Nil, app.SaveAnnouncementInput{CommandID: uuid.New(), Content: content})
	if err != nil {
		t.Fatal(err)
	}
	a, err = svc.Command(t.Context(), admin, a.ID, "publish", app.AnnouncementCommand{CommandID: uuid.New(), ExpectedVersion: a.Version})
	if err != nil {
		t.Fatal(err)
	}
	a, err = svc.Command(t.Context(), admin, a.ID, "enabled", app.AnnouncementCommand{CommandID: uuid.New(), ExpectedVersion: a.Version, Enabled: false})
	if err != nil {
		t.Fatal(err)
	}
	content.Title = "替换后的预约"
	a, err = svc.Save(t.Context(), admin, a.ID, app.SaveAnnouncementInput{CommandID: uuid.New(), ExpectedVersion: a.Version, Content: content})
	if err != nil {
		t.Fatal(err)
	}
	a, err = svc.Command(t.Context(), admin, a.ID, "publish", app.AnnouncementCommand{CommandID: uuid.New(), ExpectedVersion: a.Version})
	if err != nil {
		t.Fatal(err)
	}
	a, err = svc.Command(t.Context(), admin, a.ID, "enabled", app.AnnouncementCommand{CommandID: uuid.New(), ExpectedVersion: a.Version, Enabled: false})
	if err != nil {
		t.Fatal(err)
	}
	a, err = svc.Command(t.Context(), admin, a.ID, "enabled", app.AnnouncementCommand{CommandID: uuid.New(), ExpectedVersion: a.Version, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.ProjectTask(t.Context(), app.TaskNotificationInput{EventID: uuid.New(), RecipientID: user.ID, OperationID: uuid.New(), Status: "completed", OccurredAt: now}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := svc.Summary(t.Context(), user)
	if err != nil || snapshot.MaxInboxSeq != 1 {
		t.Fatal("pending message allocated sequence", snapshot, err)
	}
	now = now.Add(6 * time.Minute)
	result, err := svc.Read(t.Context(), user, uuid.Nil, app.ReadAllNotificationsInput{CommandID: uuid.New(), DataEpoch: snapshot.DataEpoch, ThroughSeq: snapshot.MaxInboxSeq})
	if err != nil || result.UnreadCount != 1 || result.MaxInboxSeq != 2 {
		t.Fatal("newly effective message crossed old watermark", result, err)
	}
	page, err := svc.Inbox(t.Context(), user, app.NotificationFilter{Category: "system"})
	if err != nil || len(page.Items) != 1 || page.Items[0].Title != content.Title {
		t.Fatal("stopped/replaced pending messages leaked or restore duplicated", page, err)
	}
}
