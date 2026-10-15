package identity_test

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	workspacehttp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/http"
	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/postgres"
	workspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

func TestPlatformTeamDirectoryRequiredFilters(t *testing.T) {
	tx, _, _, now := governanceFixture(t)
	_, owner := verifiedTestActor(t, tx, "filter-owner@example.invalid", now)
	_, other := verifiedTestActor(t, tx, "filter-other@example.invalid", now)
	if err := tx.Exec(`UPDATE identity.platform_settings SET team_creation_enabled=true`).Error; err != nil {
		t.Fatal(err)
	}
	store := pgworkspace.NewTeamStore(tx, func(tx *gorm.DB) workspace.TeamActorAccess {
		return pgidentity.NewTeamActorStore(tx, completionProtector(t))
	})
	teams := workspace.NewTeams(store, func() time.Time { return now })
	ids := make([]uuid.UUID, 3)
	for i, a := range []identity.Principal{owner, owner, other} {
		team, err := teams.Create(t.Context(), a, uuid.New(), "Required filter team")
		if err != nil {
			t.Fatal(err)
		}
		ids[i] = team.ID
		if err := tx.Exec(`UPDATE workspace.team SET create_time=? WHERE id=?`, now.Add(time.Duration(i-3)*time.Hour), team.ID).Error; err != nil {
			t.Fatal(err)
		}
	}
	service := workspace.NewPlatformTeams(store, completionProtector(t), func() time.Time { return now })
	auth := identity.NewEmailAuth(pgidentity.NewStore(tx), completionProtector(t), identity.EmailPolicy{TermsVersion: "test", SourceHourlyLimit: 100, GlobalHourlyLimit: 1000}, func() time.Time { return now })
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := identityhttp.NewEmailAuthHandler(auth, "http://localhost:3200", false)
	handler.Register(router.Group("/api"))
	workspacehttp.NewPlatformTeamHandler(service).Register(router.Group("/api", handler.RequireSession()))
	login := httptest.NewRequestWithContext(t.Context(), "POST", "/api/sessions", strings.NewReader(`{"email":"governance-admin@example.invalid","password":"long password for tests"}`))
	login.Header.Set("Origin", "http://localhost:3200")
	login.Header.Set("Content-Type", "application/json")
	login.Header.Set("Idempotency-Key", uuid.NewString())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, login)
	if w.Code != 201 {
		t.Fatal("login failed", w.Code)
	}
	cookie := w.Result().Cookies()[0]
	request := func(query string, status int) workspace.PlatformTeamPage {
		t.Helper()
		req := httptest.NewRequestWithContext(t.Context(), "GET", "/api/platform/teams?"+query, nil)
		req.AddCookie(cookie)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != status {
			t.Fatalf("query %s: status=%d want=%d", query, response.Code, status)
		}
		var page workspace.PlatformTeamPage
		if status == 200 {
			if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
				t.Fatal(err)
			}
		}
		return page
	}
	page := request("id="+ids[0].String(), 200)
	if len(page.Items) != 1 || page.Items[0].ID != ids[0] {
		t.Fatal("exact team ID filter failed")
	}
	if page := request("id="+uuid.NewString(), 200); len(page.Items) != 0 {
		t.Fatal("unknown team ID matched")
	}
	from, to := now.Add(-3*time.Hour), now.Add(-time.Hour)
	q := url.Values{"search": {"Required filter"}, "administrator_id": {owner.ID.String()}, "created_from": {from.Format(time.RFC3339Nano)}, "created_to": {to.Format(time.RFC3339Nano)}, "limit": {"1"}}
	first := request(q.Encode(), 200)
	if len(first.Items) != 1 || first.Items[0].ID != ids[1] || first.NextCursor == nil {
		t.Fatal("combined filters or continuation failed")
	}
	q.Set("cursor", *first.NextCursor)
	// Equivalent RFC3339 offsets must retain the same signed filter identity.
	q.Set("created_from", from.In(time.FixedZone("offset", 8*3600)).Format(time.RFC3339Nano))
	q.Set("administrator_id", strings.ToUpper(owner.ID.String()))
	second := request(q.Encode(), 200)
	if len(second.Items) != 1 || second.Items[0].ID != ids[0] || second.NextCursor != nil {
		t.Fatal("half-open interval or normalized cursor failed")
	}
	for key, value := range map[string]string{"id": ids[0].String(), "administrator_id": other.ID.String(), "created_from": from.Add(-time.Hour).Format(time.RFC3339Nano), "created_to": to.Add(time.Hour).Format(time.RFC3339Nano)} {
		changed := url.Values{}
		for k, v := range q {
			changed[k] = append([]string(nil), v...)
		}
		changed.Set(key, value)
		request(changed.Encode(), 422)
	}
	// A current administrator is not the historical creator or a suspended member.
	for _, change := range []string{"UPDATE workspace.team_member SET role='member' WHERE user_id=?", "UPDATE workspace.team_member SET status='suspended' WHERE user_id=?", "UPDATE identity.\"user\" SET status='disabled' WHERE id=?", "UPDATE identity.\"user\" SET is_delete=true WHERE id=?", "UPDATE identity.\"user\" SET email_verified_at=NULL,email_key=NULL WHERE id=?"} {
		if err := tx.Exec(change, owner.ID).Error; err != nil {
			t.Fatal(err)
		}
		if page := request("administrator_id="+owner.ID.String(), 200); len(page.Items) != 0 {
			t.Fatal("inactive administrator matched")
		}
		if err := tx.Exec(`UPDATE workspace.team_member SET status='active',role='team_admin' WHERE user_id=?`, owner.ID).Error; err != nil {
			t.Fatal(err)
		}
		if err := tx.Exec(`UPDATE identity."user" SET status='active',is_delete=false,email_key='filter-owner@example.invalid',email_verified_at=? WHERE id=?`, now, owner.ID).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Exec(`UPDATE identity."user" SET must_change_password=true WHERE id=?`, owner.ID).Error; err != nil {
		t.Fatal(err)
	}
	if page := request("administrator_id="+owner.ID.String(), 200); len(page.Items) != 2 {
		t.Fatal("password change requirement removed administrator")
	}
	for _, query := range []string{"id=invalid", "id=" + uuid.Nil.String(), "administrator_id=invalid", "created_from=2026-10-15", "created_to=invalid", "created_from=2026-10-15T12:00:00Z&created_to=2026-10-15T12:00:00Z", "created_from=2026-10-16T12:00:00Z&created_to=2026-10-15T12:00:00Z", "id=" + ids[0].String() + "&id=" + ids[1].String(), "administrator_id=" + owner.ID.String() + "&administrator_id=" + other.ID.String(), "created_from=2026-10-15T12:00:00Z&created_from=2026-10-15T12:00:00Z", "created_to=", "unknown_filter=x"} {
		request(query, 422)
	}
}
