package identity_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/credentialproof"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

func TestSecurityEventScopeAndCursor(t *testing.T) {
	tx := existingIdentityTransaction(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	_, a := verifiedTestActor(t, tx, "event-a@example.invalid", now)
	_, b := verifiedTestActor(t, tx, "event-b@example.invalid", now)
	key, err := credentialproof.New("test-only", []byte(strings.Repeat("h", 32)), []byte(strings.Repeat("e", 32)))
	if err != nil {
		t.Fatal(err)
	}
	query := identity.NewEventQuery(pgidentity.NewEventStore(tx), key, func() time.Time { return now })
	insert := func(target uuid.UUID, team *uuid.UUID, scope string) {
		t.Helper()
		if err := tx.Exec(`INSERT INTO identity.security_event(id,actor_id,target_user_id,team_id,scope_kind,action,result,reason,request_id,safe_diff,created_at) VALUES(?,?,?,?,?,'profile.updated','success','private reason','internal-request','{"status":"active","password":"secret","email":"private@example.invalid"}',?)`, uuid.New(), b.ID, target, team, scope, now.Add(-time.Second)).Error; err != nil {
			t.Fatal(err)
		}
	}
	for range 3 {
		insert(a.ID, nil, "self")
	}
	insert(b.ID, nil, "self")
	filter := identity.EventFilter{ScopeKind: "self", Action: "profile.updated", Limit: 2}
	first, err := query.List(t.Context(), a, filter)
	if err != nil || len(first.Items) != 2 || first.NextCursor == nil {
		t.Fatalf("first page: %v", err)
	}
	for _, e := range first.Items {
		if e.TargetUserID == nil || *e.TargetUserID != a.ID || e.ActorID != nil || e.Reason != "" || e.RequestID != "" || len(e.SafeDiff) != 1 || e.SafeDiff["status"] != "active" {
			t.Fatal("self event leaked private fields")
		}
	}
	filter.Cursor = *first.NextCursor
	second, err := query.List(t.Context(), a, filter)
	if err != nil || len(second.Items) != 1 || second.NextCursor != nil {
		t.Fatalf("second page: %v", err)
	}
	for _, e := range first.Items {
		if e.ID == second.Items[0].ID {
			t.Fatal("page repeated row")
		}
	}
	now = now.Add(2 * time.Second)
	insert(a.ID, nil, "self")
	replay := filter
	replay.Cursor = first.FirstCursor
	back, err := query.List(t.Context(), a, replay)
	if err != nil || len(back.Items) != 2 || back.NextCursor == nil || !back.AsOf.Equal(first.AsOf) || back.Items[0].ID != first.Items[0].ID || back.Items[1].ID != first.Items[1].ID {
		t.Fatalf("replayed first page drifted: %v", err)
	}
	replay.Cursor = *back.NextCursor
	again, err := query.List(t.Context(), a, replay)
	if err != nil || len(again.Items) != 1 || !again.AsOf.Equal(first.AsOf) || again.Items[0].ID != second.Items[0].ID {
		t.Fatalf("replayed continuation drifted: %v", err)
	}
	replay.Cursor = first.FirstCursor
	if _, err := query.List(t.Context(), b, replay); !errors.Is(err, identity.ErrInvalidEventFilter) {
		t.Fatal("foreign first cursor accepted")
	}
	if _, err := query.List(t.Context(), b, filter); !errors.Is(err, identity.ErrInvalidEventFilter) {
		t.Fatal("cross-account cursor accepted")
	}
	changed := filter
	changed.Action = "preferences.updated"
	if _, err := query.List(t.Context(), a, changed); !errors.Is(err, identity.ErrInvalidEventFilter) {
		t.Fatal("cross-filter cursor accepted")
	}
	changed = filter
	changed.Cursor += "tamper"
	if _, err := query.List(t.Context(), a, changed); !errors.Is(err, identity.ErrInvalidEventFilter) {
		t.Fatal("tampered cursor accepted")
	}
	now = now.Add(10 * time.Minute)
	if _, err := query.List(t.Context(), a, filter); !errors.Is(err, identity.ErrInvalidEventFilter) {
		t.Fatal("expired cursor accepted")
	}
	if _, err := query.List(t.Context(), a, replay); !errors.Is(err, identity.ErrInvalidEventFilter) {
		t.Fatal("expired first cursor accepted")
	}
	reopened := filter
	reopened.Cursor = ""
	fresh, err := query.List(t.Context(), a, reopened)
	if err != nil || len(fresh.Items) != 2 || fresh.Items[0].ID == first.Items[0].ID || !fresh.AsOf.Equal(now) {
		t.Fatalf("failed to reopen fresh first page: %v", err)
	}
	now = now.Add(-10 * time.Minute)
	foreign := b.ID
	if _, err := query.List(t.Context(), a, identity.EventFilter{ScopeKind: "self", TargetUserID: &foreign}); !errors.Is(err, identity.ErrInvalidEventFilter) {
		t.Fatal("self query target overridden")
	}
	if _, err := query.List(t.Context(), a, identity.EventFilter{ScopeKind: "platform"}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatal("creator read platform audit")
	}
	if err := tx.Exec(`UPDATE identity."user" SET platform_role='platform_admin',platform_epoch=platform_epoch+1 WHERE id=?`, a.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := query.List(t.Context(), a, identity.EventFilter{ScopeKind: "platform"}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatal("stale platform epoch accepted")
	}
	a.PlatformRole = "platform_admin"
	a.PlatformEpoch++
	platformFilter := identity.EventFilter{ScopeKind: "platform", TargetUserID: &b.ID, Action: "profile.updated"}
	platformPage, err := query.List(t.Context(), a, platformFilter)
	if err != nil || len(platformPage.Items) != 1 || platformPage.Items[0].SafeDiff["password"] != "" {
		t.Fatal("platform projection invalid")
	}
	team := uuid.New()
	if err := tx.Exec(`INSERT INTO workspace.team(id,name,created_by) VALUES(?,'Event team',?)`, team, a.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`INSERT INTO workspace.team_member(team_id,user_id,role,status,joined_at,source) VALUES(?,?,'team_admin','active',?,'created')`, team, a.ID, now).Error; err != nil {
		t.Fatal(err)
	}
	insert(b.ID, &team, "team")
	teamFilter := identity.EventFilter{ScopeKind: "team", TeamID: &team, Action: "profile.updated"}
	if page, err := query.List(t.Context(), a, teamFilter); err != nil || len(page.Items) != 1 {
		t.Fatal("team admin cannot read own audit")
	}
	if _, err := query.List(t.Context(), b, teamFilter); !errors.Is(err, identity.ErrForbidden) {
		t.Fatal("outsider read team audit")
	}
	if err := tx.Exec(`UPDATE workspace.team_member SET status='removed' WHERE team_id=? AND user_id=?`, team, a.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := query.List(t.Context(), a, teamFilter); !errors.Is(err, identity.ErrForbidden) {
		t.Fatal("platform role bypassed removed membership")
	}
	if err := tx.Exec(`UPDATE identity.user_session SET revoked_at=? WHERE id=?`, now, a.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	platformFilter.Cursor = platformPage.FirstCursor
	if _, err := query.List(t.Context(), a, platformFilter); !errors.Is(err, identity.ErrSessionInvalid) {
		t.Fatal("revoked session read events")
	}
}
