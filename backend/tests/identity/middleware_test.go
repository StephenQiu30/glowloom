package identity_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	. "github.com/stephenqiu30/lanverse/backend/src/identity"
	"github.com/stephenqiu30/lanverse/backend/src/platform/database"
	"github.com/stephenqiu30/lanverse/backend/src/platform/httpapi"
)

type contextCaptureStore struct {
	wantWorkspaceID uuid.UUID
	principalRole   RoleCode
	seenAuth        bool
	seenAuthorize   bool
	authorizeErr    error
	memberPage      WorkspaceMemberPage
	updatedMember   WorkspaceMember
	memberUpdate    WorkspaceMemberUpdate
	auditPage       AccessAuditPage
	auditQuery      AccessAuditQuery
}

func (s *contextCaptureStore) RegisterAccount(context.Context, PersistedRegisterInput) (SessionIssue, error) {
	return SessionIssue{}, nil
}

func (s *contextCaptureStore) FindLoginAccount(context.Context, EmailAddress) (LoginAccount, error) {
	return LoginAccount{}, nil
}

func (s *contextCaptureStore) CreateSession(context.Context, AuthIdentity) (SessionIssue, error) {
	return SessionIssue{}, nil
}

func (s *contextCaptureStore) RotateRefreshSession(context.Context, string) (SessionIssue, error) {
	return SessionIssue{}, nil
}

func (s *contextCaptureStore) RevokeRefreshSession(context.Context, string) (uuid.UUID, error) {
	return uuid.Nil, nil
}

func (s *contextCaptureStore) Authenticate(ctx context.Context, userID, sessionID, workspaceID uuid.UUID) (Principal, error) {
	got, ok := database.WorkspaceID(ctx)
	if !ok || got != s.wantWorkspaceID || got != workspaceID || userID == uuid.Nil || sessionID == uuid.Nil {
		return Principal{}, context.Canceled
	}
	s.seenAuth = true
	role := s.principalRole
	if role == "" {
		role = RoleAdmin
	}
	return Principal{UserID: userID, WorkspaceID: workspaceID, MembershipID: uuid.New(), SessionID: sessionID, Role: role}, nil
}

func (s *contextCaptureStore) AuthorizePath(ctx context.Context, workspaceID uuid.UUID, _ string) error {
	got, ok := database.WorkspaceID(ctx)
	if !ok || got != s.wantWorkspaceID || got != workspaceID {
		return context.Canceled
	}
	s.seenAuthorize = true
	return s.authorizeErr
}

func (s *contextCaptureStore) ListWorkspaceMembers(context.Context, uuid.UUID, WorkspaceMemberQuery) (WorkspaceMemberPage, error) {
	return s.memberPage, nil
}

func (s *contextCaptureStore) ListAccessAudit(_ context.Context, _ uuid.UUID, query AccessAuditQuery) (AccessAuditPage, error) {
	s.auditQuery = query
	return s.auditPage, nil
}

func (s *contextCaptureStore) UpdateWorkspaceMember(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ Principal, input WorkspaceMemberUpdate) (WorkspaceMember, error) {
	s.memberUpdate = input
	return s.updatedMember, nil
}

type allowIdentityCache struct{}

func (allowIdentityCache) AllowIdentityGCRA(context.Context, string, int64, time.Duration, int64) (bool, time.Duration, int64, error) {
	return true, 0, 1, nil
}
func (allowIdentityCache) IdentityGet(context.Context, string) (string, bool, error) {
	return "", false, nil
}
func (allowIdentityCache) IdentitySet(context.Context, string, string, time.Duration) error {
	return nil
}
func (allowIdentityCache) IdentitySetNX(context.Context, string, string, time.Duration) (bool, error) {
	return true, nil
}
func (allowIdentityCache) IdentityCompareAndDelete(context.Context, string, string) error { return nil }

