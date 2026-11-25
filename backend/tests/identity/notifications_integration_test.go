package identity_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/internal/creation/workspace/adapter/postgres"
	workspace "github.com/StephenQiu30/glowloom-server/backend/internal/creation/workspace/application"
	pg "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

func notificationContent(now time.Time) app.AnnouncementContent {
	return app.AnnouncementContent{Title: "服务维护", Body: "维护期间部分服务暂停。", Placement: "banner", Audience: "authenticated", StartsAt: now.Add(-time.Minute), EndsAt: now.Add(time.Hour), Inbox: true}
}

func TestNotificationsPublicationReadWatermarkAndReceipts(t *testing.T) {
	tx, _, admin, now := governanceFixture(t)
	_, user := verifiedTestActor(t, tx, "notification-reader@example.invalid", now)
	svc := app.NewNotifications(pg.NewNotificationStore(tx), func() time.Time { return now })
	create := app.SaveAnnouncementInput{CommandID: uuid.New(), Content: notificationContent(now)}
	a, err := svc.Save(t.Context(), admin, uuid.Nil, create)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := svc.Save(t.Context(), admin, uuid.Nil, create)
	if err != nil || replay.ID != a.ID {
		t.Fatal("create replay", err)
	}
	if _, err = svc.Save(t.Context(), user, uuid.Nil, create); !errors.Is(err, app.ErrForbidden) {
		t.Fatal("ordinary user managed announcements", err)
	}
	cmd := app.AnnouncementCommand{CommandID: uuid.New(), ExpectedVersion: a.Version}
	a, err = svc.Command(t.Context(), admin, a.ID, "publish", cmd)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Command(t.Context(), admin, a.ID, "publish", cmd); err != nil {
		t.Fatal("publication replay", err)
	}
	first, err := svc.Inbox(t.Context(), user, app.NotificationFilter{})
	if err != nil || len(first.Items) != 1 || first.UnreadCount != 1 {
		t.Fatal("projected inbox", first, err)
	}
	read := app.ReadAllNotificationsInput{CommandID: uuid.New(), DataEpoch: first.DataEpoch, ThroughSeq: first.MaxInboxSeq}
	second, err := svc.Save(t.Context(), admin, uuid.Nil, app.SaveAnnouncementInput{CommandID: uuid.New(), Content: notificationContent(now)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Command(t.Context(), admin, second.ID, "publish", app.AnnouncementCommand{CommandID: uuid.New(), ExpectedVersion: second.Version}); err != nil {
		t.Fatal(err)
	}
	summary, err := svc.Read(t.Context(), user, uuid.Nil, read)
	if err != nil || summary.UnreadCount != 1 {
		t.Fatal("new arrival incorrectly read", summary, err)
	}
	if again, err := svc.Read(t.Context(), user, uuid.Nil, read); err != nil || again.UnreadCount != 1 {
		t.Fatal("read replay", again, err)
	}
	if _, err = svc.Detail(t.Context(), admin, first.Items[0].ID); !errors.Is(err, app.ErrNotificationNotFound) {
		t.Fatal("cross account detail", err)
	}
	display, err := svc.Display(t.Context(), &user)
	if err != nil || display.Banner == nil {
		t.Fatal("banner missing", err)
	}
	if err = svc.Ack(t.Context(), user, app.NotificationDisplayAck{AnnouncementID: display.Banner.ID, Revision: display.Banner.Revision, Kind: "dismissed"}); err != nil {
		t.Fatal(err)
	}
	a, err = svc.Command(t.Context(), admin, a.ID, "enabled", app.AnnouncementCommand{CommandID: uuid.New(), ExpectedVersion: a.Version, Enabled: false})
	if err != nil || a.Status != "disabled" {
		t.Fatal("disable", a, err)
	}
	if _, err = svc.Command(t.Context(), admin, a.ID, "enabled", app.AnnouncementCommand{CommandID: uuid.New(), ExpectedVersion: a.Version - 1, Enabled: true}); !errors.Is(err, app.ErrNotificationConflict) {
		t.Fatal("stale CAS accepted", err)
	}
	if _, err = svc.Read(t.Context(), user, uuid.Nil, app.ReadAllNotificationsInput{CommandID: uuid.New(), DataEpoch: uuid.New(), ThroughSeq: 1}); !errors.Is(err, app.ErrNotificationConflict) {
		t.Fatal("wrong epoch", err)
	}
	audit, err := svc.Audit(t.Context(), admin, a.ID)
	if err != nil || len(audit.Items) != 3 {
		t.Fatal("atomic audit", audit, err)
	}
}

func TestNotificationValidation(t *testing.T) {
	now := time.Now().UTC()
	svc := app.NewNotifications(nil, func() time.Time { return now })
	for _, path := range []string{"https://example.invalid", "//evil.invalid/", "/assets\\evil", "javascript:alert(1)"} {
		c := notificationContent(now)
		c.ActionPath = path
		if _, err := svc.Save(t.Context(), app.Principal{}, uuid.Nil, app.SaveAnnouncementInput{CommandID: uuid.New(), Content: c}); !errors.Is(err, app.ErrInvalidNotification) {
			t.Fatal("unsafe action accepted", path, err)
		}
	}
	if _, err := svc.Inbox(t.Context(), app.Principal{}, app.NotificationFilter{Limit: 101}); !errors.Is(err, app.ErrInvalidNotification) {
		t.Fatal("unbounded query", err)
	}
}

func TestNotificationsSidebarPublication(t *testing.T) {
	tx, _, admin, now := governanceFixture(t)
	_, member := verifiedTestActor(t, tx, "sidebar-reader@example.invalid", now)
	svc := app.NewNotifications(pg.NewNotificationStore(tx), func() time.Time { return now })
	publish := func(content app.AnnouncementContent) app.Announcement {
		t.Helper()
		a, err := svc.Save(t.Context(), admin, uuid.Nil, app.SaveAnnouncementInput{CommandID: uuid.New(), Content: content})
		if err != nil {
			t.Fatal(err)
		}
		a, err = svc.Command(t.Context(), admin, a.ID, "publish", app.AnnouncementCommand{CommandID: uuid.New(), ExpectedVersion: a.Version})
		if err != nil {
			t.Fatal(err)
		}
		return a
	}

	promotion := notificationContent(now)
	promotion.Placement, promotion.Audience, promotion.ActionPath = "sidebar", "public", "/ai-tools"
	promotion.Body, promotion.Inbox = "", false
	public := publish(promotion)

	badge := promotion
	badge.Title, badge.Placement, badge.Audience, badge.ActionPath = "New", "sidebar_badge", "authenticated", "/short-drama"
	badge.Priority = 1
	private := publish(badge)

	for _, tc := range []struct {
		actor *app.Principal
		want  uuid.UUID
	}{{nil, public.ID}, {&member, private.ID}} {
		view, err := svc.Display(t.Context(), tc.actor)
		if err != nil || view.Sidebar == nil || view.Sidebar.ID != tc.want || view.Banner != nil || view.Popup != nil {
			t.Fatalf("sidebar display: %+v, %v", view, err)
		}
	}
	inbox, err := svc.Inbox(t.Context(), member, app.NotificationFilter{})
	if err != nil || len(inbox.Items) != 0 {
		t.Fatalf("sidebar content entered inbox: %+v, %v", inbox, err)
	}
	if _, err := svc.Command(t.Context(), admin, private.ID, "enabled", app.AnnouncementCommand{CommandID: uuid.New(), ExpectedVersion: private.Version, Enabled: false}); err != nil {
		t.Fatal(err)
	}
	view, err := svc.Display(t.Context(), &member)
	if err != nil || view.Sidebar == nil || view.Sidebar.ID != public.ID {
		t.Fatalf("stopped badge did not reveal eligible promotion: %+v, %v", view, err)
	}
	now = promotion.EndsAt
	view, err = svc.Display(t.Context(), nil)
	if err != nil || view.Sidebar != nil {
		t.Fatalf("expired sidebar promotion: %+v, %v", view, err)
	}
}

func TestNotificationsRevisionAudienceScheduleAndFilters(t *testing.T) {
	tx, _, admin, now := governanceFixture(t)
	_, user := verifiedTestActor(t, tx, "notification-target@example.invalid", now)
	_, other := verifiedTestActor(t, tx, "notification-other@example.invalid", now)
	svc := app.NewNotifications(pg.NewNotificationStore(tx), func() time.Time { return now })
	createPublish := func(c app.AnnouncementContent) app.Announcement {
		t.Helper()
		a, err := svc.Save(t.Context(), admin, uuid.Nil, app.SaveAnnouncementInput{CommandID: uuid.New(), Content: c})
		if err != nil {
			t.Fatal(err)
		}
		a, err = svc.Command(t.Context(), admin, a.ID, "publish", app.AnnouncementCommand{CommandID: uuid.New(), ExpectedVersion: a.Version})
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	modal := notificationContent(now)
	modal.Placement = "modal"
	a := createPublish(modal)
	display, err := svc.Display(t.Context(), &user)
	if err != nil || display.Popup == nil {
		t.Fatal("modal missing", display, err)
	}
	ack := app.NotificationDisplayAck{AnnouncementID: a.ID, Revision: display.Popup.Revision, Kind: "shown"}
	if err = svc.Ack(t.Context(), user, ack); err != nil {
		t.Fatal(err)
	}
	if err = svc.Ack(t.Context(), user, ack); !errors.Is(err, app.ErrNotificationConflict) {
		t.Fatal("second popup claim accepted", err)
	}
	display, err = svc.Display(t.Context(), &user)
	if err != nil || display.Popup != nil {
		t.Fatal("popup reappeared", err)
	}
	summary, err := svc.Summary(t.Context(), user)
	if err != nil || summary.UnreadCount != 1 {
		t.Fatal("show marked read", summary, err)
	}
	disabled, err := svc.Command(t.Context(), admin, a.ID, "enabled", app.AnnouncementCommand{CommandID: uuid.New(), ExpectedVersion: a.Version, Enabled: false})
	if err != nil {
		t.Fatal(err)
	}
	blocked, err := svc.Display(t.Context(), &other)
	if err != nil || blocked.Popup != nil {
		t.Fatal("disabled publication displayed to a new recipient", blocked, err)
	}
	if err = svc.Ack(t.Context(), other, ack); !errors.Is(err, app.ErrNotificationConflict) {
		t.Fatal("disabled publication claimed", err)
	}
	if _, err = svc.Command(t.Context(), admin, a.ID, "enabled", app.AnnouncementCommand{CommandID: uuid.New(), ExpectedVersion: disabled.Version, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	display, err = svc.Display(t.Context(), &user)
	if err != nil || display.Popup != nil {
		t.Fatal("restore reset show receipt", err)
	}

	public := notificationContent(now)
	public.Audience = "public"
	public.Title = "公开更新"
	public.Inbox = false
	createPublish(public)
	private := notificationContent(now)
	private.Title = "私人更新"
	private.Priority = 100
	createPublish(private)
	guest, err := svc.Display(t.Context(), nil)
	if err != nil || guest.Banner == nil || guest.Banner.Title != "公开更新" || guest.Popup != nil {
		t.Fatal("public data leaked", guest, err)
	}

	if err = tx.Exec(`UPDATE identity.platform_settings SET team_creation_enabled=true`).Error; err != nil {
		t.Fatal(err)
	}
	teams := workspace.NewTeams(pgworkspace.NewTeamStore(tx, func(tx *gorm.DB) workspace.TeamActorAccess { return pg.NewTeamActorStore(tx, completionProtector(t)) }), func() time.Time { return now })
	team, err := teams.Create(t.Context(), user, uuid.New(), "Notification recipients")
	if err != nil {
		t.Fatal(err)
	}
	teamContent := notificationContent(now)
	teamContent.Title = "团队更新"
	teamContent.Audience = "team"
	teamContent.TeamID = &team.ID
	teamContent.Priority = 99
	teamAnnouncement := createPublish(teamContent)
	validTeam, err := svc.ValidateDisplay(t.Context(), user, teamAnnouncement.ID, teamAnnouncement.Version)
	if err != nil || !validTeam.Valid {
		t.Fatal("team recipient validation", validTeam, err)
	}
	validOther, err := svc.ValidateDisplay(t.Context(), other, teamAnnouncement.ID, teamAnnouncement.Version)
	if err != nil || validOther.Valid {
		t.Fatal("foreign team validation", validOther, err)
	}
	otherPage, err := svc.Inbox(t.Context(), other, app.NotificationFilter{})
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range otherPage.Items {
		if n.AnnouncementID != nil && *n.AnnouncementID == teamAnnouncement.ID {
			t.Fatal("team message leaked")
		}
	}

	scheduled := notificationContent(now)
	scheduled.Title = "稍后生效"
	scheduled.StartsAt = now.Add(5 * time.Minute)
	scheduled.EndsAt = now.Add(10 * time.Minute)
	scheduledAnnouncement := createPublish(scheduled)
	if scheduledAnnouncement.Status != "scheduled" {
		t.Fatal("not scheduled", scheduledAnnouncement)
	}
	before, err := svc.Inbox(t.Context(), user, app.NotificationFilter{})
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range before.Items {
		if n.AnnouncementID != nil && *n.AnnouncementID == scheduledAnnouncement.ID {
			t.Fatal("scheduled message delivered early")
		}
	}
	now = now.Add(6 * time.Minute)
	after, err := svc.Inbox(t.Context(), user, app.NotificationFilter{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, n := range after.Items {
		if n.AnnouncementID != nil && *n.AnnouncementID == scheduledAnnouncement.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("login backfill missing")
	}
	page, err := svc.List(t.Context(), admin, app.NotificationFilter{Limit: 1, Status: "active"})
	if err != nil || len(page.Items) != 1 || page.NextCursor == nil {
		t.Fatal("filtered cursor page", page, err)
	}
	if _, err = svc.List(t.Context(), admin, app.NotificationFilter{Limit: 1, Status: "disabled", Cursor: *page.NextCursor}); !errors.Is(err, app.ErrInvalidNotification) {
		t.Fatal("changed filter reused cursor", err)
	}
	if _, err = svc.List(t.Context(), admin, app.NotificationFilter{Limit: 1, Status: "active", Cursor: *page.NextCursor}); err != nil {
		t.Fatal("valid cursor", err)
	}

	original := after.Items[0]
	if original.AnnouncementID == nil || *original.AnnouncementID != scheduledAnnouncement.ID {
		t.Fatal("notification order", original)
	}
	scheduled.Title = "修改草稿"
	draft, err := svc.Save(t.Context(), admin, scheduledAnnouncement.ID, app.SaveAnnouncementInput{CommandID: uuid.New(), ExpectedVersion: scheduledAnnouncement.Version, Content: scheduled})
	if err != nil {
		t.Fatal(err)
	}
	if draft.Published == nil || draft.Published.Title != "稍后生效" || draft.Draft.Title != "修改草稿" {
		t.Fatal("draft overwrote active publication", draft)
	}
	if _, err = svc.Command(t.Context(), admin, draft.ID, "publish", app.AnnouncementCommand{CommandID: uuid.New(), ExpectedVersion: draft.Version}); err != nil {
		t.Fatal(err)
	}
	old, err := svc.Detail(t.Context(), user, original.ID)
	if err != nil || old.Title != "稍后生效" {
		t.Fatal("inbox snapshot changed", old, err)
	}
	now = now.Add(5 * time.Minute)
	latest, err := svc.Get(t.Context(), admin, draft.ID)
	if err != nil || latest.Status != "expired" {
		t.Fatal("expiry", latest, err)
	}
	if _, err = svc.Command(t.Context(), admin, latest.ID, "enabled", app.AnnouncementCommand{CommandID: uuid.New(), ExpectedVersion: latest.Version, Enabled: true}); !errors.Is(err, app.ErrInvalidNotification) {
		t.Fatal("expired announcement reenabled", err)
	}
}

func TestNotificationsAuditFailureRollsBackProjection(t *testing.T) {
	tx, _, admin, now := governanceFixture(t)
	svc := app.NewNotifications(pg.NewNotificationStore(tx), func() time.Time { return now })
	a, err := svc.Save(t.Context(), admin, uuid.Nil, app.SaveAnnouncementInput{CommandID: uuid.New(), Content: notificationContent(now)})
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Exec(`ALTER TABLE identity.security_event ADD CONSTRAINT notification_test_audit_failure CHECK(action<>'notification.published') NOT VALID`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Command(t.Context(), admin, a.ID, "publish", app.AnnouncementCommand{CommandID: uuid.New(), ExpectedVersion: a.Version}); err == nil {
		t.Fatal("audit failure committed")
	}
	stored, err := svc.Get(t.Context(), admin, a.ID)
	if err != nil || stored.Version != a.Version || stored.Published != nil {
		t.Fatal("failed publication persisted", stored, err)
	}
	var n int64
	if err = tx.Table("identity.notification_inbox").Where("announcement_id=?", a.ID).Count(&n).Error; err != nil || n != 0 {
		t.Fatal("projection survived rollback", n, err)
	}
}

func TestNotificationsDisplayEffectiveTimeFence(t *testing.T) {
	tx, _, admin, now := governanceFixture(t)
	svc := app.NewNotifications(pg.NewNotificationStore(tx), func() time.Time { return now })
	content := notificationContent(now)
	content.Audience = "public"
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
	view, err := svc.Display(t.Context(), nil)
	if err != nil || view.Banner != nil {
		t.Fatal("scheduled banner displayed early", view, err)
	}
	now = now.Add(6 * time.Minute)
	view, err = svc.Display(t.Context(), nil)
	if err != nil || view.Banner == nil || view.Banner.ID != a.ID {
		t.Fatal("effective banner missing", view, err)
	}
	now = now.Add(5 * time.Minute)
	view, err = svc.Display(t.Context(), nil)
	if err != nil || view.Banner != nil {
		t.Fatal("expired banner displayed", view, err)
	}
}

func TestNotificationsRuntimeACL(t *testing.T) {
	tx, _, admin, now := governanceFixture(t)
	svc := app.NewNotifications(pg.NewNotificationStore(tx), func() time.Time { return now })
	for _, role := range []string{"glowloom_app", "glowloom_identity"} {
		if err := tx.Exec("SET LOCAL ROLE " + role).Error; err != nil {
			t.Fatal(err)
		}
		a, err := svc.Save(t.Context(), admin, uuid.Nil, app.SaveAnnouncementInput{CommandID: uuid.New(), Content: notificationContent(now)})
		if err != nil {
			t.Fatal(role, "save", err)
		}
		if _, err = svc.Command(t.Context(), admin, a.ID, "publish", app.AnnouncementCommand{CommandID: uuid.New(), ExpectedVersion: a.Version}); err != nil {
			t.Fatal(role, "publish", err)
		}
		if _, err = svc.Summary(t.Context(), admin); err != nil {
			t.Fatal(role, "summary", err)
		}
		if err = tx.Exec("RESET ROLE").Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, role := range []string{"glowloom_creation", "glowloom_media", "glowloom_execution"} {
		if err := tx.Exec("SAVEPOINT notification_role_probe").Error; err != nil {
			t.Fatal(err)
		}
		if err := tx.Exec("SET LOCAL ROLE " + role).Error; err != nil {
			t.Fatal(err)
		}
		var n int64
		if err := tx.Table("identity.notification_inbox").Count(&n).Error; err == nil {
			t.Fatal(role, "read another owner's inbox")
		}
		if err := tx.Exec("ROLLBACK TO SAVEPOINT notification_role_probe").Error; err != nil {
			t.Fatal(err)
		}
	}
}
