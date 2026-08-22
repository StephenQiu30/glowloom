package identity

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/stephenqiu30/lanverse/backend/src/platform/database"
)

type contextCaptureStore struct {
	wantWorkspaceID uuid.UUID
	seenAuth        bool
	seenAuthorize   bool
}

func (s *contextCaptureStore) CreateSession(context.Context, string, uuid.UUID) (Session, error) {
	return Session{}, nil
}

func (s *contextCaptureStore) Authenticate(ctx context.Context, _ string, workspaceID uuid.UUID) (Principal, error) {
	got, ok := database.WorkspaceID(ctx)
	if !ok || got != s.wantWorkspaceID || got != workspaceID {
		return Principal{}, context.Canceled
	}
	s.seenAuth = true
	return Principal{UserID: uuid.New(), WorkspaceID: workspaceID, MembershipID: uuid.New(), Role: "owner"}, nil
}

func (s *contextCaptureStore) Revoke(context.Context, string, uuid.UUID) error {
	return nil
}

func (s *contextCaptureStore) AuthorizePath(ctx context.Context, workspaceID uuid.UUID, _ string) error {
	got, ok := database.WorkspaceID(ctx)
	if !ok || got != s.wantWorkspaceID || got != workspaceID {
		return context.Canceled
	}
	s.seenAuthorize = true
	return nil
}

func TestRequirePropagatesWorkspaceContext(t *testing.T) {
	workspaceID := uuid.New()
	store := &contextCaptureStore{wantWorkspaceID: workspaceID}
	service := NewIdentityService(store)
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
	request.Header.Set("Authorization", "Bearer test-token")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
	if !store.seenAuth || !store.seenAuthorize {
		t.Fatalf("identity calls = authenticate:%v authorize:%v, want both true", store.seenAuth, store.seenAuthorize)
	}
}
