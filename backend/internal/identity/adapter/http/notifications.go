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

// NotificationHandler exposes account inboxes and platform announcement management.
type NotificationHandler struct{ service *application.Notifications }

// NewNotificationHandler constructs the notification HTTP boundary.
func NewNotificationHandler(service *application.Notifications) *NotificationHandler {
	return &NotificationHandler{service: service}
}

// RegisterPublic exposes only audience-public effective announcements.
func (h *NotificationHandler) RegisterPublic(group *gin.RouterGroup) {
	group.GET("/announcements/public", h.public)
}

// Register must precede Creation scope middleware: inboxes are account-owned.
func (h *NotificationHandler) Register(group *gin.RouterGroup) {
	group.GET("/notifications", h.inbox)
	group.GET("/notifications/summary", h.summary)
	group.GET("/notifications/display", h.display)
	group.GET("/notifications/display/:id/:revision", h.validateDisplay)
	group.POST("/notifications/display/ack", h.ack)
	group.POST("/notifications/read-all", h.readAll)
	group.GET("/notifications/:id", h.detail)
	group.POST("/notifications/:id/read", h.read)
	group.GET("/platform/notifications", h.list)
	group.POST("/platform/notifications", h.create)
	group.GET("/platform/notifications/:id", h.get)
	group.PUT("/platform/notifications/:id", h.save)
	group.POST("/platform/notifications/:id/publish", h.publish)
	group.POST("/platform/notifications/:id/delete", h.deleteDraft)
	group.PATCH("/platform/notifications/:id/enabled", h.enabled)
	group.GET("/platform/notifications/:id/audit", h.audit)
}

func notificationProblem(c *gin.Context, err error) {
	switch {
	case errors.Is(err, application.ErrInvalidNotification):
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
	case errors.Is(err, application.ErrNotificationConflict):
		httpapi.WriteProblem(c, 409, "revision_conflict", nil)
	case errors.Is(err, application.ErrNotificationNotFound):
		httpapi.WriteProblem(c, 404, "not_found", nil)
	default:
		emailAuthProblem(c, err)
	}
}
func parseNotificationFilter(raw string, admin bool) (application.NotificationFilter, error) {
	f := application.NotificationFilter{}
	values, err := url.ParseQuery(raw)
	if err != nil {
		return f, application.ErrInvalidNotification
	}
	for k, vs := range values {
		if len(vs) != 1 {
			return f, application.ErrInvalidNotification
		}
		v := vs[0]
		switch k {
		case "cursor":
			f.Cursor = v
		case "limit":
			f.Limit, err = strconv.Atoi(v)
			if err != nil || f.Limit < 1 {
				return f, application.ErrInvalidNotification
			}
		case "from", "to":
			p, err := time.Parse(time.RFC3339Nano, v)
			if err != nil {
				return f, application.ErrInvalidNotification
			}
			if k == "from" {
				f.From = &p
			} else {
				f.To = &p
			}
		case "search", "placement", "audience", "status":
			if !admin {
				return f, application.ErrInvalidNotification
			}
			switch k {
			case "search":
				f.Search = v
			case "placement":
				f.Placement = v
			case "audience":
				f.Audience = v
			case "status":
				f.Status = v
			}
		case "category":
			if admin {
				return f, application.ErrInvalidNotification
			}
			f.Category = v
		case "unread_only":
			if admin || (v != "true" && v != "false") {
				return f, application.ErrInvalidNotification
			}
			f.UnreadOnly = v == "true"
		default:
			return f, application.ErrInvalidNotification
		}
	}
	return f, nil
}

