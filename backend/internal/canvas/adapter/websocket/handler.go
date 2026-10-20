// Package websocket carries existing CAS canvas transactions over a live channel.
package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// Authorize revalidates credentials and document read/write authority. It must
// not touch session activity; heartbeat traffic is not human activity.
type Authorize func(context.Context, *http.Request, uuid.UUID, uuid.UUID, string) (identity.Principal, bool, error)

// Handler owns only bounded connection notification hints, never document data.
type Handler struct {
	service   *application.Service
	authorize Authorize
	origin    string
	mu        sync.Mutex
	watchers  map[uuid.UUID]map[chan struct{}]struct{}
}

// NewHandler injects the existing application and current session authority.
func NewHandler(service *application.Service, authorize Authorize, origin string) *Handler {
	return &Handler{service: service, authorize: authorize, origin: origin, watchers: make(map[uuid.UUID]map[chan struct{}]struct{})}
}

// Register binds socket lifetimes to the API role rather than hijacked requests.
func (h *Handler) Register(lifetime context.Context, group *gin.RouterGroup) {
	group.GET("/canvases/:id/live", func(c *gin.Context) { h.serve(lifetime, c) })
}

// Notify wakes current subscribers after an external commit. A missed hint is
// repaired by heartbeat reading PostgreSQL; no notification is a durable receipt.
func (h *Handler) Notify(id uuid.UUID) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.watchers[id] {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}
func (h *Handler) subscribe(id uuid.UUID) (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	h.mu.Lock()
	if h.watchers[id] == nil {
		h.watchers[id] = make(map[chan struct{}]struct{})
	}
	h.watchers[id][ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		delete(h.watchers[id], ch)
		if len(h.watchers[id]) == 0 {
			delete(h.watchers, id)
		}
	}
}

var errReadOnly = errors.New("canvas is currently read-only")

type incoming struct {
	Type            string                    `json:"type"`
	ProtocolVersion int                       `json:"protocol_version"`
	ScopeID         uuid.UUID                 `json:"scope_id"`
	CSRFToken       string                    `json:"csrf_token"`
	OpID            string                    `json:"op_id"`
	Body            application.CommandsInput `json:"body"`
}
type packet struct {
	Type     string              `json:"type"`
	Revision int64               `json:"revision,omitempty"`
	Writable bool                `json:"writable"`
	OpID     string              `json:"op_id,omitempty"`
	Status   int                 `json:"status,omitempty"`
	Code     string              `json:"code,omitempty"`
	Document *domain.Document    `json:"document,omitempty"`
	Result   *application.Result `json:"result,omitempty"`
}

