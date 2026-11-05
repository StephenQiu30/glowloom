package operation_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	catalogevent "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/event"
	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	inbox "github.com/StephenQiu30/glowloom-server/backend/internal/infra/inbox/application"
	operation "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

type revokedDelivery struct {
	err   error
	calls int
}

func (s *revokedDelivery) VerifyCredentialTestRequest(context.Context, catalog.CredentialTestRequest, uuid.UUID) error {
	return s.err
}
func (s *revokedDelivery) StartCredentialTest(context.Context, catalog.CredentialTestRequest) error {
	s.calls++
	return nil
}
func TestRevokedCredentialEventsDoNotBlockNextDelivery(t *testing.T) {
	state := &revokedDelivery{}
	processed := &auditInbox{done: map[string]bool{}}
	project, actor, target, provider := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	handle := catalogevent.NewCredentialTestHandler(processed, state, state).Handle
	for _, failure := range []error{catalog.ErrCredentialTestRevoked, nil, errors.New("database offline"), catalog.ErrInvalidCredentialTest} {
		requestID := uuid.New()
		testID := uuid.NewSHA1(requestID, []byte(actor.String()+project.String()+provider.String()+target.String()))
		eventID := uuid.NewSHA1(testID, []byte("credential_test_requested"))
		body := map[string]any{"event_id": eventID, "event_type": catalogevent.CredentialTestTopic, "occurred_at": time.Now().UTC(), "org_id": project, "actor": map[string]any{"kind": "user", "id": actor}, "aggregate": map[string]any{"type": "provider_credential", "id": target}, "data": map[string]any{"test_id": testID, "provider_id": provider, "credential_id": target, "request_id": requestID.String()}}
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		state.err = failure
		err = handle(t.Context(), inbox.Record{Topic: catalogevent.CredentialTestTopic, Key: []byte(project.String()), Value: raw})
		ack := failure == nil || errors.Is(failure, catalog.ErrCredentialTestRevoked)
		require.Equal(t, ack, err == nil)
		require.Equal(t, ack, processed.done[eventID.String()])
	}
	require.Equal(t, 1, state.calls)
}
func TestRevokedControlTaskDoesNotBlockNextDelivery(t *testing.T) {
	s := newStateStore()
	d := operation.WorkflowControlDelivery{EventID: uuid.New(), TargetID: s.loaded.Operation.ID, Action: "cancel", TargetType: "operation"}
	raw, err := json.Marshal(d)
	require.NoError(t, err)
	s.controlErr = operation.ErrExecutionAuthorityRevoked
	_, err = s.handlers()["operation.control"](t.Context(), taskruntime.Job{BusinessID: d.EventID.String(), Reference: raw})
	require.NoError(t, err)
	require.Empty(t, s.scheduled)
	s.controlErr = nil
	_, err = s.handlers()["operation.control"](t.Context(), taskruntime.Job{BusinessID: d.EventID.String(), Reference: raw})
	require.NoError(t, err)
	require.Equal(t, []string{"operation.run"}, s.scheduled)
}

type auditInbox struct{ done map[string]bool }

func (i *auditInbox) ProcessExternalOnce(ctx context.Context, _, id string, effect func(context.Context) error) (bool, error) {
	if i.done[id] {
		return false, nil
	}
	if err := effect(ctx); err != nil {
		return false, err
	}
	i.done[id] = true
	return true, nil
}