// @Summary listNotifications
// @ID listNotifications
// @Tags notifications
// @Produce json
// @Param limit query int false "Default 20, maximum 100"
// @Param cursor query string false "Condition-bound continuation"
// @Param category query string false "interaction (collaboration), official (system/task/account), or a source category"
// @Param unread_only query boolean false "Unread only"
// @Param from query string false "Inclusive RFC3339 time"
// @Param to query string false "Exclusive RFC3339 time"
// @Success 200 {object} application.NotificationPage
// @Failure 401 {object} httpapi.Problem
// @Router /api/notifications [get]
func (h *NotificationHandler) inbox(c *gin.Context) {
	f, err := parseNotificationFilter(c.Request.URL.RawQuery, false)
	if err != nil {
		notificationProblem(c, err)
		return
	}
	out, err := h.service.Inbox(c.Request.Context(), Principal(c), f)
	if err != nil {
		notificationProblem(c, err)
		return
	}
	c.JSON(200, out)
}

// @Summary getNotificationSummary
// @ID getNotificationSummary
// @Tags notifications
// @Produce json
// @Success 200 {object} application.NotificationSummary
// @Failure 401 {object} httpapi.Problem
// @Router /api/notifications/summary [get]
func (h *NotificationHandler) summary(c *gin.Context) {
	out, err := h.service.Summary(c.Request.Context(), Principal(c))
	if err != nil {
		notificationProblem(c, err)
		return
	}
	c.JSON(200, out)
}

// @Summary getNotification
// @ID getNotification
// @Tags notifications
// @Produce json
// @Param id path string true "Inbox notification UUID"
// @Success 200 {object} application.InboxNotification
// @Failure 404 {object} httpapi.Problem
// @Router /api/notifications/{id} [get]
func (h *NotificationHandler) detail(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	out, err := h.service.Detail(c.Request.Context(), Principal(c), id)
	if err != nil {
		notificationProblem(c, err)
		return
	}
	c.JSON(200, out)
}

// @Summary readNotification
// @ID readNotification
// @Tags notifications
// @Accept json
// @Produce json
// @Param id path string true "Inbox notification UUID"
// @Param body body application.ReadNotificationInput true "Stable read command"
// @Success 200 {object} application.NotificationSummary
// @Router /api/notifications/{id}/read [post]
func (h *NotificationHandler) read(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var in application.ReadNotificationInput
	if !httpapi.Decode(c, &in) {
		return
	}
	out, err := h.service.Read(c.Request.Context(), Principal(c), id, application.ReadAllNotificationsInput{CommandID: in.CommandID})
	if err != nil {
		notificationProblem(c, err)
		return
	}
	c.JSON(200, out)
}

// @Summary readAllNotifications
// @ID readAllNotifications
// @Tags notifications
// @Accept json
// @Produce json
// @Param body body application.ReadAllNotificationsInput true "Snapshot watermark and stable command"
// @Success 200 {object} application.NotificationSummary
// @Failure 409 {object} httpapi.Problem
// @Router /api/notifications/read-all [post]
func (h *NotificationHandler) readAll(c *gin.Context) {
	var in application.ReadAllNotificationsInput
	if !httpapi.Decode(c, &in) {
		return
	}
	out, err := h.service.Read(c.Request.Context(), Principal(c), uuid.Nil, in)
	if err != nil {
		notificationProblem(c, err)
		return
	}
	c.JSON(200, out)
}

// @Summary getPublicAnnouncements
// @ID getPublicAnnouncements
// @Tags notifications
// @Produce json
// @Success 200 {object} application.NotificationDisplayView
// @Router /api/announcements/public [get]
func (h *NotificationHandler) public(c *gin.Context) {
	out, err := h.service.Display(c.Request.Context(), nil)
	if err != nil {
		notificationProblem(c, err)
		return
	}
	c.JSON(200, out)
}

// @Summary getNotificationDisplay
// @ID getNotificationDisplay
// @Tags notifications
// @Produce json
// @Success 200 {object} application.NotificationDisplayView
// @Router /api/notifications/display [get]
func (h *NotificationHandler) display(c *gin.Context) {
	actor := Principal(c)
	out, err := h.service.Display(c.Request.Context(), &actor)
	if err != nil {
		notificationProblem(c, err)
		return
	}
	c.JSON(200, out)
}

