// Package sse streams authorized project events from Redis to HTTP clients.
package sse

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	redisclient "github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	redisrealtime "github.com/StephenQiu30/lanverse/backend/internal/infra/realtime/adapter/redis"
	"github.com/StephenQiu30/lanverse/backend/internal/infra/realtime/application"
)

const (
	defaultHeartbeat = 20 * time.Second
	defaultMaxAge    = 30 * time.Minute
	seenLimit        = 5000
)

// ErrInvalidHandler means a required HTTP streaming dependency is missing.
var ErrInvalidHandler = errors.New("invalid project SSE handler")

// AuthorizeProject checks the authenticated request's access to a project.
// The API composition root must supply this before mounting the route.
type AuthorizeProject func(*http.Request, string) bool

// Options sets SSE timing; zero values use the DES-03 defaults.
type Options struct {
	HeartbeatInterval time.Duration
	MaxConnectionAge  time.Duration
}

// Handler serves one authorized project's replay and live events.
type Handler struct {
	client    *redisclient.Client
	buffer    *redisrealtime.Sink
	authorize AuthorizeProject
	logger    *zap.Logger
	heartbeat time.Duration
	maxAge    time.Duration
}

// NewHandler requires an authorization check before any project subscription.
func NewHandler(client *redisclient.Client, authorize AuthorizeProject, logger *zap.Logger, options Options) (*Handler, error) {
	if client == nil || authorize == nil || logger == nil || options.HeartbeatInterval < 0 || options.MaxConnectionAge < 0 {
		return nil, ErrInvalidHandler
	}
	if options.HeartbeatInterval == 0 {
		options.HeartbeatInterval = defaultHeartbeat
	}
	if options.MaxConnectionAge == 0 {
		options.MaxConnectionAge = defaultMaxAge
	}
	return &Handler{client: client, buffer: redisrealtime.NewSink(client), authorize: authorize, logger: logger,
		heartbeat: options.HeartbeatInterval, maxAge: options.MaxConnectionAge}, nil
}

// ServeProject serves a project route after an outer authentication middleware.
// It checks project membership before subscribing or reading the replay buffer.
func (h *Handler) ServeProject(w http.ResponseWriter, r *http.Request, projectID string) {
	id, err := uuid.Parse(projectID)
	if err != nil {
		http.Error(w, "invalid project ID", http.StatusBadRequest)
		return
	}
	projectID = id.String()
	if !h.authorize(r, projectID) {
		http.Error(w, "project access denied", http.StatusForbidden)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unavailable", http.StatusInternalServerError)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.maxAge)
	defer cancel()
	streamRequest := r.WithContext(ctx)
	channel := "project:" + projectID
	subscription := h.client.Subscribe(ctx, channel)
	var readerDone <-chan struct{}
	defer func() {
		cancel()
		if err := subscription.Close(); err != nil && r.Context().Err() == nil {
			h.logger.Warn("close project subscription", zap.String("project_id", projectID), zap.Error(err))
		}
		if readerDone != nil {
			<-readerDone
		}
	}()
	if _, err := subscription.Receive(ctx); err != nil {
		h.logger.Error("subscribe project events", zap.String("project_id", projectID), zap.Error(err))
		http.Error(w, "event stream unavailable", http.StatusServiceUnavailable)
		return
	}
	replay, resync, err := h.buffer.Replay(ctx, projectID, r.Header.Get("Last-Event-ID"))
	if err != nil {
		h.logger.Error("read project events", zap.String("project_id", projectID), zap.Error(err))
		http.Error(w, "event stream unavailable", http.StatusServiceUnavailable)
		return
	}
	if !h.authorize(streamRequest, projectID) {
		http.Error(w, "project access denied", http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("X-Accel-Buffering", "no")
	seen := newSeenIDs(r.Header.Get("Last-Event-ID"))
	if resync {
		if _, err := io.WriteString(w, "event: resync\ndata: {}\n\n"); err != nil {
			return
		}
		flusher.Flush()
	}
	for _, message := range replay {
		if err := writeMessage(w, message); err != nil {
			h.logger.Warn("write replay event", zap.String("project_id", projectID), zap.Error(err))
			return
		}
		seen.add(message.ID)
		flusher.Flush()
	}
	if _, err := io.WriteString(w, ": connected\n\n"); err != nil {
		return
	}
	flusher.Flush()

	messages := make(chan *redisclient.Message)
	readErrors := make(chan error, 1)
	done := make(chan struct{})
	readerDone = done
	go func() {
		defer close(done)
		for {
			message, err := subscription.ReceiveMessage(ctx)
			if err != nil {
				readErrors <- err
				return
			}
			select {
			case messages <- message:
			case <-ctx.Done():
				return
			}
		}
	}()
	heartbeat := time.NewTicker(h.heartbeat)
	defer heartbeat.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case err := <-readErrors:
			if ctx.Err() == nil {
				h.logger.Warn("receive project event", zap.String("project_id", projectID), zap.Error(err))
			}
			return
		case <-heartbeat.C:
			if !h.authorize(streamRequest, projectID) {
				return
			}
			if _, err := io.WriteString(w, ": heartbeat\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case raw := <-messages:
			if !h.authorize(streamRequest, projectID) {
				return
			}
			var message application.Message
			if err := json.Unmarshal([]byte(raw.Payload), &message); err != nil || !validMessage(message) {
				h.logger.Warn("invalid project event", zap.String("project_id", projectID), zap.Error(err))
				return
			}
			if seen.has(message.ID) {
				continue
			}
			if err := writeMessage(w, message); err != nil {
				return
			}
			seen.add(message.ID)
			flusher.Flush()
		}
	}
}

func writeMessage(w io.Writer, message application.Message) error {
	if !validMessage(message) {
		return errors.New("invalid SSE message")
	}
	if _, err := fmt.Fprintf(w, "id: %s\nevent: %s\ndata: %s\n\n", message.ID, message.Event, message.Data); err != nil {
		return fmt.Errorf("write SSE event: %w", err)
	}
	return nil
}

func validMessage(message application.Message) bool {
	if _, err := uuid.Parse(message.ID); err != nil {
		return false
	}
	return message.Event != "" && !strings.ContainsAny(message.Event, "\r\n:") && json.Valid(message.Data)
}

type seenIDs struct {
	set   map[string]bool
	order []string
}

func newSeenIDs(lastID string) *seenIDs {
	seen := &seenIDs{set: make(map[string]bool)}
	if lastID != "" {
		seen.add(lastID)
	}
	return seen
}

func (s *seenIDs) has(id string) bool { return s.set[id] }

func (s *seenIDs) add(id string) {
	if s.set[id] {
		return
	}
	s.set[id] = true
	s.order = append(s.order, id)
	if len(s.order) > seenLimit {
		delete(s.set, s.order[0])
		s.order = s.order[1:]
	}
}
