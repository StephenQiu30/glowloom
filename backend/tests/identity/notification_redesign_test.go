package identity_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	pg "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// Decode through the public JSON contract so this regression also catches fields
// silently ignored by the application boundary.
func redesignedNotificationContent(t *testing.T, c app.AnnouncementContent, fields map[string]any) app.AnnouncementContent {
	t.Helper()
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var values map[string]any
	if err = json.Unmarshal(b, &values); err != nil {
		t.Fatal(err)
	}
	for k, v := range fields {
		values[k] = v
	}
	b, err = json.Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	return c
}

func publishRedesignedNotification(t *testing.T, svc *app.Notifications, admin app.Principal, c app.AnnouncementContent) app.Announcement {
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

func TestNotificationRedesignEndlessAndFrequencyValidation(t *testing.T) {
	tx, _, admin, now := governanceFixture(t)
	svc := app.NewNotifications(pg.NewNotificationStore(tx), func() time.Time { return now })
	c := notificationContent(now)
	c.EndsAt = time.Time{}
	c = redesignedNotificationContent(t, c, map[string]any{"endless": true, "manual_close": true})
	a := publishRedesignedNotification(t, svc, admin, c)
	if a.Status != "active" {
		t.Fatal(a)
	}
	b, _ := json.Marshal(a.Published)
	var fields map[string]any
	if err := json.Unmarshal(b, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["frequency"] != "once" || fields["endless"] != true || fields["manual_close"] != true || fields["duration_seconds"] != float64(5) {
		t.Fatal("new fields/default lost", fields)
	}
	view, err := svc.Display(t.Context(), &admin)
	if err != nil || view.Banner == nil {
		t.Fatal("endless banner", view, err)
	}
	valid, err := svc.ValidateDisplay(t.Context(), admin, a.ID, a.Version)
	if err != nil || !valid.Valid {
		t.Fatal("endless validity", valid, err)
	}
	now = now.Add(30 * time.Minute)
	page, err := svc.List(t.Context(), admin, app.NotificationFilter{Status: "active"})
	if err != nil || len(page.Items) != 1 {
		t.Fatal("endless status", page, err)
	}
	for _, freq := range []string{"weekly", "ONCE"} {
		invalid := redesignedNotificationContent(t, notificationContent(now), map[string]any{"frequency": freq})
		if _, err = svc.Save(t.Context(), admin, uuid.Nil, app.SaveAnnouncementInput{CommandID: uuid.New(), Content: invalid}); !errors.Is(err, app.ErrInvalidNotification) {
			t.Fatal("invalid frequency", freq, err)
		}
	}
	invalid := redesignedNotificationContent(t, c, map[string]any{"duration_seconds": 61})
	if _, err = svc.Save(t.Context(), admin, uuid.Nil, app.SaveAnnouncementInput{CommandID: uuid.New(), Content: invalid}); !errors.Is(err, app.ErrInvalidNotification) {
		t.Fatal("manual close bypassed duration bounds", err)
	}
}

func TestNotificationRedesignRevisionKeepsReceiptAndInboxIdentity(t *testing.T) {
	tx, _, admin, now := governanceFixture(t)
	_, user := verifiedTestActor(t, tx, "redesign-revision@example.invalid", now)
	svc := app.NewNotifications(pg.NewNotificationStore(tx), func() time.Time { return now })
	c := notificationContent(now)
	c.Placement = "modal"
	a := publishRedesignedNotification(t, svc, admin, c)
	if err := svc.Ack(t.Context(), user, app.NotificationDisplayAck{AnnouncementID: a.ID, Revision: a.Version, Kind: "shown"}); err != nil {
		t.Fatal(err)
	}
	original, err := svc.Inbox(t.Context(), user, app.NotificationFilter{})
	if err != nil || len(original.Items) != 1 {
		t.Fatal(original, err)
	}
	oldRevision := a.Version
	c.Title = "修订后标题"
	c.EndsAt = c.EndsAt.Add(time.Hour)
	a, err = svc.Save(t.Context(), admin, a.ID, app.SaveAnnouncementInput{CommandID: uuid.New(), ExpectedVersion: a.Version, Content: c})
	if err != nil {
		t.Fatal(err)
	}
	a, err = svc.Command(t.Context(), admin, a.ID, "publish", app.AnnouncementCommand{CommandID: uuid.New(), ExpectedVersion: a.Version})
	if err != nil {
		t.Fatal(err)
	}
	view, err := svc.Display(t.Context(), &user)
	if err != nil || view.Popup != nil {
		t.Fatal("revision reset once receipt", view, err)
	}
	page, err := svc.Inbox(t.Context(), user, app.NotificationFilter{})
	if err != nil || len(page.Items) != 1 || page.MaxInboxSeq != original.MaxInboxSeq || page.Items[0].Title != "服务维护" {
		t.Fatal("revision resent/changed snapshot", page, err)
	}
	valid, err := svc.ValidateDisplay(t.Context(), user, a.ID, oldRevision)
	if err != nil || valid.Valid {
		t.Fatal("stale revision remains valid", valid, err)
	}
	_, newUser := verifiedTestActor(t, tx, "redesign-new@example.invalid", now)
	backfill, err := svc.Inbox(t.Context(), newUser, app.NotificationFilter{})
	if err != nil || len(backfill.Items) != 1 || backfill.Items[0].Title != "修订后标题" {
		t.Fatal("new account current revision", backfill, err)
	}
}

func TestNotificationRedesignDailyBeijingAndLoginSession(t *testing.T) {
	for _, frequency := range []string{"daily", "login"} {
		t.Run(frequency, func(t *testing.T) {
			tx, _, admin, initial := governanceFixture(t)
			// Align just before midnight in Beijing, but within existing session validity.
			now := initial
			boundary := time.Date(initial.Year(), initial.Month(), initial.Day(), 16, 0, 0, 0, time.UTC)
			if !boundary.After(now) {
				boundary = boundary.Add(24 * time.Hour)
			}
			now = boundary.Add(-time.Minute)
			if err := tx.Exec(`UPDATE identity.user_session SET created_at=?,last_active_at=?,absolute_expires_at=?,idle_timeout_seconds=259200`, now.Add(-time.Hour), now, now.Add(72*time.Hour)).Error; err != nil {
				t.Fatal(err)
			}
			auth, user := verifiedTestActor(t, tx, "redesign-frequency@example.invalid", now)
			svc := app.NewNotifications(pg.NewNotificationStore(tx), func() time.Time { return now })
			c := notificationContent(now)
			c.Placement = "modal"
			c.EndsAt = now.Add(2 * time.Hour)
			c = redesignedNotificationContent(t, c, map[string]any{"frequency": frequency})
			a := publishRedesignedNotification(t, svc, admin, c)
			ack := app.NotificationDisplayAck{AnnouncementID: a.ID, Revision: a.Version, Kind: "shown"}
			if err := svc.Ack(t.Context(), user, ack); err != nil {
				t.Fatal(err)
			}
			if err := svc.Ack(t.Context(), user, ack); !errors.Is(err, app.ErrNotificationConflict) {
				t.Fatal("same bucket claimed twice", err)
			}
			now = boundary.Add(time.Minute)
			view, err := svc.Display(t.Context(), &user)
			if err != nil {
				t.Fatal(err)
			}
			if frequency == "daily" {
				if view.Popup == nil {
					t.Fatal("Beijing new day missing", view)
				}
				if err = svc.Ack(t.Context(), user, ack); err != nil {
					t.Fatal(err)
				}
				// Another real session is the same account/day bucket.
				login, e := auth.Login(t.Context(), "redesign-frequency@example.invalid", "long password for tests", false)
				if e != nil {
					t.Fatal(e)
				}
				other := user
				other.SessionID = login.Session.ID
				if err = svc.Ack(t.Context(), other, ack); !errors.Is(err, app.ErrNotificationConflict) {
					t.Fatal("daily cross session duplicate", err)
				}
			} else {
				if view.Popup != nil {
					t.Fatal("same session became new login after midnight", view)
				}
				login, e := auth.Login(t.Context(), "redesign-frequency@example.invalid", "long password for tests", false)
				if e != nil {
					t.Fatal(e)
				}
				other := user
				other.SessionID = login.Session.ID
				view, err = svc.Display(t.Context(), &other)
				if err != nil || view.Popup == nil {
					t.Fatal("new real session missing", view, err)
				}
				if err = svc.Ack(t.Context(), other, ack); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestNotificationRedesignDraftDeleteAndAudit(t *testing.T) {
	tx, _, admin, now := governanceFixture(t)
	_, user := verifiedTestActor(t, tx, "redesign-delete@example.invalid", now)
	svc := app.NewNotifications(pg.NewNotificationStore(tx), func() time.Time { return now })
	a, err := svc.Save(t.Context(), admin, uuid.Nil, app.SaveAnnouncementInput{CommandID: uuid.New(), Content: notificationContent(now)})
	if err != nil {
		t.Fatal(err)
	}
	cmd := app.AnnouncementCommand{CommandID: uuid.New(), ExpectedVersion: a.Version}
	if _, err = svc.Command(t.Context(), user, a.ID, "delete", cmd); !errors.Is(err, app.ErrForbidden) {
		t.Fatal("ordinary user delete", err)
	}
	stale := cmd
	stale.CommandID = uuid.New()
	stale.ExpectedVersion++
	if _, err = svc.Command(t.Context(), admin, a.ID, "delete", stale); !errors.Is(err, app.ErrNotificationConflict) {
		t.Fatal("stale delete", err)
	}
	deleted, err := svc.Command(t.Context(), admin, a.ID, "delete", cmd)
	if err != nil || deleted.Status != "deleted" {
		t.Fatal("draft delete", deleted, err)
	}
	replay, err := svc.Command(t.Context(), admin, a.ID, "delete", cmd)
	if err != nil || replay.Version != deleted.Version {
		t.Fatal("delete replay", replay, err)
	}
	if _, err = svc.Get(t.Context(), admin, a.ID); !errors.Is(err, app.ErrNotificationNotFound) {
		t.Fatal("deleted detail", err)
	}
	page, err := svc.List(t.Context(), admin, app.NotificationFilter{})
	if err != nil || len(page.Items) != 0 {
		t.Fatal("deleted in list", page, err)
	}
	audit, err := svc.Audit(t.Context(), admin, a.ID)
	if err != nil || len(audit.Items) != 2 {
		t.Fatal("delete audit", audit, err)
	}
	deletedAudit := false
	for _, entry := range audit.Items {
		if entry.Action == "deleted" && entry.Version == deleted.Version {
			deletedAudit = true
		}
	}
	if !deletedAudit {
		t.Fatal("deleted command audit missing", audit)
	}
	published := publishRedesignedNotification(t, svc, admin, notificationContent(now))
	if _, err = svc.Command(t.Context(), admin, published.ID, "delete", app.AnnouncementCommand{CommandID: uuid.New(), ExpectedVersion: published.Version}); !errors.Is(err, app.ErrInvalidNotification) {
		t.Fatal("published deletion allowed", err)
	}
}

func TestNotificationRedesignGroupsFilterBeforePagination(t *testing.T) {
	tx, _, admin, now := governanceFixture(t)
	_, user := verifiedTestActor(t, tx, "redesign-group@example.invalid", now)
	svc := app.NewNotifications(pg.NewNotificationStore(tx), func() time.Time { return now })
	for _, category := range []string{"account", "system", "collaboration"} {
		c := notificationContent(now)
		c.Title = category
		a := publishRedesignedNotification(t, svc, admin, c)
		if err := tx.Exec(`UPDATE identity.notification_inbox SET category=? WHERE account_id=? AND announcement_id=?`, category, user.ID, a.ID).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.ProjectTask(t.Context(), app.TaskNotificationInput{EventID: uuid.New(), RecipientID: user.ID, OperationID: uuid.New(), Status: "completed", OccurredAt: now}); err != nil {
		t.Fatal(err)
	}
	// The last row is interaction, so filtering after LIMIT would yield an empty
	// official page and incorrectly lose a continuation.
	c := notificationContent(now)
	c.Title = "last interaction"
	a := publishRedesignedNotification(t, svc, admin, c)
	if err := tx.Exec(`UPDATE identity.notification_inbox SET category='collaboration' WHERE account_id=? AND announcement_id=?`, user.ID, a.ID).Error; err != nil {
		t.Fatal(err)
	}
	f := app.NotificationFilter{Category: "official", Limit: 1}
	var categories []string
	for {
		page, err := svc.Inbox(t.Context(), user, f)
		if err != nil || len(page.Items) != 1 {
			t.Fatal("official pagination", page, err)
		}
		categories = append(categories, page.Items[0].Category)
		if page.NextCursor == nil {
			break
		}
		if _, err = svc.Inbox(t.Context(), user, app.NotificationFilter{Category: "interaction", Limit: 1, Cursor: *page.NextCursor}); !errors.Is(err, app.ErrInvalidNotification) {
			t.Fatal("group cursor not bound", err)
		}
		f.Cursor = *page.NextCursor
	}
	if len(categories) != 3 || categories[0] != "task" || categories[1] != "system" || categories[2] != "account" {
		t.Fatal("official changed source categories", categories)
	}
	interaction, err := svc.Inbox(t.Context(), user, app.NotificationFilter{Category: "interaction", Limit: 1})
	if err != nil || len(interaction.Items) != 1 || interaction.NextCursor == nil || interaction.Items[0].Category != "collaboration" {
		t.Fatal("interaction pagination", interaction, err)
	}
}

func TestNotificationRedesignBannerDismissalSurvivesRevisionAndDay(t *testing.T) {
	tx, _, admin, now := governanceFixture(t)
	_, user := verifiedTestActor(t, tx, "redesign-banner@example.invalid", now)
	svc := app.NewNotifications(pg.NewNotificationStore(tx), func() time.Time { return now })
	c := notificationContent(now)
	c = redesignedNotificationContent(t, c, map[string]any{"endless": true, "frequency": "daily"})
	a := publishRedesignedNotification(t, svc, admin, c)
	if err := svc.Ack(t.Context(), user, app.NotificationDisplayAck{AnnouncementID: a.ID, Revision: a.Version, Kind: "dismissed"}); err != nil {
		t.Fatal(err)
	}
	c.Title = "改标题"
	a, err := svc.Save(t.Context(), admin, a.ID, app.SaveAnnouncementInput{CommandID: uuid.New(), ExpectedVersion: a.Version, Content: c})
	if err != nil {
		t.Fatal(err)
	}
	a, err = svc.Command(t.Context(), admin, a.ID, "publish", app.AnnouncementCommand{CommandID: uuid.New(), ExpectedVersion: a.Version})
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(24 * time.Hour)
	if err = tx.Exec(`UPDATE identity.user_session SET last_active_at=?,absolute_expires_at=? WHERE id IN ?`, now, now.Add(time.Hour), []uuid.UUID{admin.SessionID, user.SessionID}).Error; err != nil {
		t.Fatal(err)
	}
	view, err := svc.Display(t.Context(), &user)
	if err != nil || view.Banner != nil {
		t.Fatal("banner dismissal reset", view, err)
	}
	page, err := svc.Inbox(t.Context(), user, app.NotificationFilter{})
	if err != nil || len(page.Items) != 1 || page.UnreadCount != 1 || page.Items[0].Status != "active" {
		t.Fatal("banner dismissal affected unread/endless history", page, err)
	}
	_, late := verifiedTestActor(t, tx, "redesign-endless-late@example.invalid", now)
	latePage, err := svc.Inbox(t.Context(), late, app.NotificationFilter{})
	if err != nil || len(latePage.Items) != 1 || latePage.Items[0].Title != "改标题" {
		t.Fatal("endless new account backfill", latePage, err)
	}
	a, err = svc.Command(t.Context(), admin, a.ID, "enabled", app.AnnouncementCommand{CommandID: uuid.New(), ExpectedVersion: a.Version, Enabled: false})
	if err != nil {
		t.Fatal("endless disable", err)
	}
	historical, err := svc.Detail(t.Context(), user, page.Items[0].ID)
	if err != nil || historical.Status != "disabled" {
		t.Fatal("disabled history", historical, err)
	}
	_, afterStop := verifiedTestActor(t, tx, "redesign-disabled-late@example.invalid", now)
	stoppedPage, err := svc.Inbox(t.Context(), afterStop, app.NotificationFilter{})
	if err != nil || len(stoppedPage.Items) != 0 {
		t.Fatal("disabled backfilled", stoppedPage, err)
	}
	if _, err = svc.Command(t.Context(), admin, a.ID, "enabled", app.AnnouncementCommand{CommandID: uuid.New(), ExpectedVersion: a.Version, Enabled: true}); err != nil {
		t.Fatal("endless enable", err)
	}
}

func TestNotificationRedesignRefreshKeepsLoginBucket(t *testing.T) {
	tx, _, admin, now := governanceFixture(t)
	auth, _ := verifiedTestActor(t, tx, "redesign-refresh@example.invalid", now)
	auth.WithSessionSecurity(&v2Codec{claims: map[string]app.AccessClaims{}}, &v2Receipt{}, &v2Coordinator{})
	login, err := auth.Login(t.Context(), "redesign-refresh@example.invalid", "long password for tests", false)
	if err != nil {
		t.Fatal(err)
	}
	user := app.Principal{ID: login.User.ID, SessionID: login.Session.ID, CredentialEpoch: login.User.SessionEpoch}
	svc := app.NewNotifications(pg.NewNotificationStore(tx), func() time.Time { return now })
	c := notificationContent(now)
	c.Placement = "modal"
	c.Frequency = "login"
	a := publishRedesignedNotification(t, svc, admin, c)
	ack := app.NotificationDisplayAck{AnnouncementID: a.ID, Revision: a.Version, Kind: "shown"}
	if err = svc.Ack(t.Context(), user, ack); err != nil {
		t.Fatal(err)
	}
	refreshed, err := auth.Refresh(t.Context(), login.RefreshToken, login.CSRFToken, "notification-refresh", uuid.New())
	if err != nil {
		t.Fatal("refresh failed", err)
	}
	if refreshed.Session.ID != user.SessionID || refreshed.RefreshToken == login.RefreshToken {
		t.Fatal("refresh did not rotate within same session")
	}
	user.SessionID = refreshed.Session.ID
	view, err := svc.Display(t.Context(), &user)
	if err != nil || view.Popup != nil {
		t.Fatal("refresh created a new login bucket", view, err)
	}
	if err = svc.Ack(t.Context(), user, ack); !errors.Is(err, app.ErrNotificationConflict) {
		t.Fatal("refresh allowed duplicate claim", err)
	}
}

func TestNotificationRedesignDailyDismissalAcrossMidnight(t *testing.T) {
	for _, manualClose := range []bool{true, false} {
		name := "timeout"
		if manualClose {
			name = "manual"
		}
		t.Run(name, func(t *testing.T) {
			tx, _, admin, initial := governanceFixture(t)
			boundary := time.Date(initial.Year(), initial.Month(), initial.Day(), 16, 0, 0, 0, time.UTC)
			if !boundary.After(initial) {
				boundary = boundary.Add(24 * time.Hour)
			}
			now := boundary.Add(-time.Second)
			if err := tx.Exec(`UPDATE identity.user_session SET last_active_at=?,absolute_expires_at=? WHERE id=?`, now, now.Add(time.Hour), admin.SessionID).Error; err != nil {
				t.Fatal(err)
			}
			_, user := verifiedTestActor(t, tx, "redesign-midnight-"+name+"@example.invalid", now)
			svc := app.NewNotifications(pg.NewNotificationStore(tx), func() time.Time { return now })
			c := notificationContent(now)
			c.Placement = "modal"
			c.Frequency = "daily"
			c.ManualClose = manualClose
			a := publishRedesignedNotification(t, svc, admin, c)
			ack := app.NotificationDisplayAck{AnnouncementID: a.ID, Revision: a.Version, Kind: "shown"}
			if err := svc.Ack(t.Context(), user, ack); err != nil {
				t.Fatal("claim before Beijing midnight", err)
			}
			now = boundary.Add(4 * time.Second)
			ack.Kind = "dismissed"
			for range 2 {
				if err := svc.Ack(t.Context(), user, ack); err != nil {
					t.Fatal("close previous-day popup", err)
				}
			}
			var receipts int64
			if err := tx.Table("identity.notification_display_receipt").Where("account_id=? AND announcement_id=?", user.ID, a.ID).Count(&receipts).Error; err != nil || receipts != 1 {
				t.Fatal("dismissal created an unclaimed next-day receipt", receipts, err)
			}
			view, err := svc.Display(t.Context(), &user)
			if err != nil || view.Popup == nil {
				t.Fatal("previous-day close suppressed the new Beijing day", view, err)
			}
			ack.Kind = "shown"
			if err = svc.Ack(t.Context(), user, ack); err != nil {
				t.Fatal("next-day popup could not be claimed", err)
			}
			ack.Kind = "dismissed"
			if err = svc.Ack(t.Context(), user, ack); err != nil {
				t.Fatal("close current-day claimed popup", err)
			}
			var dismissed int64
			if err = tx.Table("identity.notification_display_receipt").Where("account_id=? AND announcement_id=? AND shown_at IS NOT NULL AND dismissed_at IS NOT NULL", user.ID, a.ID).Count(&dismissed).Error; err != nil || dismissed != 1 {
				t.Fatal("current-day shown receipt was not closed", dismissed, err)
			}
			summary, err := svc.Summary(t.Context(), user)
			if err != nil || summary.UnreadCount != 1 {
				t.Fatal("popup close changed mailbox read state", summary, err)
			}
		})
	}
}

func TestNotificationRedesignUnclaimedPopupDismissalDoesNotSuppress(t *testing.T) {
	tx, _, admin, now := governanceFixture(t)
	_, user := verifiedTestActor(t, tx, "redesign-unclaimed-close@example.invalid", now)
	svc := app.NewNotifications(pg.NewNotificationStore(tx), func() time.Time { return now })
	c := notificationContent(now)
	c.Placement = "modal"
	a := publishRedesignedNotification(t, svc, admin, c)
	ack := app.NotificationDisplayAck{AnnouncementID: a.ID, Revision: a.Version, Kind: "dismissed"}
	if err := svc.Ack(t.Context(), user, ack); err != nil {
		t.Fatal(err)
	}
	var receipts int64
	if err := tx.Table("identity.notification_display_receipt").Where("account_id=? AND announcement_id=?", user.ID, a.ID).Count(&receipts).Error; err != nil || receipts != 0 {
		t.Fatal("unclaimed modal dismissal stored a suppression", receipts, err)
	}
	view, err := svc.Display(t.Context(), &user)
	if err != nil || view.Popup == nil {
		t.Fatal("unclaimed modal dismissal hid the candidate", view, err)
	}
	ack.Kind = "shown"
	if err = svc.Ack(t.Context(), user, ack); err != nil {
		t.Fatal("unclaimed dismissal prevented first claim", err)
	}
}
