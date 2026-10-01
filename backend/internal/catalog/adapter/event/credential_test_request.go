// Package event adapts durable catalog requests to the existing Temporal workflow.
package event

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/catalog/application"
	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
)

// CredentialTestTopic carries only authorized credential test identities.
const CredentialTestTopic = "lanverse.catalog.credential_test_requested.v1"

// ErrInvalidCredentialTestEvent rejects malformed and unbound test requests.
var ErrInvalidCredentialTestEvent = errors.New("invalid credential test request event")

// ProcessedStore marks a durable event only after its external delivery succeeds.
type ProcessedStore interface {
	ProcessExternalOnce(context.Context, string, string, func(context.Context) error) (bool, error)
}

// CredentialTestDeliveryStore proves the event was accepted by the authorized transaction.
type CredentialTestDeliveryStore interface {
	VerifyCredentialTestRequest(context.Context, application.CredentialTestRequest, uuid.UUID) error
}

// CredentialTestStarter starts only the catalog's reviewed test workflow.
type CredentialTestStarter interface {
	StartCredentialTest(context.Context, application.CredentialTestRequest) error
}

// CredentialTestHandler retries scheduling failures through the existing inbox.
type CredentialTestHandler struct {
	processed ProcessedStore
	store     CredentialTestDeliveryStore
	starter   CredentialTestStarter
}

// NewCredentialTestHandler injects durable deduplication, request proof, and Temporal.
func NewCredentialTestHandler(processed ProcessedStore, store CredentialTestDeliveryStore, starter CredentialTestStarter) *CredentialTestHandler {
	return &CredentialTestHandler{processed: processed, store: store, starter: starter}
}

// Handle verifies the persisted request before scheduling a test.
func (h *CredentialTestHandler) Handle(ctx context.Context, record inbox.Record) error {
	if h == nil || h.processed == nil || h.store == nil || h.starter == nil {
		return ErrInvalidCredentialTestEvent
	}
	request, eventID, err := parseCredentialTest(record)
	if err != nil {
		return err
	}
	_, err = h.processed.ProcessExternalOnce(ctx, "catalog-credential-test", eventID.String(), func(ctx context.Context) error {
		if err := h.store.VerifyCredentialTestRequest(ctx, request, eventID); err != nil {
			return fmt.Errorf("verify credential test request: %w", err)
		}
		return h.starter.StartCredentialTest(ctx, request)
	})
	if err != nil {
		return fmt.Errorf("deliver credential test request: %w", err)
	}
	return nil
}

func parseCredentialTest(record inbox.Record) (application.CredentialTestRequest, uuid.UUID, error) {
	var body struct {
		EventID    uuid.UUID `json:"event_id"`
		EventType  string    `json:"event_type"`
		OccurredAt time.Time `json:"occurred_at"`
		OrgID      uuid.UUID `json:"org_id"`
		Actor      struct {
			Kind string    `json:"kind"`
			ID   uuid.UUID `json:"id"`
		} `json:"actor"`
		Aggregate struct {
			Type string    `json:"type"`
			ID   uuid.UUID `json:"id"`
		} `json:"aggregate"`
		Data struct {
			TestID       uuid.UUID `json:"test_id"`
			ProviderID   uuid.UUID `json:"provider_id"`
			CredentialID uuid.UUID `json:"credential_id"`
			RequestID    string    `json:"request_id"`
		} `json:"data"`
	}
	invalid := func() (application.CredentialTestRequest, uuid.UUID, error) {
		return application.CredentialTestRequest{}, uuid.Nil, ErrInvalidCredentialTestEvent
	}
	if len(record.Value) == 0 || len(record.Value) > 8192 {
		return invalid()
	}
	decoder := json.NewDecoder(bytes.NewReader(record.Value))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&body) != nil || decoder.Decode(new(any)) != io.EOF || record.Topic != CredentialTestTopic || body.EventType != record.Topic || string(record.Key) != body.OrgID.String() || body.OccurredAt.IsZero() || body.Actor.Kind != "user" || body.Aggregate.Type != "provider_credential" || body.Aggregate.ID != body.Data.CredentialID {
		return invalid()
	}
	key, err := uuid.Parse(body.Data.RequestID)
	if err != nil || key == uuid.Nil || key.String() != body.Data.RequestID {
		return invalid()
	}
	for _, id := range []uuid.UUID{body.EventID, body.OrgID, body.Actor.ID, body.Data.TestID, body.Data.ProviderID, body.Data.CredentialID} {
		if id == uuid.Nil {
			return invalid()
		}
	}
	request := application.CredentialTestRequest{TestID: body.Data.TestID, ProviderID: body.Data.ProviderID, CredentialID: body.Data.CredentialID, ActorID: body.Actor.ID, OrgID: body.OrgID, RequestID: body.Data.RequestID}
	if request.TestID != uuid.NewSHA1(key, []byte(request.ActorID.String()+request.OrgID.String()+request.ProviderID.String()+request.CredentialID.String())) || body.EventID != uuid.NewSHA1(request.TestID, []byte("credential_test_requested")) {
		return invalid()
	}
	return request, body.EventID, nil
}

var _ inbox.Handler = (*CredentialTestHandler)(nil)
