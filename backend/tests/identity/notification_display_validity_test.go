package identity_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	pg "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

func TestNotificationsDisplayValidityAfterClaimStopAndRevision(t *testing.T) {
	tx, _, admin, now := governanceFixture(t)
	_, user := verifiedTestActor(t, tx, "notification-validity@example.invalid", now)
	svc := app.NewNotifications(pg.NewNotificationStore(tx), func() time.Time { return now })
	content := notificationContent(now)
	content.Placement = "modal"
	content.Inbox = false
	content.EndsAt = now.Add(10 * time.Minute)
	a, err := svc.Save(t.Context(), admin, uuid.Nil, app.SaveAnnouncementInput{CommandID: uuid.New(), Content: content})
	if err != nil {
		t.Fatal(err)
	}
	a, err = svc.Command(t.Context(), admin, a.ID, "publish", app.AnnouncementCommand{CommandID: uuid.New(), ExpectedVersion: a.Version})
	if err != nil {
		t.Fatal(err)
	}
	display, err := svc.Display(t.Context(), &user)
	if err != nil || display.Popup == nil || display.Popup.NotificationID != nil {
		t.Fatal("popup without inbox", display, err)
	}
	rev := display.Popup.Revision
	if err = svc.Ack(t.Context(), user, app.NotificationDisplayAck{AnnouncementID: a.ID, Revision: rev, Kind: "shown"}); err != nil {
		t.Fatal(err)
	}
	valid, err := svc.ValidateDisplay(t.Context(), user, a.ID, rev)
	if err != nil || !valid.Valid {
		t.Fatal("claimed popup should remain valid", valid, err)
	}
	a, err = svc.Command(t.Context(), admin, a.ID, "enabled", app.AnnouncementCommand{CommandID: uuid.New(), ExpectedVersion: a.Version, Enabled: false})
	if err != nil {
		t.Fatal(err)
	}
	valid, err = svc.ValidateDisplay(t.Context(), user, a.ID, rev)
	if err != nil || valid.Valid {
		t.Fatal("disabled popup remained valid", valid, err)
	}
	content.Title = "新版本"
	a, err = svc.Save(t.Context(), admin, a.ID, app.SaveAnnouncementInput{CommandID: uuid.New(), ExpectedVersion: a.Version, Content: content})
	if err != nil {
		t.Fatal(err)
	}
	a, err = svc.Command(t.Context(), admin, a.ID, "publish", app.AnnouncementCommand{CommandID: uuid.New(), ExpectedVersion: a.Version})
	if err != nil {
		t.Fatal(err)
	}
	valid, err = svc.ValidateDisplay(t.Context(), user, a.ID, rev)
	if err != nil || valid.Valid {
		t.Fatal("old publication remained valid", valid, err)
	}
	valid, err = svc.ValidateDisplay(t.Context(), user, a.ID, a.Version)
	if err != nil || !valid.Valid {
		t.Fatal("new publication invalid", valid, err)
	}
	now = now.Add(11 * time.Minute)
	valid, err = svc.ValidateDisplay(t.Context(), user, a.ID, a.Version)
	if err != nil || valid.Valid {
		t.Fatal("expired popup remained valid", valid, err)
	}
}
