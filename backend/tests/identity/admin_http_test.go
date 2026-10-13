package identity_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityhttp "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/http"
	"github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
)

type adminListStore struct {
	calls      int
	actor, org uuid.UUID
}

func (s *adminListStore) ListForAdmin(_ context.Context, actor, org uuid.UUID, _ int, _ *application.UserListCursor) (application.UserListPage, error) {
	s.calls++
	s.actor = actor
	s.org = org
	return application.UserListPage{Users: []application.UserListItem{{ID: uuid.New(), LoginName: "creator.one", DisplayName: "创作者", Role: domain.RoleProducer, Status: domain.StatusActive, Revision: 1}}, Next: &application.UserListCursor{ID: uuid.New(), CreateTime: time.Now()}}, nil
}
func TestAdminListScopeAndCursor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &adminListStore{}
	actor := application.Principal{ID: uuid.New(), OrgID: uuid.New(), Role: domain.RoleAdmin}
	router := gin.New()
	router.Use(httpapi.Middleware("https://local.test"))
	api := router.Group("/api")
	api.Use(func(c *gin.Context) { c.Set("principal", actor) })
	identityhttp.NewAdminHandler(identityhttp.AdminDependencies{List: application.NewListUsersQuery(store)}).Register(api)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), "GET", "/api/admin/users?limit=10", nil))
	if w.Code != 200 || store.actor != actor.ID || store.org != actor.OrgID {
		t.Fatalf("list status=%d, scoped=%t", w.Code, store.org == actor.OrgID)
	}
	var page identityhttp.AdminUserPage
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Role != "creator" || page.NextCursor == "" {
		t.Fatalf("unexpected safe page: %+v", page)
	}
	actor.OrgID = uuid.New()
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), "GET", "/api/admin/users?limit=10&cursor="+page.NextCursor, nil))
	if w.Code != 422 || store.calls != 1 {
		t.Fatalf("cross-org cursor accepted: %d", w.Code)
	}
	actor.Role = domain.RoleProducer
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), "GET", "/api/admin/users", nil))
	if w.Code != 403 || store.calls != 1 {
		t.Fatalf("creator reached directory: %d", w.Code)
	}
}
func TestAdminWriteRejectsUnknownFieldsAndSelfDisable(t *testing.T) {
	actor := application.Principal{ID: uuid.New(), OrgID: uuid.New(), Role: domain.RoleAdmin}
	r := gin.New()
	r.Use(httpapi.Middleware("https://local.test"))
	g := r.Group("/api")
	g.Use(func(c *gin.Context) { c.Set("principal", actor) })
	identityhttp.NewAdminHandler(identityhttp.AdminDependencies{}).Register(g)
	for _, tc := range []struct {
		path, body string
		status     int
	}{{"/api/admin/users", `{"login_name":"valid","role":"admin","org_id":"injected"}`, 422}, {"/api/admin/users/" + actor.ID.String() + "/disable", `{"expected_revision":1}`, 409}} {
		w := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), "POST", tc.path, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "https://local.test")
		req.Header.Set("Idempotency-Key", uuid.NewString())
		r.ServeHTTP(w, req)
		if w.Code != tc.status {
			t.Errorf("%s: got %d want %d", tc.path, w.Code, tc.status)
		}
	}
}

type adminCreateStore struct {
	actor  uuid.UUID
	user   domain.User
	events []application.OutboxEvent
	err    error
}

func (s *adminCreateStore) CreateWithEvents(_ context.Context, actor uuid.UUID, u domain.User, events []application.OutboxEvent) (domain.User, error) {
	s.actor = actor
	s.user = u
	s.events = events
	u.CreateTime = time.Now()
	return u, s.err
}
func TestAdminCreateUsesAuditedCommandWithoutReturningSecrets(t *testing.T) {
	actor := application.Principal{ID: uuid.New(), OrgID: uuid.New(), Role: domain.RoleAdmin}
	store := &adminCreateStore{}
	r := gin.New()
	r.Use(httpapi.Middleware("https://local.test"))
	g := r.Group("/api")
	g.Use(func(c *gin.Context) { c.Set("principal", actor) })
	identityhttp.NewAdminHandler(identityhttp.AdminDependencies{Create: application.NewCreateUserCommand(store, time.Now)}).Register(g)
	call := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), "POST", "/api/admin/users", strings.NewReader(`{"login_name":"Creator.One","display_name":"创作者","role":"creator","initial_password":"Validation1234"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "https://local.test")
		req.Header.Set("Idempotency-Key", uuid.NewString())
		r.ServeHTTP(w, req)
		return w
	}
	w := call()
	if w.Code != 201 {
		t.Fatalf("create status=%d", w.Code)
	}
	if store.actor != actor.ID || store.user.OrgID != actor.OrgID || store.user.Role != domain.RoleProducer || store.user.LoginName != "creator.one" || !store.user.MustChangePassword || len(store.events) != 2 {
		t.Fatal("create did not use the scoped, audited command")
	}
	if store.user.PasswordHash == "Validation1234" || store.user.PasswordHash == "" {
		t.Fatal("password was not hashed")
	}
	if strings.Contains(w.Body.String(), "password_hash") || strings.Contains(w.Body.String(), "Validation1234") || strings.Contains(w.Body.String(), "org_id") {
		t.Fatal("create returned secret or organization fields")
	}
	store.err = application.ErrLoginTaken
	w = call()
	if w.Code != 422 || !strings.Contains(w.Body.String(), "login_name_taken") {
		t.Fatalf("duplicate login status=%d", w.Code)
	}
}

type adminDisableStore struct {
	actor, org, target uuid.UUID
	revision           int64
	err                error
}

func (s *adminDisableStore) DisableWithEvents(_ context.Context, actor, org, target uuid.UUID, revision int64, _ []application.OutboxEvent) (domain.User, error) {
	s.actor = actor
	s.org = org
	s.target = target
	s.revision = revision
	return domain.User{ID: target, OrgID: org, Revision: revision + 1, Status: domain.StatusDisabled}, s.err
}
func TestAdminDisablePreservesCASAndLastAdministratorGuard(t *testing.T) {
	actor := application.Principal{ID: uuid.New(), OrgID: uuid.New(), Role: domain.RoleAdmin}
	target := uuid.New()
	store := &adminDisableStore{}
	r := gin.New()
	r.Use(httpapi.Middleware("https://local.test"))
	g := r.Group("/api")
	g.Use(func(c *gin.Context) { c.Set("principal", actor) })
	identityhttp.NewAdminHandler(identityhttp.AdminDependencies{Disable: application.NewDisableUserCommand(store, time.Now)}).Register(g)
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{{nil, 200, ""}, {domain.ErrRevisionConflict, 409, "revision_conflict"}, {domain.ErrLastActiveAdmin, 409, "last_active_admin"}, {application.ErrForbidden, 403, "forbidden"}} {
		store.err = tc.err
		w := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), "POST", "/api/admin/users/"+target.String()+"/disable", strings.NewReader(`{"expected_revision":3}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "https://local.test")
		req.Header.Set("Idempotency-Key", uuid.NewString())
		r.ServeHTTP(w, req)
		if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.code) {
			t.Errorf("status=%d want %d", w.Code, tc.status)
		}
		if store.revision != 3 || store.org != actor.OrgID || store.actor != actor.ID || store.target != target {
			t.Fatal("transport changed CAS or identity scope")
		}
	}
}
