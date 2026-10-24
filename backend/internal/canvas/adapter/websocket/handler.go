// Package websocket carries Yjs updates and durable receipts.
package websocket

import (
	"bytes"
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
	presence  map[uuid.UUID]*presenceRoom
	yjs       *application.YjsService
	authorize Authorize
	origin    string
	mu        sync.Mutex
	watchers  map[uuid.UUID]map[chan struct{}]struct{}
}

// NewHandler injects the existing application and current session authority.
func NewHandler(authorize Authorize, origin string) *Handler {
	return &Handler{authorize: authorize, origin: origin, watchers: make(map[uuid.UUID]map[chan struct{}]struct{})}
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
	Type            string          `json:"type"`
	ProtocolVersion int             `json:"protocol_version"`
	ClientID        uuid.UUID       `json:"client_id"`
	ScopeID         uuid.UUID       `json:"scope_id"`
	CSRFToken       string          `json:"csrf_token"`
	OpID            string          `json:"op_id"`
	Body            json.RawMessage `json:"body"`
}
type packet struct {
	Presence          []Presence                    `json:"presence,omitempty"`
	YjsVersions       []application.YjsVersion      `json:"yjs_versions,omitempty"`
	YjsRestore        *application.YjsRestoreResult `json:"yjs_restore,omitempty"`
	YjsState          *application.YjsState         `json:"yjs_state,omitempty"`
	YjsReceipt        *application.YjsReceipt       `json:"yjs_receipt,omitempty"`
	Type              string                        `json:"type"`
	Retryable         bool                          `json:"retryable"`
	ActorID           uuid.UUID                     `json:"actor_id,omitempty"`
	Revision          int64                         `json:"revision,omitempty,string"`
	Permissions       *application.Permissions      `json:"permissions,omitempty"`
	SupportedVersions []int                         `json:"supported_versions,omitempty"`
	OpID              string                        `json:"op_id,omitempty"`
	Status            int                           `json:"status,omitempty"`
	Code              string                        `json:"code,omitempty"`
	Document          *application.DocumentDTO      `json:"document,omitempty"`
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
	conn.SetReadLimit(2 << 20) // bounded base64 Yjs update plus its immutable envelope
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
			decoder := json.NewDecoder(bytes.NewReader(body))
			decoder.DisallowUnknownFields()
			if kind != websocket.MessageText || !json.Valid(body) || decoder.Decode(&msg) != nil {
				msg.Type = "invalid_message"
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
		if err != nil {
			return false
		}
		if len(body) > 16<<20 {
			body = []byte(`{"type":"error","status":413,"code":"canvas_capacity_exceeded","retryable":false}`)
			writeCtx, stop := context.WithTimeout(ctx, 5*time.Second)
			defer stop()
			_ = conn.Write(writeCtx, websocket.MessageText, body)
			return false
		}
		writeCtx, stop := context.WithTimeout(ctx, 5*time.Second)
		defer stop()
		return conn.Write(writeCtx, websocket.MessageText, body) == nil
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
	if join.Type == "invalid_message" {
		send(packet{Type: "error", Status: 422, Code: "invalid_message"})
		return
	}
	if join.ProtocolVersion != application.ProtocolVersion {
		send(packet{Type: "error", Status: 422, Code: "unsupported_protocol_version", SupportedVersions: []int{application.ProtocolVersion}})
		return
	}
	if h.yjs == nil {
		send(packet{Type: "error", Status: 503, Code: "collaboration_unavailable", Retryable: true})
		return
	}
	h.serveYjs(ctx, c.Request, conn, id, join, messages, syncRequests, send)
}

func problem(err error) (int, string) {
	var rejection *domain.OperationError
	var runtimeRejection *application.YjsRejection
	switch {
	case errors.As(err, &runtimeRejection):
		if runtimeRejection.Code == "STATE_TOO_LARGE" {
			return 413, "canvas_capacity_exceeded"
		}
		return 422, runtimeRejection.Code
	case errors.As(err, &rejection):
		if rejection.Code == "canvas_capacity_exceeded" {
			return 413, rejection.Code
		}
		return 409, rejection.Code
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
	case errors.Is(err, application.ErrYjsContention):
		return 409, "review_stale"
	case errors.Is(err, application.ErrIdempotencyConflict):
		return 409, "op_id_reused"
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
