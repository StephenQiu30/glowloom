package identity

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/stephenqiu30/lanverse/backend/src/platform/httpapi"
)

type failingIdentityCache struct{}

func (failingIdentityCache) AllowIdentityGCRA(context.Context, string, int64, time.Duration, int64) (bool, time.Duration, int64, error) {
	return false, 0, 0, errors.New("redis unavailable")
}
func (failingIdentityCache) IdentityGet(context.Context, string) (string, bool, error) {
	return "", false, errors.New("redis unavailable")
}
func (failingIdentityCache) IdentitySet(context.Context, string, string, time.Duration) error {
	return errors.New("redis unavailable")
}
func (failingIdentityCache) IdentitySetNX(context.Context, string, string, time.Duration) (bool, error) {
	return false, errors.New("redis unavailable")
}
func (failingIdentityCache) IdentityCompareAndDelete(context.Context, string, string) error {
	return errors.New("redis unavailable")
}

func TestLoginFailsClosedWhenRedisRateLimitIsUnavailable(t *testing.T) {
	store := &contextCaptureStore{}
	manager, err := NewJWTManager(strings.Repeat("s", 32), "test", "test", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	service := NewIdentityService(store, failingIdentityCache{}, manager, AuthConfig{RefreshTTL: time.Hour})
	_, _, err = service.Login(context.Background(), "user@example.com", "a-valid-password", "127.0.0.1")
	apiErr := httpapi.From(err)
	if apiErr.Status != httpapi.StatusServiceUnavailable || apiErr.Code != httpapi.CodeDependencyUnavailable {
		t.Fatalf("error = %#v, want dependency_unavailable/503", apiErr)
	}
	if store.seenAuth {
		t.Fatal("login should not authenticate when Redis is unavailable")
	}
}

func TestRegistrationValidatesEmailAndPasswordBeforePersistence(t *testing.T) {
	workspaceID := uuid.New()
	store := &contextCaptureStore{wantWorkspaceID: workspaceID}
	manager, err := NewJWTManager(strings.Repeat("s", 32), "test", "test", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	service := NewIdentityService(store, allowIdentityCache{}, manager, AuthConfig{RefreshTTL: time.Hour})
	_, _, err = service.Register(context.Background(), RegisterInput{Email: "not-an-email", Password: "short", Workspace: "Lanverse"}, "127.0.0.1")
	apiErr := httpapi.From(err)
	if apiErr.Status != httpapi.StatusUnprocessableEntity || apiErr.Code != httpapi.CodeValidationFailed {
		t.Fatalf("error = %#v, want validation_failed/422", apiErr)
	}
}

func TestWorkspaceMemberManagementRequiresAdministrator(t *testing.T) {
	store := &contextCaptureStore{}
	manager, err := NewJWTManager(strings.Repeat("s", 32), "test", "test", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	service := NewIdentityService(store, allowIdentityCache{}, manager, AuthConfig{RefreshTTL: time.Hour})
	_, err = service.ListWorkspaceMembers(context.Background(), Principal{WorkspaceID: uuid.New(), Role: RoleUser}, WorkspaceMemberQuery{})
	apiErr := httpapi.From(err)
	if apiErr.Status != httpapi.StatusForbidden || apiErr.Code != httpapi.CodeForbidden {
		t.Fatalf("error = %#v, want forbidden", apiErr)
	}
}

func TestUserCannotGrantAdminRole(t *testing.T) {
	store := &contextCaptureStore{}
	manager, err := NewJWTManager(strings.Repeat("s", 32), "test", "test", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	service := NewIdentityService(store, allowIdentityCache{}, manager, AuthConfig{RefreshTTL: time.Hour})
	admin := RoleAdmin
	_, err = service.UpdateWorkspaceMember(context.Background(), Principal{WorkspaceID: uuid.New(), MembershipID: uuid.New(), Role: RoleUser}, uuid.New(), WorkspaceMemberUpdate{Role: &admin})
	apiErr := httpapi.From(err)
	if apiErr.Status != httpapi.StatusForbidden || apiErr.Code != httpapi.CodeForbidden {
		t.Fatalf("error = %#v, want forbidden", apiErr)
	}
}