func (h *Handler) serve(lifetime context.Context, c *gin.Context) {
	if c.GetHeader("Origin") != h.origin || h.origin == "" {
		httpapi.WriteProblem(c, 403, "origin_forbidden", nil)
		return
	}
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	conn, err := websocket.Accept(c.Writer, c.Request, &websocket.AcceptOptions{InsecureSkipVerify: true, CompressionMode: websocket.CompressionDisabled})
	if err != nil {
		return
	}
	defer func() { _ = conn.CloseNow() }()
	ctx, cancel := context.WithCancel(lifetime)
	defer cancel()
	conn.SetReadLimit(1 << 20)
	// Keep the control reader moving while a database command runs. Sync hints
	// coalesce; a second queued business request closes the overloaded connection
	// rather than blocking Pong processing or allocating an unbounded queue.
	messages := make(chan incoming, 1)
	syncRequests := make(chan struct{}, 1)
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		defer cancel()
		for {
			kind, body, err := conn.Read(ctx)
			if err != nil {
				return
			}
			var msg incoming
			if kind != websocket.MessageText || json.Unmarshal(body, &msg) != nil {
				return
			}
			if msg.Type == "sync" {
				select {
				case syncRequests <- struct{}{}:
				default:
				}
				continue
			}
			select {
			case messages <- msg:
			case <-ctx.Done():
				return
			default:
				return
			}
		}
	}()
	defer func() { cancel(); _ = conn.CloseNow(); <-readerDone }()
	send := func(p packet) bool {
		body, err := json.Marshal(p)
		if err != nil || len(body) > 16<<20 {
			return false
		}
		writeCtx, stop := context.WithTimeout(ctx, 5*time.Second)
		defer stop()
		return conn.Write(writeCtx, websocket.MessageText, body) == nil
	}
	fail := func(op string, err error) bool {
		status, code := problem(err)
		return send(packet{Type: "error", OpID: op, Status: status, Code: code}) && status != 401 && status != 403 && status != 404
	}
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	var join incoming
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
		return
	case join = <-messages:
	}
	if join.Type != "join" || join.ProtocolVersion != 1 || join.ScopeID == uuid.Nil || join.CSRFToken == "" {
		send(packet{Type: "error", Status: 422, Code: "invalid_join"})
		return
	}
	wake, unsubscribe := h.subscribe(id)
	defer unsubscribe()
	var revision int64
	var writable bool
	authorized := func(opCtx context.Context) (identity.Principal, bool, error) {
		return h.authorize(opCtx, c.Request, id, join.ScopeID, join.CSRFToken)
	}
	syncHead := func(force bool) bool {
		opCtx, stop := context.WithTimeout(ctx, 10*time.Second)
		defer stop()
		actor, canWrite, err := authorized(opCtx)
		if err != nil {
			fail("", err)
			return false
		}
		doc, err := h.service.Get(opCtx, actor, id)
		if err != nil {
			fail("", err)
			return false
		}
		if force || doc.Revision != revision || canWrite != writable { // Also catches archive without a graph revision change.
			if !send(packet{Type: "snapshot", Revision: doc.Revision, Document: &doc, Writable: canWrite}) {
				return false
			}
		}
		revision, writable = doc.Revision, canWrite
		return true
	}
	if !syncHead(true) || !send(packet{Type: "ready", Revision: revision, Writable: writable}) {
		return
	}
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-syncRequests:
			if !syncHead(true) {
				return
			}
		case <-wake:
			if !syncHead(false) {
				return
			}
		case <-tick.C:
			pingCtx, stop := context.WithTimeout(ctx, 5*time.Second)
			err := conn.Ping(pingCtx)
			stop()
			if err != nil {
				return
			}
			if !syncHead(false) || !send(packet{Type: "heartbeat", Revision: revision, Writable: writable}) {
				return
			}
		case msg := <-messages:
			if msg.Type != "submit" && msg.Type != "resolve" {
				if !send(packet{Type: "error", Status: 422, Code: "invalid_message"}) {
					return
				}
				continue
			}
			opCtx, stop := context.WithTimeout(ctx, 10*time.Second)
			actor, canWrite, err := authorized(opCtx)
			var result *application.Result
			if err == nil {
				// Always resolve first: a committed save remains confirmable after archive.
				result, err = h.service.Resolve(opCtx, actor, id, msg.OpID, msg.Body)
				if err == nil && result == nil && msg.Type == "submit" {
					if !canWrite {
						err = errReadOnly
					} else {
						var saved application.Result
						saved, err = h.service.Execute(opCtx, actor, id, msg.OpID, msg.Body)
						if err == nil {
							result = &saved
							h.Notify(id)
						}
					}
				}
			}
			stop()
			if err != nil {
				if !fail(msg.OpID, err) {
					return
				}
				if errors.Is(err, errReadOnly) && !syncHead(true) {
					return
				}
				continue
			}
			// Recheck before sending any private receipt, including durable replays.
			authCtx, done := context.WithTimeout(ctx, 10*time.Second)
			_, _, err = authorized(authCtx)
			done()
			if err != nil {
				fail(msg.OpID, err)
				return
			}
			if result == nil {
				if !send(packet{Type: "not_found", OpID: msg.OpID}) {
					return
				}
			} else if !send(packet{Type: "ack", OpID: msg.OpID, Revision: result.Revision, Result: result, Writable: canWrite}) {
				return
			}
		}
	}
}

func problem(err error) (int, string) {
	var conflict *application.RevisionConflict
	switch {
	case errors.Is(err, errReadOnly):
		return 409, "canvas_read_only"
	case errors.Is(err, identity.ErrSessionInvalid):
		return 401, "session_invalid"
	case errors.Is(err, identity.ErrAccessExpired):
		return 401, "access_expired"
	case errors.Is(err, identity.ErrForbidden):
		return 403, "forbidden"
	case errors.Is(err, application.ErrNotFound), errors.Is(err, media.ErrNotFound):
		return 404, "not_found"
	case errors.As(err, &conflict):
		return 409, "revision_conflict"
	case errors.Is(err, application.ErrIdempotencyConflict):
		return 409, "idempotency_key_reused"
	case errors.Is(err, domain.ErrGraphReadOnly):
		return 422, "canvas_graph_read_only"
	case errors.Is(err, domain.ErrUnsupportedCommand):
		return 422, "unsupported_command"
	case errors.Is(err, domain.ErrInvalidCommand):
		return 422, "invalid_request"
	default:
		return 503, "dependency_unavailable"
	}
}
