package identity_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"

	pg "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/postgres"
	app "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	platformdb "github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
)

func TestNotificationsColdMailboxReadAndEnable(t *testing.T) {
	dsn := os.Getenv("GLOWLOOM_TEST_NOTIFICATION_DB_DSN")
	if dsn == "" {
		t.Skip("dedicated disposable notifications database absent")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	conn, err := platformdb.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			t.Error(err)
		}
	}()
	now := time.Now().UTC().Truncate(time.Microsecond)
	suffix := uuid.NewString()
	_, admin := verifiedTestActor(t, conn.DB, "notification-cold-admin-"+suffix+"@example.invalid", now)
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := conn.DB.WithContext(cleanup).Exec(`UPDATE identity.announcement SET enabled=false WHERE created_by=?`, admin.ID).Error; err != nil {
			t.Error(err)
		}
	}()
	if err = conn.DB.Exec(`UPDATE identity."user" SET platform_role='platform_admin' WHERE id=?`, admin.ID).Error; err != nil {
		t.Fatal(err)
	}
	admin.PlatformRole = "platform_admin"
	svc := app.NewNotifications(pg.NewNotificationStore(conn.DB), func() time.Time { return now })
	a, err := svc.Save(ctx, admin, uuid.Nil, app.SaveAnnouncementInput{CommandID: uuid.New(), Content: notificationContent(now)})
	if err != nil {
		t.Fatal(err)
	}
	a, err = svc.Command(ctx, admin, a.ID, "publish", app.AnnouncementCommand{CommandID: uuid.New(), ExpectedVersion: a.Version})
	if err != nil {
		t.Fatal(err)
	}
	_, reader := verifiedTestActor(t, conn.DB, "notification-cold-reader-"+suffix+"@example.invalid", now)
	var mailboxes int64
	if err = conn.DB.Table("identity.notification_mailbox").Where("account_id=?", reader.ID).Count(&mailboxes).Error; err != nil || mailboxes != 0 {
		t.Fatal("reader mailbox was not cold", mailboxes, err)
	}
	writer := conn.DB.WithContext(ctx).Begin()
	if writer.Error != nil {
		t.Fatal(writer.Error)
	}
	defer func() { _ = writer.Rollback().Error }()
	var locked struct{ ID uuid.UUID }
	if err = writer.Raw(`SELECT id FROM identity.announcement WHERE id=? FOR UPDATE`, a.ID).Scan(&locked).Error; err != nil {
		t.Fatal(err)
	}
	readerResult := make(chan error, 1)
	go func() { _, err := svc.Summary(ctx, reader); readerResult <- err }()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for waiting := false; !waiting; {
		if err = conn.DB.WithContext(ctx).Raw(`SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE '%FROM identity.announcement a%')`).Scan(&waiting).Error; err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		select {
		case <-ticker.C:
		case err := <-readerResult:
			t.Fatal("reader did not reach announcement wait", err)
		case <-deadline.C:
			t.Fatal("reader did not acquire its account lock")
		}
	}
	writerService := app.NewNotifications(pg.NewNotificationStore(writer), func() time.Time { return now })
	if _, err = writerService.Command(ctx, admin, a.ID, "enabled", app.AnnouncementCommand{CommandID: uuid.New(), ExpectedVersion: a.Version, Enabled: true}); err != nil {
		t.Fatal("cold mailbox projection deadlocked against authenticated reader", err)
	}
	if err = writer.Commit().Error; err != nil {
		t.Fatal(err)
	}
	if err = <-readerResult; err != nil {
		t.Fatal("cold reader failed", err)
	}
	if summary, err := svc.Summary(ctx, reader); err != nil || summary.UnreadCount != 1 {
		t.Fatal("cold mailbox delivery", summary, err)
	}
}