func TestRequirePropagatesWorkspaceContext(t *testing.T) {
	workspaceID := uuid.New()
	userID := uuid.New()
	sessionID := uuid.New()
	store := &contextCaptureStore{wantWorkspaceID: workspaceID}
	jwtManager, err := NewJWTManager(strings.Repeat("a", 32), "test", "test", 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	service := NewIdentityService(store, allowIdentityCache{}, jwtManager, AuthConfig{RefreshTTL: time.Hour})
	accessToken, _, err := jwtManager.Issue(SessionIssue{SessionID: sessionID, Identity: AuthIdentity{Account: Account{ID: userID}, Workspace: Workspace{ID: workspaceID}, Role: RoleAdmin}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, ok := database.WorkspaceID(r.Context())
		if !ok || got != workspaceID {
			t.Fatalf("handler workspace context = %s, %v; want %s, true", got, ok, workspaceID)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	handler := Require(service, next)
	request := httptest.NewRequest(http.MethodGet, "/api/projects/"+uuid.NewString()+"/analysis", nil)
	request.Header.Set("Authorization", "Bearer "+accessToken)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusNoContent, recorder.Body.String())
	}
	if !store.seenAuth || !store.seenAuthorize {
		t.Fatalf("identity calls = authenticate:%v, authorize:%v, want both true", store.seenAuth, store.seenAuthorize)
	}
}

func TestRequireAdminRejectsNonAdministrator(t *testing.T) {
	workspaceID := uuid.New()
	store := &contextCaptureStore{wantWorkspaceID: workspaceID, principalRole: RoleUser}
	jwtManager, err := NewJWTManager(strings.Repeat("a", 32), "test", "test", 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	service := NewIdentityService(store, allowIdentityCache{}, jwtManager, AuthConfig{RefreshTTL: time.Hour})
	accessToken, _, err := jwtManager.Issue(SessionIssue{
		SessionID: uuid.New(),
		Identity: AuthIdentity{
			Account:   Account{ID: uuid.New()},
			Workspace: Workspace{ID: workspaceID},
			Role:      RoleUser,
		},
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := Require(service, RequireAdmin(next))
	request := httptest.NewRequest(http.MethodGet, "/api/admin/members", nil)
	request.Header.Set("Authorization", "Bearer "+accessToken)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
}

func TestAdminMemberUpdatePropagatesReasonAndServerRequestID(t *testing.T) {
	workspaceID, userID, sessionID, targetMembershipID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	store := &contextCaptureStore{
		wantWorkspaceID: workspaceID,
		updatedMember: WorkspaceMember{
			MembershipID:     targetMembershipID,
			UserID:           uuid.New(),
			Role:             RoleBan,
			MembershipStatus: MembershipStatusActive,
		},
	}
	jwtManager, err := NewJWTManager(strings.Repeat("a", 32), "test", "test", 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	service := NewIdentityService(store, allowIdentityCache{}, jwtManager, AuthConfig{RefreshTTL: time.Hour})
	accessToken, _, err := jwtManager.Issue(SessionIssue{
		SessionID: sessionID,
		Identity: AuthIdentity{
			Account:   Account{ID: userID},
			Workspace: Workspace{ID: workspaceID},
			Role:      RoleAdmin,
		},
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	router := chi.NewRouter()
	NewIdentityAdminController(service).Mount(router)
	handler := httpapi.RequestIDMiddleware(Require(service, router))
	request := httptest.NewRequest(http.MethodPatch, "/api/admin/members/"+targetMembershipID.String(), bytes.NewBufferString(`{"role":"ban","reason":"违规访问处置"}`))
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Request-Id", "member-audit-request")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if store.memberUpdate.Reason != "违规访问处置" || store.memberUpdate.RequestID != "member-audit-request" {
		t.Fatalf("member update = %#v", store.memberUpdate)
	}
}

func TestAdminAccessAuditPropagatesScopedFiltersAndPagination(t *testing.T) {
	workspaceID, userID, sessionID := uuid.New(), uuid.New(), uuid.New()
	store := &contextCaptureStore{
		wantWorkspaceID: workspaceID,
		auditPage: AccessAuditPage{
			Items: []AccessAuditEvent{{WorkspaceID: workspaceID, Action: "iam.membership.updated", Result: "succeeded"}},
			Total: 1, Page: 2, PageSize: 10,
		},
	}
	jwtManager, err := NewJWTManager(strings.Repeat("a", 32), "test", "test", 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	service := NewIdentityService(store, allowIdentityCache{}, jwtManager, AuthConfig{RefreshTTL: time.Hour})
	accessToken, _, err := jwtManager.Issue(SessionIssue{
		SessionID: sessionID,
		Identity: AuthIdentity{
			Account: Account{ID: userID}, Workspace: Workspace{ID: workspaceID}, Role: RoleAdmin,
		},
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	router := chi.NewRouter()
	NewIdentityAdminController(service).Mount(router)
	handler := Require(service, router)
	request := httptest.NewRequest(http.MethodGet, "/api/admin/audit-events?actor=Audit+Actor&object=Audit+Target&action=iam.membership.updated&result=succeeded&occurred_from=2026-08-23T00%3A00%3A00Z&occurred_to=2026-08-24T00%3A00%3A00Z&page=2&page_size=10", nil)
	request.Header.Set("Authorization", "Bearer "+accessToken)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if store.auditQuery.Actor != "Audit Actor" || store.auditQuery.Object != "Audit Target" ||
		store.auditQuery.Action != "iam.membership.updated" || store.auditQuery.Result != "succeeded" ||
		store.auditQuery.OccurredFrom == nil || store.auditQuery.OccurredTo == nil ||
		store.auditQuery.Page != 2 || store.auditQuery.PageSize != 10 {
		t.Fatalf("audit query = %#v", store.auditQuery)
	}
}

func TestRequireDoesNotHideAuthorizationRepositoryFailureAsNotFound(t *testing.T) {
	workspaceID := uuid.New()
	store := &contextCaptureStore{wantWorkspaceID: workspaceID, authorizeErr: context.DeadlineExceeded}
	jwtManager, err := NewJWTManager(strings.Repeat("a", 32), "test", "test", 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	service := NewIdentityService(store, allowIdentityCache{}, jwtManager, AuthConfig{RefreshTTL: time.Hour})
	accessToken, _, err := jwtManager.Issue(SessionIssue{
		SessionID: uuid.New(),
		Identity: AuthIdentity{
			Account:   Account{ID: uuid.New()},
			Workspace: Workspace{ID: workspaceID},
			Role:      RoleAdmin,
		},
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	handler := Require(service, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("next handler must not run")
	}))
	request := httptest.NewRequest(http.MethodGet, "/api/projects/"+uuid.NewString(), nil)
	request.Header.Set("Authorization", "Bearer "+accessToken)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusInternalServerError, recorder.Body.String())
	}
}
