package identity_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
)

func TestUpdateProfileProtectsAccountInvariants(t *testing.T) {
	user := domain.User{DisplayName: "Old", Role: domain.RoleAdmin, Status: domain.StatusActive, Revision: 1, SessionEpoch: 4}
	producer := domain.RoleProducer
	if err := user.UpdateProfile(nil, &producer, 1); !errors.Is(err, domain.ErrLastActiveAdmin) {
		t.Fatalf("demote last administrator: %v", err)
	}
	if user.Role != domain.RoleAdmin || user.Revision != 1 {
		t.Fatal("rejected role change mutated account")
	}
	if err := user.UpdateProfile(nil, &producer, 2); err != nil || user.Role != producer || user.Revision != 2 || user.SessionEpoch != 4 {
		t.Fatalf("role change: %+v, %v", user, err)
	}
	name := "  张三  "
	if err := user.UpdateProfile(&name, nil, 1); err != nil || user.DisplayName != "张三" || user.Revision != 3 {
		t.Fatalf("display name change: %+v, %v", user, err)
	}
	if err := user.UpdateProfile(&name, nil, 1); !errors.Is(err, domain.ErrAccountUnchanged) || user.Revision != 3 {
		t.Fatalf("unchanged profile: %v, revision %d", err, user.Revision)
	}
	for _, invalid := range []string{" ", strings.Repeat("字", 51), "a\x00b"} {
		if err := user.UpdateProfile(&invalid, nil, 1); !errors.Is(err, domain.ErrInvalidProfile) {
			t.Fatalf("accepted invalid display name %q: %v", invalid, err)
		}
	}
	invalidRole := domain.Role("owner")
	if err := user.UpdateProfile(nil, &invalidRole, 1); !errors.Is(err, domain.ErrInvalidProfile) {
		t.Fatalf("accepted invalid role: %v", err)
	}
	if user.DisplayName != "张三" || user.Revision != 3 || user.SessionEpoch != 4 {
		t.Fatal("invalid profile mutated account")
	}
}

type directoryStoreStub struct {
	user   domain.User
	page   identityapp.UserListPage
	events []identityapp.OutboxEvent
	writes int
	err    error
}

func (s *directoryStoreStub) FindByID(_ context.Context, _, _ uuid.UUID) (domain.User, error) {
	return s.user, s.err
}

func (s *directoryStoreStub) UpdateProfileWithEvents(_ context.Context, _ uuid.UUID, _, after domain.User, events []identityapp.OutboxEvent) (domain.User, error) {
	s.events = events
	s.writes++
	return after, s.err
}

func (s *directoryStoreStub) ListForAdmin(_ context.Context, _, _ uuid.UUID, _ int, _ *identityapp.UserListCursor) (identityapp.UserListPage, error) {
	return s.page, s.err
}

func TestUpdateUserCommandWritesSafeChangeAndPreservesConflict(t *testing.T) {
	orgID, actorID, targetID := uuid.New(), uuid.New(), uuid.New()
	actor := identityapp.Principal{ID: actorID, OrgID: orgID, Role: domain.RoleAdmin}
	store := &directoryStoreStub{user: domain.User{ID: targetID, OrgID: orgID, DisplayName: "Before",
		Role: domain.RoleProducer, Status: domain.StatusActive, PasswordHash: "private-hash", Revision: 1, SessionEpoch: 1}}
	name := "After"
	result, err := identityapp.NewUpdateUserCommand(store, time.Now).Execute(t.Context(), actor,
		identityapp.UpdateUserInput{TargetID: targetID, ExpectedRevision: 1, DisplayName: &name, RequestID: "update-1"})
	if err != nil || result.ID != targetID || result.DisplayName != name || result.Revision != 2 || store.writes != 1 {
		t.Fatalf("updated account: %+v, writes %d, error %v", result, store.writes, err)
	}
	checkLifecycleAudit(t, store.events, orgID, actorID, targetID, "updated", "user.updated", "update-1")
	for _, event := range store.events {
		if containsSecret(event.Payload, "private-hash") {
			t.Fatal("profile event exposed password hash")
		}
	}
	if _, err := identityapp.NewUpdateUserCommand(store, time.Now).Execute(t.Context(), actor,
		identityapp.UpdateUserInput{TargetID: targetID, ExpectedRevision: 2, DisplayName: &name, RequestID: "stale"}); !errors.Is(err, domain.ErrRevisionConflict) {
		t.Fatalf("stale revision: %v", err)
	}
	store.user.DisplayName = name
	store.user.Revision = 2
	if _, err := identityapp.NewUpdateUserCommand(store, time.Now).Execute(t.Context(), actor,
		identityapp.UpdateUserInput{TargetID: targetID, ExpectedRevision: 2, DisplayName: &name, RequestID: "same"}); !errors.Is(err, domain.ErrAccountUnchanged) || store.writes != 1 {
		t.Fatalf("unchanged profile: %v, writes %d", err, store.writes)
	}
	producer := identityapp.Principal{ID: actorID, OrgID: orgID, Role: domain.RoleProducer}
	if _, err := identityapp.NewUpdateUserCommand(store, time.Now).Execute(t.Context(), producer,
		identityapp.UpdateUserInput{TargetID: targetID, ExpectedRevision: 2, DisplayName: &name, RequestID: "deny"}); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatalf("producer update: %v", err)
	}
}

func TestListUsersQueryChecksAdministratorAndPageLimit(t *testing.T) {
	orgID, actorID := uuid.New(), uuid.New()
	store := &directoryStoreStub{page: identityapp.UserListPage{Users: []identityapp.UserListItem{{ID: actorID, LoginName: "admin"}}}}
	query := identityapp.NewListUsersQuery(store)
	actor := identityapp.Principal{ID: actorID, OrgID: orgID, Role: domain.RoleAdmin}
	page, err := query.Execute(t.Context(), actor, identityapp.ListUsersInput{})
	if err != nil || len(page.Users) != 1 || page.Users[0].LoginName != "admin" {
		t.Fatalf("list page: %+v, %v", page, err)
	}
	if _, err := query.Execute(t.Context(), actor, identityapp.ListUsersInput{Limit: 201}); !errors.Is(err, identityapp.ErrInvalidUserList) {
		t.Fatalf("oversized page: %v", err)
	}
	actor.MustChangePassword = true
	if _, err := query.Execute(t.Context(), actor, identityapp.ListUsersInput{}); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatalf("first-login administrator list: %v", err)
	}
}
