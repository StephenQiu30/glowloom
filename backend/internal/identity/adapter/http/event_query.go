package http

import (
	"errors"
	"net/url"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

type EventHandler struct{ query *application.EventQuery }

func NewEventHandler(query *application.EventQuery) *EventHandler { return &EventHandler{query: query} }

// Register requires email session middleware; the store rechecks current authority.
func (h *EventHandler) Register(group *gin.RouterGroup) {
	group.GET("/me/security-events", h.list("self"))
	group.GET("/teams/:id/audit-events", h.list("team"))
	group.GET("/platform/audit-events", h.list("platform"))
}

type SecurityEventView struct {
	ID           uuid.UUID         `json:"id"`
	ActorID      *uuid.UUID        `json:"actor_id"`
	TargetUserID *uuid.UUID        `json:"target_user_id"`
	TeamID       *uuid.UUID        `json:"team_id"`
	Action       string            `json:"action"`
	Result       string            `json:"result"`
	Reason       string            `json:"reason"`
	RequestID    string            `json:"request_id"`
	SafeDiff     map[string]string `json:"safe_diff"`
	CreatedAt    time.Time         `json:"created_at"`
}

type SecurityEventPageView struct {
	Items      []SecurityEventView `json:"items"`
	NextCursor *string             `json:"next_cursor"`
	AsOf       time.Time           `json:"as_of"`
}

func parseEventFilter(c *gin.Context, scope string) (application.EventFilter, error) {
	f := application.EventFilter{ScopeKind: scope}
	values, err := url.ParseQuery(c.Request.URL.RawQuery)
	if err != nil {
		return f, application.ErrInvalidEventFilter
	}
	// Reject duplicated and unsupported filters instead of silently widening scope.
	for key, items := range values {
		if len(items) != 1 {
			return f, application.ErrInvalidEventFilter
		}
		value := items[0]
		switch key {
		case "cursor":
			f.Cursor = value
		case "action":
			f.Action = value
		case "limit":
			f.Limit, err = strconv.Atoi(value)
			if err != nil || f.Limit < 1 {
				return f, application.ErrInvalidEventFilter
			}
		case "from", "to":
			parsed, err := time.Parse(time.RFC3339Nano, value)
			if err != nil {
				return f, application.ErrInvalidEventFilter
			}
			if key == "from" {
				f.From = &parsed
			} else {
				f.To = &parsed
			}
		case "actor_id", "target_user_id", "team_id":
			if scope != "platform" {
				return f, application.ErrInvalidEventFilter
			}
			id, err := uuid.Parse(value)
			if err != nil || id == uuid.Nil {
				return f, application.ErrInvalidEventFilter
			}
			switch key {
			case "actor_id":
				f.ActorID = &id
			case "target_user_id":
				f.TargetUserID = &id
			case "team_id":
				f.TeamID = &id
			}
		default:
			return f, application.ErrInvalidEventFilter
		}
	}
	if scope == "team" {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil || id == uuid.Nil {
			return f, application.ErrInvalidEventFilter
		}
		f.TeamID = &id
	}
	return f, nil
}

func (h *EventHandler) list(scope string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		filter, err := parseEventFilter(c, scope)
		if err != nil {
			httpapi.WriteProblem(c, 422, "invalid_request", nil)
			return
		}
		page, err := h.query.List(c.Request.Context(), Principal(c), filter)
		if errors.Is(err, application.ErrInvalidEventFilter) {
			httpapi.WriteProblem(c, 422, "invalid_request", nil)
			return
		}
		if err != nil {
			emailAuthProblem(c, err)
			return
		}
		items := make([]SecurityEventView, 0, len(page.Items))
		for _, e := range page.Items {
			items = append(items, SecurityEventView{ID: e.ID, ActorID: e.ActorID, TargetUserID: e.TargetUserID, TeamID: e.TeamID, Action: e.Action, Result: e.Result, Reason: e.Reason, RequestID: e.RequestID, SafeDiff: e.SafeDiff, CreatedAt: e.CreatedAt})
		}
		c.JSON(200, SecurityEventPageView{Items: items, NextCursor: page.NextCursor, AsOf: page.AsOf})
	}
}
