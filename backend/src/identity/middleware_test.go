package identity

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/stephenqiu30/lanverse/backend/src/platform/database"
)

type contextCaptureStore struct {
	wantWorkspaceID uuid.UUID
	seenAuth        bool
	seenAuthorize   bool
}

func (s *contextCaptureStore) RegisterAccount(context.Context, PersistedRegisterInput) (SessionIssue, error) {
	return SessionIssue{}, nil
}

func (s *contextCaptureStore) FindLoginAccount(context.Context, EmailAddress, uuid.UUID) (LoginAccount, error) {
	return LoginAccount{}, nil
}

func (s *contextCaptureStore) CreateSession(context.Context, AuthIdentity) (SessionIssue, error) {
	return SessionIssue{}, nil
}

func (s *contextCaptureStore) RotateRefreshSession(context.Context, uuid.UUID, string) (SessionIssue, error) {
	return SessionIssue{}, nil
}

func (s *contextCaptureStore) RevokeRefreshSession(context.Context, uuid.UUID, string) (uuid.UUID, error) {
	return uuid.Nil, nil
}

func (s *contextCaptureStore) Authenticate(ctx context.Context, userID, sessionID, workspaceID uuid.UUID) (Principal, error) {
	got, ok := database.WorkspaceID(ctx)
	if !ok || got != s.wantWorkspaceID || got != workspaceID || userID == uuid.Nil || sessionID == uuid.Nil {
		return Principal{}, context.Canceled
	}
	s.seenAuth = true
	return Principal{UserID: userID, WorkspaceID: workspaceID, MembershipID: uuid.New(), SessionID: sessionID, Role: RoleOwner}, nil
}

func (s *contextCaptureStore) AuthorizePath(ctx context.Context, workspaceID uuid.UUID, _ string) error {
	got, ok := database.WorkspaceID(ctx)
	if !ok || got != s.wantWorkspaceID || got != workspaceID {
		return context.Canceled
	}
	s.seenAuthorize = true
	return nil
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
	accessToken, _, err := jwtManager.Issue(SessionIssue{SessionID: sessionID, Identity: AuthIdentity{Account: Account{ID: userID}, Workspace: Workspace{ID: workspaceID}, Role: RoleOwner}}, time.Now())
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
	request.Header.Set("X-Workspace-Id", workspaceID.String())
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