// @Summary validateNotificationDisplay
// @ID validateNotificationDisplay
// @Tags notifications
// @Produce json
// @Param id path string true "Announcement UUID"
// @Param revision path string true "Published revision, decimal string"
// @Success 200 {object} application.NotificationDisplayValidity
// @Failure 401 {object} httpapi.Problem
// @Router /api/notifications/display/{id}/{revision} [get]
func (h *NotificationHandler) validateDisplay(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	raw := c.Param("revision")
	revision, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || revision < 1 || strconv.FormatInt(revision, 10) != raw {
		notificationProblem(c, application.ErrInvalidNotification)
		return
	}
	out, err := h.service.ValidateDisplay(c.Request.Context(), Principal(c), id, revision)
	if err != nil {
		notificationProblem(c, err)
		return
	}
	c.JSON(200, out)
}

// @Summary ackNotificationDisplay
// @ID ackNotificationDisplay
// @Tags notifications
// @Accept json
// @Param body body application.NotificationDisplayAck true "Display receipt, independent of read state"
// @Success 204
// @Router /api/notifications/display/ack [post]
func (h *NotificationHandler) ack(c *gin.Context) {
	var in application.NotificationDisplayAck
	if !httpapi.Decode(c, &in) {
		return
	}
	if err := h.service.Ack(c.Request.Context(), Principal(c), in); err != nil {
		notificationProblem(c, err)
		return
	}
	c.Status(204)
}

// @Summary listPlatformNotifications
// @ID listPlatformNotifications
// @Tags notifications
// @Produce json
// @Param limit query int false "Default 20, maximum 100"
// @Param cursor query string false "Condition-bound continuation"
// @Param search query string false "Title keyword"
// @Param placement query string false "banner, modal, inbox, sidebar, sidebar_badge"
// @Param audience query string false "public, authenticated, team"
// @Param status query string false "draft, scheduled, active, expired, disabled"
// @Param from query string false "Inclusive effective start time"
// @Param to query string false "Exclusive effective start time"
// @Success 200 {object} application.AnnouncementPage
// @Failure 403 {object} httpapi.Problem
// @Router /api/platform/notifications [get]
func (h *NotificationHandler) list(c *gin.Context) {
	f, err := parseNotificationFilter(c.Request.URL.RawQuery, true)
	if err != nil {
		notificationProblem(c, err)
		return
	}
	out, err := h.service.List(c.Request.Context(), Principal(c), f)
	if err != nil {
		notificationProblem(c, err)
		return
	}
	c.JSON(200, out)
}

// @Summary getPlatformNotification
// @ID getPlatformNotification
// @Tags notifications
// @Produce json
// @Param id path string true "Announcement UUID"
// @Success 200 {object} application.Announcement
// @Router /api/platform/notifications/{id} [get]
func (h *NotificationHandler) get(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	out, err := h.service.Get(c.Request.Context(), Principal(c), id)
	if err != nil {
		notificationProblem(c, err)
		return
	}
	c.JSON(200, out)
}

// @Summary createPlatformNotification
// @ID createPlatformNotification
// @Tags notifications
// @Accept json
// @Produce json
// @Param body body application.SaveAnnouncementInput true "Draft content and stable command UUID"
// @Success 201 {object} application.Announcement
// @Router /api/platform/notifications [post]
func (h *NotificationHandler) create(c *gin.Context) { h.saveDraft(c, uuid.Nil) }

// @Summary savePlatformNotification
// @ID savePlatformNotification
// @Tags notifications
// @Accept json
// @Produce json
// @Param id path string true "Announcement UUID"
// @Param body body application.SaveAnnouncementInput true "Draft revision with expected version"
// @Success 200 {object} application.Announcement
// @Failure 409 {object} httpapi.Problem
// @Router /api/platform/notifications/{id} [put]
func (h *NotificationHandler) save(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	h.saveDraft(c, id)
}
func (h *NotificationHandler) saveDraft(c *gin.Context, id uuid.UUID) {
	var in application.SaveAnnouncementInput
	if !httpapi.Decode(c, &in) {
		return
	}
	out, err := h.service.Save(c.Request.Context(), Principal(c), id, in)
	if err != nil {
		notificationProblem(c, err)
		return
	}
	status := 200
	if id == uuid.Nil {
		status = 201
	}
	c.JSON(status, out)
}

