package websocket

import (
	"bytes"
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

func (h *Handler) processCollaboration(ctx context.Context, authorize func(context.Context) (identity.Principal, bool, error), id uuid.UUID, msg incoming, blocked bool, send func(packet) bool, fail func(string, error) bool) bool {
	input, decodeErr := application.DecodeSubmission(msg.Body)
	if decodeErr != nil && msg.Type == "resolve" {
		input, decodeErr = decodeLegacyReceiptRequest(msg.Body)
	}
	if decodeErr != nil {
		return fail(msg.OpID, domain.ErrInvalidCommand)
	}
	opCtx, stop := context.WithTimeout(ctx, 10*time.Second)
	actor, writable, err := authorize(opCtx)
	var receipt *application.CollaborationReceipt
	if err == nil {
		receipt, err = h.collaboration.Resolve(opCtx, actor, id, msg.OpID, input)
		if err == nil && receipt == nil && msg.Type == "submit" {
			if !writable || blocked {
				err = errReadOnly
			} else {
				var result application.CollaborationReceipt
				result, err = h.collaboration.Submit(opCtx, actor, id, msg.OpID, input)
				if err == nil {
					receipt = &result
					if result.Status == "accepted" {
						h.Notify(id)
					}
				}
			}
		}
	}
	stop()
	if err != nil {
		return fail(msg.OpID, err)
	}
	authCtx, done := context.WithTimeout(ctx, 10*time.Second)
	_, currentWritable, authErr := authorize(authCtx)
	err = authErr
	done()
	if err != nil {
		fail(msg.OpID, err)
		return false
	}
	if receipt == nil {
		return send(packet{Type: "receipt_not_found", OpID: msg.OpID})
	}
	dto := application.Receipt(*receipt)
	return send(packet{Type: "ack", OpID: msg.OpID, Revision: receipt.Revision, Receipt: &dto, Permissions: &application.Permissions{Read: true, Write: currentWritable && !blocked && receipt.Code != "schema_unsupported" && receipt.Code != "canvas_graph_read_only"}})
}

// Only an original version-1 request without client_id may use the historical
// receipt decoder. It cannot enter submission or acquire a new request meaning.
func decodeLegacyReceiptRequest(body []byte) (application.CollaborationInput, error) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(body, &fields) != nil {
		return application.CollaborationInput{}, domain.ErrInvalidCommand
	}
	if _, present := fields["client_id"]; present {
		return application.CollaborationInput{}, domain.ErrInvalidCommand
	}
	var input application.CollaborationInput
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || input.OperationVersion != 1 || input.BaseRevision < 1 {
		return application.CollaborationInput{}, domain.ErrInvalidCommand
	}
	var operations []map[string]json.RawMessage
	if json.Unmarshal(fields["operations"], &operations) != nil && input.UndoOf == "" {
		return application.CollaborationInput{}, domain.ErrInvalidCommand
	}
	for _, operation := range operations {
		for field, target := range map[string]any{"node": new(domain.Node), "edge": new(domain.Edge)} {
			if raw, ok := operation[field]; ok {
				decoder := json.NewDecoder(bytes.NewReader(raw))
				decoder.DisallowUnknownFields()
				if decoder.Decode(target) != nil {
					return application.CollaborationInput{}, domain.ErrInvalidCommand
				}
			}
		}
	}
	return input, nil
}
