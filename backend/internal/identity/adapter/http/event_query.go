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

// EventHandler exposes separate self, team and platform security event queries.
type EventHandler struct{ query *application.EventQuery }

// NewEventHandler constructs EventHandler with explicit caller-supplied dependencies.
func NewEventHandler(query *application.EventQuery) *EventHandler { return &EventHandler{query: query} }

// Register requires email session middleware; the store rechecks current authority.
func (h *EventHandler) Register(group *gin.RouterGroup) {
	group.GET("/me/security-events", h.self)
	group.GET("/teams/:id/audit-events", h.team)
	group.GET("/platform/audit-events", h.platform)
}

// SecurityEventView contains the authorized, redacted event projection.
type SecurityEventView struct {
	ID           uuid.UUID         `json:"id" binding:"required"`
	ActorID      *uuid.UUID        `json:"actor_id" binding:"required"`
	TargetUserID *uuid.UUID        `json:"target_user_id" binding:"required"`
	TeamID       *uuid.UUID        `json:"team_id" binding:"required"`
	Action       string            `json:"action" binding:"required"`
	Result       string            `json:"result" binding:"required"`
	Reason       string            `json:"reason" binding:"required"`
	RequestID    string            `json:"request_id" binding:"required"`
	SafeDiff     map[string]string `json:"safe_diff" binding:"required"`
	CreatedAt    time.Time         `json:"created_at" binding:"required"`
}

// SecurityEventPageView returns a bounded event page and its query snapshot.
type SecurityEventPageView struct {
	Items      []SecurityEventView `json:"items" binding:"required"`
	NextCursor *string             `json:"next_cursor" binding:"required"`
	AsOf       time.Time           `json:"as_of" binding:"required"`
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

// @Summary getMySecurityEvents
// @Param cursor query string false "Continuation cursor"
// @Param limit query int false "Page size"
// @ID getMySecurityEvents
// @Tags identity
// @Accept json
// @Produce json
// @Success 200 {object} SecurityEventPageView
// @Failure 401 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Router /api/me/security-events [get]
func (h *EventHandler) self(c *gin.Context) { h.list("self")(c) }

// @Summary getTeamSecurityEvents
// @ID getTeamSecurityEvents
// @Tags identity
// @Accept json
// @Produce json
// @Param id path string true "Resource UUID"
// @Success 200 {object} SecurityEventPageView
// @Failure 401 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Router /api/teams/{id}/audit-events [get]
func (h *EventHandler) team(c *gin.Context) { h.list("team")(c) }

// @Summary getPlatformSecurityEvents
// @ID getPlatformSecurityEvents
// @Tags identity
// @Accept json
// @Produce json
// @Success 200 {object} SecurityEventPageView
// @Failure 401 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Router /api/platform/audit-events [get]
func (h *EventHandler) platform(c *gin.Context) { h.list("platform")(c) }
