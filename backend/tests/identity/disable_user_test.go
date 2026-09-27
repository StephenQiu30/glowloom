package identity_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	auditapp "github.com/StephenQiu30/lanverse/backend/internal/audit/application"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
)

type disableUserStoreStub struct {
	actorID  uuid.UUID
	orgID    uuid.UUID
	targetID uuid.UUID
	revision int64
	events   []identityapp.OutboxEvent
	result   domain.User
	err      error
	calls    int
}

func (s *disableUserStoreStub) DisableWithEvents(_ context.Context, actorID, orgID, targetID uuid.UUID, revision int64, events []identityapp.OutboxEvent) (domain.User, error) {
	s.calls++
	s.actorID, s.orgID, s.targetID, s.revision, s.events = actorID, orgID, targetID, revision, events
	return s.result, s.err
}

func TestDisableUserCommandBuildsAuditedAccountChange(t *testing.T) {
	orgID, actorID, targetID := uuid.New(), uuid.New(), uuid.New()
	store := &disableUserStoreStub{result: domain.User{
		ID: targetID, OrgID: orgID, Status: domain.StatusDisabled, SessionEpoch: 2, Revision: 2,
	}}
	actor := identityapp.Principal{ID: actorID, OrgID: orgID, Role: domain.RoleAdmin}
	command := identityapp.NewDisableUserCommand(store, time.Now)
	result, err := command.Execute(t.Context(), actor, identityapp.DisableUserInput{
		TargetID: targetID, ExpectedRevision: 1, RequestID: "disable-request",
	})
	if err != nil || result.ID != targetID || result.Status != domain.StatusDisabled || result.Revision != 2 ||
		store.calls != 1 || store.actorID != actorID || store.orgID != orgID || store.targetID != targetID || store.revision != 1 || len(store.events) != 2 {
		t.Fatalf("disable result = %+v, store = %+v, error %v", result, store, err)
	}
	var changed struct {
		Actor struct {
			ID uuid.UUID `json:"id"`
		} `json:"actor"`
		Aggregate struct {
			ID       uuid.UUID `json:"id"`
			Revision int64     `json:"revision"`
		} `json:"aggregate"`
		Data struct {
			Change string `json:"change"`
		} `json:"data"`
	}
	if store.events[0].Topic != "lanverse.identity.user_changed.v1" ||
		store.events[0].PartitionKey != orgID.String() ||
		json.Unmarshal(store.events[0].Payload, &changed) != nil ||
		changed.Actor.ID != actorID || changed.Aggregate.ID != targetID ||
		changed.Aggregate.Revision != 2 || changed.Data.Change != "disabled" {
		t.Fatalf("identity event = %+v", store.events[0])
	}
	record, err := auditapp.NewIdentityActionParser().Parse(inbox.Record{
		Topic: store.events[1].Topic, Key: []byte(store.events[1].PartitionKey), Value: store.events[1].Payload,
	})
	if err != nil || record.Action != "user.disabled" || record.ActorID == nil || *record.ActorID != actorID ||
		record.ObjectID != targetID.String() || record.RequestID != "disable-request" ||
		string(record.Before) != `{"status":"active"}` || string(record.After) != `{"status":"disabled"}` {
		t.Fatalf("disable audit = %+v, error %v", record, err)
	}
}

func TestDisableUserCommandRejectsUntrustedActorAndInput(t *testing.T) {
	orgID, actorID, targetID := uuid.New(), uuid.New(), uuid.New()
	actor := identityapp.Principal{ID: actorID, OrgID: orgID, Role: domain.RoleAdmin}
	input := identityapp.DisableUserInput{TargetID: targetID, ExpectedRevision: 1, RequestID: "disable-request"}
	for _, tc := range []struct {
		name  string
		actor identityapp.Principal
		input identityapp.DisableUserInput
		want  error
	}{
		{name: "producer", actor: identityapp.Principal{ID: actorID, OrgID: orgID, Role: domain.RoleProducer}, input: input, want: identityapp.ErrForbidden},
		{name: "forced password change", actor: identityapp.Principal{ID: actorID, OrgID: orgID, Role: domain.RoleAdmin, MustChangePassword: true}, input: input, want: identityapp.ErrForbidden},
		{name: "missing target", actor: actor, input: identityapp.DisableUserInput{ExpectedRevision: 1, RequestID: "disable-request"}, want: identityapp.ErrInvalidDisableUser},
		{name: "missing revision", actor: actor, input: identityapp.DisableUserInput{TargetID: targetID, RequestID: "disable-request"}, want: identityapp.ErrInvalidDisableUser},
		{name: "missing request ID", actor: actor, input: identityapp.DisableUserInput{TargetID: targetID, ExpectedRevision: 1}, want: identityapp.ErrInvalidDisableUser},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &disableUserStoreStub{}
			_, err := identityapp.NewDisableUserCommand(store, time.Now).Execute(t.Context(), tc.actor, tc.input)
			if !errors.Is(err, tc.want) || store.calls != 0 {
				t.Fatalf("disable error = %v, store calls %d", err, store.calls)
			}
		})
	}
}

func TestDisableUserCommandPreservesStoreConflict(t *testing.T) {
	orgID, actorID, targetID := uuid.New(), uuid.New(), uuid.New()
	actor := identityapp.Principal{ID: actorID, OrgID: orgID, Role: domain.RoleAdmin}
	input := identityapp.DisableUserInput{TargetID: targetID, ExpectedRevision: 1, RequestID: "disable-request"}
	for _, want := range []error{domain.ErrLastActiveAdmin, domain.ErrRevisionConflict, identityapp.ErrForbidden} {
		store := &disableUserStoreStub{err: want}
		_, err := identityapp.NewDisableUserCommand(store, time.Now).Execute(t.Context(), actor, input)
		if !errors.Is(err, want) {
			t.Fatalf("store error %v became %v", want, err)
		}
	}
}
