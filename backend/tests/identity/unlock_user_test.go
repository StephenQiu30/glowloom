package identity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

type unlockStore struct {
	calls  int
	err    error
	events []application.OutboxEvent
}

func (s *unlockStore) UnlockWithEvents(_ context.Context, _, org, target uuid.UUID, rev int64, _ time.Time, events []application.OutboxEvent) (domain.User, error) {
	s.calls++
	s.events = events
	return domain.User{ID: target, OrgID: org, Status: domain.StatusDisabled, Revision: rev + 1}, s.err
}
func TestUnlockPreservesDisabledStatusAndChecksAuthorization(t *testing.T) {
	s := &unlockStore{}
	actor := application.Principal{ID: uuid.New(), OrgID: uuid.New(), Role: domain.RoleAdmin}
	input := application.UnlockUserInput{TargetID: uuid.New(), ExpectedRevision: 3, RequestID: "unlock-test"}
	command := application.NewUnlockUserCommand(s, time.Now)
	result, err := command.Execute(t.Context(), actor, input)
	if err != nil || result.Status != domain.StatusDisabled || result.Revision != 4 || len(s.events) != 2 {
		t.Fatalf("unlock receipt/events: %+v %v", result, err)
	}
	input.TargetID = actor.ID
	if _, err := command.Execute(t.Context(), actor, input); !errors.Is(err, application.ErrForbidden) || s.calls != 1 {
		t.Fatal("self unlock allowed")
	}
	input.TargetID = uuid.New()
	actor.Role = domain.RoleProducer
	if _, err := command.Execute(t.Context(), actor, input); !errors.Is(err, application.ErrForbidden) || s.calls != 1 {
		t.Fatal("non-admin allowed")
	}
	actor.Role = domain.RoleAdmin
	s.err = domain.ErrRevisionConflict
	if _, err := command.Execute(t.Context(), actor, input); !errors.Is(err, domain.ErrRevisionConflict) {
		t.Fatalf("lost conflict: %v", err)
	}
}
