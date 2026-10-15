package operation_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	catalogevent "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/event"
	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	inbox "github.com/StephenQiu30/glowloom-server/backend/internal/infra/inbox/application"
	operationevent "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/event"
	operation "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

type revokedDelivery struct {
	err   error
	calls int
}

func (s *revokedDelivery) CanDeliverWorkflowControl(context.Context, operationevent.ControlDelivery) (bool, error) {
	return s.err == nil, s.err
}
func (s *revokedDelivery) SignalControl(context.Context, string, uuid.UUID, string) error {
	s.calls++
	return nil
}
func (s *revokedDelivery) VerifyCredentialTestRequest(context.Context, catalog.CredentialTestRequest, uuid.UUID) error {
	return s.err
}
func (s *revokedDelivery) StartCredentialTest(context.Context, catalog.CredentialTestRequest) error {
	s.calls++
	return nil
}
func TestRevokedControlAndCredentialEventsDoNotBlockNextDelivery(t *testing.T) {
	for _, kind := range []string{"control", "credential"} {
		t.Run(kind, func(t *testing.T) {
			state := &revokedDelivery{}
			processed := &fakeStarterInbox{done: map[string]bool{}}
			project, actor, target, provider := uuid.New(), uuid.New(), uuid.New(), uuid.New()
			topic, aggregate := operationevent.WorkflowControlTopic, "operation"
			data := map[string]any{"action": "cancel", "request_id": uuid.NewString()}
			permanent, invalid := operation.ErrExecutionAuthorityRevoked, operation.ErrInvalidWorkflowControl
			var handle func(context.Context, inbox.Record) error
			if kind == "control" {
				handle = operationevent.NewWorkflowControlHandler(processed, state, state).Handle
			} else {
				topic, aggregate = catalogevent.CredentialTestTopic, "provider_credential"
				data = map[string]any{"test_id": uuid.New(), "provider_id": provider, "credential_id": target, "request_id": uuid.NewString()}
				permanent, invalid = catalog.ErrCredentialTestRevoked, catalog.ErrInvalidCredentialTest
				handle = catalogevent.NewCredentialTestHandler(processed, state, state).Handle
			}
			for _, failure := range []error{permanent, nil, errors.New("database offline"), invalid} {
				eventID := uuid.New()
				if kind == "credential" {
					requestID := uuid.New()
					testID := uuid.NewSHA1(requestID, []byte(actor.String()+project.String()+provider.String()+target.String()))
					data["request_id"] = requestID.String()
					data["test_id"] = testID
					eventID = uuid.NewSHA1(testID, []byte("credential_test_requested"))
				}
				body := map[string]any{"event_id": eventID, "event_type": topic, "occurred_at": time.Now().UTC(), "org_id": project, "actor": map[string]any{"kind": "user", "id": actor}, "aggregate": map[string]any{"type": aggregate, "id": target}, "data": data}
				if kind == "control" {
					body["project_id"] = project
				}
				payload, err := json.Marshal(body)
				if err != nil {
					t.Fatal(err)
				}
				state.err = failure
				err = handle(t.Context(), inbox.Record{Topic: topic, Key: []byte(project.String()), Value: payload})
				acknowledged := failure == nil || errors.Is(failure, permanent)
				if (err == nil) != acknowledged || processed.done[eventID.String()] != acknowledged {
					t.Fatalf("error %v: got %v acknowledged %v", failure, err, processed.done[eventID.String()])
				}
			}
			if state.calls != 1 {
				t.Fatalf("external actions %d", state.calls)
			}
		})
	}
}
