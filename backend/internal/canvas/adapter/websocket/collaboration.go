package websocket

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

func (h *Handler) processCollaboration(ctx context.Context, authorize func(context.Context) (identity.Principal, bool, error), id uuid.UUID, msg incoming, blocked bool, send func(packet) bool, fail func(string, error) bool) bool {
	var input application.CollaborationInput
	decoder := json.Unmarshal(msg.Body, &input)
	if decoder != nil {
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
	_, _, err = authorize(authCtx)
	done()
	if err != nil {
		fail(msg.OpID, err)
		return false
	}
	if receipt == nil {
		return send(packet{Type: "not_found", OpID: msg.OpID})
	}
	return send(packet{Type: "ack", OpID: msg.OpID, Revision: receipt.Revision, Receipt: receipt, Writable: writable && !blocked})
}
