package http

import (
	"context"
	"errors"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// VideoLinkImporter returns only actual authorized personal-library media.
type VideoLinkImporter interface {
	Import(context.Context, identity.Principal, uuid.UUID, uuid.UUID, application.VideoLinkRequest) (application.PersonalUploadResult, error)
}

// VideoLinkHandler bounds simultaneous synchronous public link imports.
type VideoLinkHandler struct {
	service VideoLinkImporter
	slots   chan struct{}
}

// NewVideoLinkHandler permits two concurrent imports per application process.
func NewVideoLinkHandler(service VideoLinkImporter) *VideoLinkHandler {
	return &VideoLinkHandler{service: service, slots: make(chan struct{}, 2)}
}

// Register installs the authenticated personal-library route.
func (h *VideoLinkHandler) Register(group *gin.RouterGroup) {
	group.POST("/media/library/video-links", h.Import)
}

// Import resolves public share metadata and uploads verified original bytes.
// @Summary 读取公共视频分享链接并存入个人素材库
// @ID importPersonalVideoLink
// @Tags media
// @Accept json
// @Produce json
// @Param Idempotency-Key header string true "UUID永久幂等键"
// @Param request body application.VideoLinkRequest true "仅抖音、头条、西瓜公开HTTPS分享页，须明确确认内容与使用权限；无法公开读取时使用本地上传"
// @Success 201 {object} application.PersonalUploadResult
// @Failure 403 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 413 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 429 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/media/library/video-links [post]
func (h *VideoLinkHandler) Import(c *gin.Context) {
	if h == nil || h.service == nil {
		httpapi.WriteProblem(c, 503, "video_link_unavailable", nil)
		return
	}
	if len(c.Request.URL.Query()) != 0 {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	key, err := uuid.Parse(c.GetHeader("Idempotency-Key"))
	if err != nil || key == uuid.Nil {
		httpapi.WriteProblem(c, 422, "invalid_idempotency_key", nil)
		return
	}
	requestID, err := uuid.Parse(c.GetString("request_id"))
	if err != nil || requestID == uuid.Nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	var input application.VideoLinkRequest
	if !httpapi.Decode(c, &input) {
		return
	}
	if !input.LocalReviewConfirmed {
		httpapi.WriteProblem(c, 422, "invalid_video_link", nil)
		return
	}
	select {
	case h.slots <- struct{}{}:
		defer func() { <-h.slots }()
	default:
		httpapi.WriteProblem(c, 429, "video_link_busy", nil)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Minute)
	defer cancel()
	result, err := h.service.Import(ctx, identityhttp.Principal(c), key, requestID, input)
	if err != nil {
		switch {
		case errors.Is(err, application.ErrInvalidVideoLink):
			httpapi.WriteProblem(c, 422, "invalid_video_link", nil)
		case errors.Is(err, application.ErrVideoLinkUnresolvable), errors.Is(err, application.ErrUnsupportedUpload):
			httpapi.WriteProblem(c, 422, "video_link_unresolvable", nil)
		case errors.Is(err, application.ErrVideoLinkUnavailable), errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			httpapi.WriteProblem(c, 503, "video_link_unavailable", nil)
		default:
			uploadError(c, err)
		}
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(201, result)
}
