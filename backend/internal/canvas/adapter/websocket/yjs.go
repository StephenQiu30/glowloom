package websocket

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// WithYjs enables protocol 4 only when its private runtime is configured.
func (h *Handler) WithYjs(service *application.YjsService) *Handler { h.yjs = service; return h }
func (h *Handler) serveYjs(ctx context.Context, request *http.Request, conn *websocket.Conn, id uuid.UUID, join incoming, messages <-chan incoming, syncRequests <-chan struct{}, send func(packet) bool) {
	if join.Type != "join" || join.ClientID == uuid.Nil || join.ScopeID == uuid.Nil || join.CSRFToken == "" {
		send(packet{Type: "error", Status: 422, Code: "invalid_join"})
		return
	}
	var actorID uuid.UUID
	var lastEpoch uuid.UUID
	var lastSeq application.Revision = -1
	var writeAllowed bool
	authorize := func() (identity.Principal, error) {
		actor, write, err := h.authorize(ctx, request, id, join.ScopeID, join.CSRFToken)
		if err != nil {
			return actor, err
		}
		if actorID != uuid.Nil && actorID != actor.ID {
			return actor, identity.ErrForbidden
		}
		actorID = actor.ID
		writeAllowed = write
		return actor, nil
	}
	fail := func(op string, err error) bool {
		status, code := problem(err)
		return send(packet{Type: "error", OpID: op, Status: status, Code: code, Retryable: status == 503}) && status != 401 && status != 403 && status != 404
	}
	syncHead := func(force bool) bool {
		actor, err := authorize()
		if err != nil {
			fail("", err)
			return false
		}
		state, err := h.yjs.Load(ctx, actor, id)
		if err != nil {
			fail("", err)
			return false
		}
		state.Document.Permissions.Write = state.Document.Permissions.Write && writeAllowed
		if len(state.State) == 0 {
			return send(packet{Type: "yjs_migration_required", YjsState: &state, ActorID: actor.ID})
		}
		if force || lastEpoch != state.Epoch || lastSeq != state.CommitSeq {
			if !send(packet{Type: "yjs_snapshot", YjsState: &state, ActorID: actor.ID}) {
				return false
			}
			lastEpoch = state.Epoch
			lastSeq = state.CommitSeq
		}
		return send(packet{Type: "heartbeat", Permissions: &state.Document.Permissions, Revision: int64(state.Document.Revision), ActorID: actor.ID})
	}
	if !syncHead(true) {
		return
	}
	presenceWake, leavePresence, ok := h.joinPresence(id, actorID, join.ClientID)
	if !ok {
		send(packet{Type: "error", Status: 429, Code: "presence_capacity"})
		return
	}
	defer leavePresence()
	var lastCursor time.Time
	sendPresence := func() bool {
		if _, err := authorize(); err != nil {
			fail("", err)
			return false
		}
		return send(packet{Type: "yjs_presence", Presence: h.presenceSnapshot(id)})
	}
	wake, unsubscribe := h.subscribe(id)
	defer unsubscribe()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-presenceWake:
			if !sendPresence() {
				return
			}
		case <-wake:
			if !syncHead(false) {
				return
			}
		case <-syncRequests:
			if !syncHead(true) {
				return
			}
		case <-ticker.C:
			pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := conn.Ping(pingCtx)
			cancel()
			if err != nil || !syncHead(false) {
				return
			}
			h.updatePresence(id, actorID, join.ClientID, nil, true)
		case msg := <-messages:
			actor, err := authorize()
			if err != nil {
				if !fail(msg.OpID, err) {
					return
				}
				continue
			}
			switch msg.Type {
			case "yjs_presence":
				if time.Since(lastCursor) < 80*time.Millisecond {
					continue
				}
				var input struct {
					Cursor *Cursor `json:"cursor"`
				}
				if !decodeYjsBody(msg.Body, &input) || !h.updatePresence(id, actor.ID, join.ClientID, input.Cursor, false) {
					send(packet{Type: "error", Status: 422, Code: "invalid_presence"})
					continue
				}
				lastCursor = time.Now()

			case "yjs_history":
				var input struct {
					Before application.Revision `json:"before"`
				}
				if !decodeYjsBody(msg.Body, &input) {
					send(packet{Type: "error", OpID: msg.OpID, Status: 422, Code: "invalid_message"})
					continue
				}
				versions, err := h.yjs.Versions(ctx, actor, id, input.Before)
				if err != nil {
					if !fail(msg.OpID, err) {
						return
					}
					continue
				}
				if !send(packet{Type: "yjs_history", OpID: msg.OpID, YjsVersions: versions}) {
					return
				}
			case "yjs_version":
				var input struct {
					ID uuid.UUID `json:"id"`
				}
				if !decodeYjsBody(msg.Body, &input) || input.ID == uuid.Nil {
					send(packet{Type: "error", OpID: msg.OpID, Status: 422, Code: "invalid_message"})
					continue
				}
				document, err := h.yjs.Version(ctx, actor, id, input.ID)
				if err != nil {
					if !fail(msg.OpID, err) {
						return
					}
					continue
				}
				if !send(packet{Type: "yjs_version", OpID: msg.OpID, Document: &document}) {
					return
				}
			case "yjs_restore":
				var input application.YjsRestore
				if !decodeYjsBody(msg.Body, &input) || input.SessionID != join.ClientID || input.ID.String() != msg.OpID {
					send(packet{Type: "error", OpID: msg.OpID, Status: 422, Code: "invalid_message"})
					continue
				}
				var result application.YjsRestoreResult
				replay, err := h.yjs.ResolveRestore(ctx, actor, id, input)
				if err == nil && replay != nil {
					result = *replay
				} else if err == nil {
					if !writeAllowed {
						err = errReadOnly
					} else {
						result, err = h.yjs.RestoreVersion(ctx, actor, id, input)
					}
				}
				if err != nil {
					if !fail(msg.OpID, err) {
						return
					}
					continue
				}
				h.Notify(id)
				if !syncHead(true) || !send(packet{Type: "yjs_restore_ack", OpID: msg.OpID, YjsRestore: &result}) {
					return
				}

			case "yjs_initialize":
				if !writeAllowed {
					if !fail(msg.OpID, errReadOnly) {
						return
					}
					continue
				}
				var input struct {
					Revision application.Revision `json:"revision"`
				}
				if !decodeYjsBody(msg.Body, &input) {
					send(packet{Type: "error", OpID: msg.OpID, Status: 422, Code: "invalid_message"})
					continue
				}
				_, err := h.yjs.Initialize(ctx, actor, id, input.Revision)
				if err != nil {
					if !fail(msg.OpID, err) {
						return
					}
					continue
				}
				h.Notify(id)
				if !syncHead(true) {
					return
				}
			case "yjs_submit", "yjs_resolve":
				var input application.YjsUpdate
				if !decodeYjsBody(msg.Body, &input) || input.SessionID != join.ClientID || input.UpdateID.String() != msg.OpID {
					send(packet{Type: "error", OpID: msg.OpID, Status: 422, Code: "invalid_message"})
					continue
				}
				receipt, err := h.yjs.Resolve(ctx, actor, id, input)
				if err == nil && receipt == nil && msg.Type == "yjs_submit" {
					if !writeAllowed {
						err = errReadOnly
					} else {
						var accepted application.YjsReceipt
						accepted, err = h.yjs.Submit(ctx, actor, id, input)
						if err == nil {
							receipt = &accepted
						}
					}
				}
				if err != nil {
					if !fail(msg.OpID, err) {
						return
					}
					continue
				}
				// Recheck authority and deliver the complete accepted state before its receipt.
				if !syncHead(true) {
					return
				}
				if receipt == nil {
					if !send(packet{Type: "yjs_not_found", OpID: msg.OpID}) {
						return
					}
					continue
				}
				if receipt.Status == "accepted" {
					h.Notify(id)
				}
				if !send(packet{Type: "yjs_ack", OpID: msg.OpID, YjsReceipt: receipt}) {
					return
				}
			default:
				if !send(packet{Type: "error", OpID: msg.OpID, Status: 422, Code: "invalid_message"}) {
					return
				}
			}
		}
	}
}
func decodeYjsBody(body []byte, target any) bool {
	if !json.Valid(body) {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target) == nil
}