// PublishAnnouncementRequest deliberately excludes enable state.
type PublishAnnouncementRequest struct {
	CommandID       uuid.UUID `json:"command_id"`
	ExpectedVersion int64     `json:"expected_version,string" swaggertype:"string"`
}

// EnableAnnouncementRequest requires an explicit desired state and expected revision.
type EnableAnnouncementRequest struct {
	CommandID       uuid.UUID `json:"command_id"`
	ExpectedVersion int64     `json:"expected_version,string" swaggertype:"string"`
	Enabled         *bool     `json:"enabled"`
}

// @Summary publishPlatformNotification
// @ID publishPlatformNotification
// @Tags notifications
// @Accept json
// @Produce json
// @Param id path string true "Announcement UUID"
// @Param body body PublishAnnouncementRequest true "Publish the saved draft"
// @Success 200 {object} application.Announcement
// @Failure 409 {object} httpapi.Problem
// @Router /api/platform/notifications/{id}/publish [post]
func (h *NotificationHandler) publish(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var in PublishAnnouncementRequest
	if !httpapi.Decode(c, &in) {
		return
	}
	out, err := h.service.Command(c.Request.Context(), Principal(c), id, "publish", application.AnnouncementCommand{CommandID: in.CommandID, ExpectedVersion: in.ExpectedVersion})
	if err != nil {
		notificationProblem(c, err)
		return
	}
	c.JSON(200, out)
}

// @Summary deletePlatformNotification
// @ID deletePlatformNotification
// @Tags notifications
// @Accept json
// @Produce json
// @Param id path string true "Never-published announcement UUID"
// @Param body body PublishAnnouncementRequest true "Stable delete command and expected version"
// @Success 200 {object} application.Announcement
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Router /api/platform/notifications/{id}/delete [post]
func (h *NotificationHandler) deleteDraft(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var in PublishAnnouncementRequest
	if !httpapi.Decode(c, &in) {
		return
	}
	out, err := h.service.Command(c.Request.Context(), Principal(c), id, "delete", application.AnnouncementCommand{CommandID: in.CommandID, ExpectedVersion: in.ExpectedVersion})
	if err != nil {
		notificationProblem(c, err)
		return
	}
	c.JSON(200, out)
}

// @Summary setPlatformNotificationEnabled
// @ID setPlatformNotificationEnabled
// @Tags notifications
// @Accept json
// @Produce json
// @Param id path string true "Announcement UUID"
// @Param body body EnableAnnouncementRequest true "Desired enable state and expected version"
// @Success 200 {object} application.Announcement
// @Failure 409 {object} httpapi.Problem
// @Router /api/platform/notifications/{id}/enabled [patch]
func (h *NotificationHandler) enabled(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var in EnableAnnouncementRequest
	if !httpapi.Decode(c, &in) {
		return
	}
	if in.Enabled == nil {
		notificationProblem(c, application.ErrInvalidNotification)
		return
	}
	out, err := h.service.Command(c.Request.Context(), Principal(c), id, "enabled", application.AnnouncementCommand{CommandID: in.CommandID, ExpectedVersion: in.ExpectedVersion, Enabled: *in.Enabled})
	if err != nil {
		notificationProblem(c, err)
		return
	}
	c.JSON(200, out)
}

// @Summary getPlatformNotificationAudit
// @ID getPlatformNotificationAudit
// @Tags notifications
// @Produce json
// @Param id path string true "Announcement UUID"
// @Success 200 {object} application.NotificationAuditPage
// @Router /api/platform/notifications/{id}/audit [get]
func (h *NotificationHandler) audit(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	out, err := h.service.Audit(c.Request.Context(), Principal(c), id)
	if err != nil {
		notificationProblem(c, err)
		return
	}
	c.JSON(200, out)
}
