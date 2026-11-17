package identity_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"

	pg "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/postgres"
	app "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	platformdb "github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
)

// This test intentionally commits and uses independent connections. It requires
// a dedicated disposable database, never the generic existing-database fixture.
func TestNotificationsConcurrentCommittedWatermarkAndClaim(t *testing.T) {
	dsn := os.Getenv("GLOWLOOM_TEST_NOTIFICATION_DB_DSN")
	if dsn == "" {
		t.Skip("dedicated disposable notifications database absent")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	conn, err := platformdb.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatal("connect notification test database failed")
	}
	defer func() {
		if err := conn.Close(); err != nil {
			t.Error("close notification test database", err)
		}
	}()
	now := time.Now().UTC().Truncate(time.Microsecond)
	suffix := uuid.NewString()
	_, admin := verifiedTestActor(t, conn.DB, "notification-admin-"+suffix+"@example.invalid", now)
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := conn.DB.WithContext(cleanup).Exec(`UPDATE identity.announcement SET enabled=false WHERE created_by=?`, admin.ID).Error; err != nil {
			t.Error("disable committed test announcements", err)
		}
	}()
	userAuth, user := verifiedTestActor(t, conn.DB, "notification-reader-"+suffix+"@example.invalid", now)
	if err = conn.DB.Exec(`UPDATE identity."user" SET platform_role='platform_admin' WHERE id=?`, admin.ID).Error; err != nil {
		t.Fatal(err)
	}
	admin.PlatformRole = "platform_admin"
	svc := app.NewNotifications(pg.NewNotificationStore(conn.DB), func() time.Time { return now })
	for i := 0; i < 3; i++ {
		a, err := svc.Save(ctx, admin, uuid.Nil, app.SaveAnnouncementInput{CommandID: uuid.New(), Content: notificationContent(now)})
		if err != nil {
			t.Fatal(err)
		}
		snapshot, err := svc.Summary(ctx, user)
		if err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		results := make(chan error, 2)
		go func() {
			<-start
			_, err := svc.Command(ctx, admin, a.ID, "publish", app.AnnouncementCommand{CommandID: uuid.New(), ExpectedVersion: a.Version})
			results <- err
		}()
		go func() {
			<-start
			_, err := svc.Read(ctx, user, uuid.Nil, app.ReadAllNotificationsInput{CommandID: uuid.New(), DataEpoch: snapshot.DataEpoch, ThroughSeq: snapshot.MaxInboxSeq})
			results <- err
		}()
		close(start)
		for j := 0; j < 2; j++ {
			if err = <-results; err != nil {
				t.Fatal("concurrent mutation", err)
			}
		}
		after, err := svc.Summary(ctx, user)
		if err != nil || after.UnreadCount != 1 {
			t.Fatal("new arrival crossed old watermark", after, err)
		}
	}
	modal := notificationContent(now)
	modal.Placement = "modal"
	modal.Frequency = "daily"
	a, err := svc.Save(ctx, admin, uuid.Nil, app.SaveAnnouncementInput{CommandID: uuid.New(), Content: modal})
	if err != nil {
		t.Fatal(err)
	}
	a, err = svc.Command(ctx, admin, a.ID, "publish", app.AnnouncementCommand{CommandID: uuid.New(), ExpectedVersion: a.Version})
	if err != nil {
		t.Fatal(err)
	}
	display, err := svc.Display(ctx, &user)
	if err != nil || display.Popup == nil {
		t.Fatal("missing popup", err)
	}
	ack := app.NotificationDisplayAck{AnnouncementID: display.Popup.ID, Revision: display.Popup.Revision, Kind: "shown"}
	secondLogin, err := userAuth.Login(ctx, "notification-reader-"+suffix+"@example.invalid", "long password for tests", false)
	if err != nil {
		t.Fatal(err)
	}
	otherSession := user
	otherSession.SessionID = secondLogin.Session.ID
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, actor := range []app.Principal{user, otherSession} {
		go func() { <-start; results <- svc.Ack(ctx, actor, ack) }()
	}
	close(start)
	claimed, conflicted := 0, 0
	for i := 0; i < 2; i++ {
		err = <-results
		switch {
		case err == nil:
			claimed++
		case errors.Is(err, app.ErrNotificationConflict):
			conflicted++
		default:
			t.Fatal(err)
		}
	}
	if claimed != 1 || conflicted != 1 {
		t.Fatal("popup claim not exclusive", claimed, conflicted)
	}
	start = make(chan struct{})
	results = make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			_, err := svc.Command(ctx, admin, a.ID, "enabled", app.AnnouncementCommand{CommandID: uuid.New(), ExpectedVersion: a.Version, Enabled: false})
			results <- err
		}()
	}
	close(start)
	changed, stale := 0, 0
	for i := 0; i < 2; i++ {
		err = <-results
		switch {
		case err == nil:
			changed++
		case errors.Is(err, app.ErrNotificationConflict):
			stale++
		default:
			t.Fatal(err)
		}
	}
	if changed != 1 || stale != 1 {
		t.Fatal("CAS accepted concurrent stale writes", changed, stale)
	}
	snapshot, err := svc.Summary(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	task := app.TaskNotificationInput{EventID: uuid.New(), RecipientID: user.ID, OperationID: uuid.New(), Status: "completed", OccurredAt: now}
	start = make(chan struct{})
	results = make(chan error, 3)
	for i := 0; i < 2; i++ {
		go func() { <-start; results <- svc.ProjectTask(ctx, task) }()
	}
	go func() {
		<-start
		_, err := svc.Read(ctx, user, uuid.Nil, app.ReadAllNotificationsInput{CommandID: uuid.New(), DataEpoch: snapshot.DataEpoch, ThroughSeq: snapshot.MaxInboxSeq})
		results <- err
	}()
	close(start)
	for i := 0; i < 3; i++ {
		if err = <-results; err != nil {
			t.Fatal("task event/read concurrency", err)
		}
	}
	tasks, err := svc.Inbox(ctx, user, app.NotificationFilter{Category: "task"})
	if err != nil || len(tasks.Items) != 1 || tasks.UnreadCount != 1 || tasks.MaxInboxSeq != snapshot.MaxInboxSeq+1 {
		t.Fatal("task duplicate/watermark", tasks, err)
	}
}
