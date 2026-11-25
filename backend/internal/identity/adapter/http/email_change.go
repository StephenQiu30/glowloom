package http

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// EmailChangeHandler exposes authenticated mailbox replacement operations.
type EmailChangeHandler struct {
	service *application.EmailChange
	secure  bool
}

// NewEmailChangeHandler injects the account use case and session cookie policy.
func NewEmailChangeHandler(s *application.EmailChange, secure bool) *EmailChangeHandler {
	return &EmailChangeHandler{service: s, secure: secure}
}

// Register requires current email-session and CSRF middleware on group.
func (h *EmailChangeHandler) Register(group *gin.RouterGroup) {
	group.POST("/me/email-change", h.start)
	group.POST("/me/email-change/:id/verify", h.verify)
	group.POST("/me/email-change/:id/complete", h.complete)
	group.DELETE("/me/email-change/:id", h.cancel)
}

// EmailChangeStartRequest binds the new mailbox to the visible account version.
type EmailChangeStartRequest struct {
	ExpectedRevision string `json:"expected_revision" binding:"required"`
	NewEmail         string `json:"new_email" binding:"required"`
}

// EmailChangeVerifyRequest selects exactly one side of the operation.
type EmailChangeVerifyRequest struct {
	Side string `json:"side" binding:"required"`
	Code string `json:"code" binding:"required"`
}

// EmailChangeCompleteRequest supplies the unchanged account version.
type EmailChangeCompleteRequest struct {
	ExpectedRevision string `json:"expected_revision" binding:"required"`
}

func emailChangeProblem(c *gin.Context, err error) {
	if errors.Is(err, application.ErrEmailUnavailable) {
		httpapi.WriteProblem(c, 409, "email_unavailable", nil)
		return
	}
	emailAuthProblem(c, err)
}

// @Summary startEmailChange
// @ID startEmailChange
// @Tags identity
// @Accept json
// @Produce json
// @Param body body EmailChangeStartRequest true "Request"
// @Success 202 {object} AcceptedView
// @Failure 401 {object} httpapi.Problem
// @Failure 403 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 429 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/me/email-change [post]
func (h *EmailChangeHandler) start(c *gin.Context) {
	var in EmailChangeStartRequest
	if !httpapi.Decode(c, &in) {
		return
	}
	rev, err := strconv.ParseInt(in.ExpectedRevision, 10, 64)
	if err != nil || rev < 1 {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	op, err := h.service.StartFrom(c.Request.Context(), Principal(c), in.NewEmail, rev, requestSource(c))
	if err != nil {
		emailChangeProblem(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(202, AcceptedView{OperationID: op, RetryAfterSeconds: 60})
}

// @Summary verifyEmailChange
// @ID verifyEmailChange
// @Tags identity
// @Accept json
// @Param id path string true "Operation UUID"
// @Param body body EmailChangeVerifyRequest true "Request"
// @Success 204
// @Failure 401 {object} httpapi.Problem
// @Failure 403 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/me/email-change/{id}/verify [post]
func (h *EmailChangeHandler) verify(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var in EmailChangeVerifyRequest
	if !httpapi.Decode(c, &in) {
		return
	}
	if (in.Side != "old" && in.Side != "new") || len(in.Code) != 6 {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	for _, r := range in.Code {
		if r < '0' || r > '9' {
			httpapi.WriteProblem(c, 422, "invalid_request", nil)
			return
		}
	}
	if err := h.service.Verify(c.Request.Context(), Principal(c), id, in.Side, in.Code); err != nil {
		emailChangeProblem(c, err)
		return
	}
	c.Status(204)
}

// @Summary completeEmailChange
// @ID completeEmailChange
// @Tags identity
// @Accept json
// @Param id path string true "Operation UUID"
// @Param body body EmailChangeCompleteRequest true "Request"
// @Success 204
// @Failure 401 {object} httpapi.Problem
// @Failure 403 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/me/email-change/{id}/complete [post]
func (h *EmailChangeHandler) complete(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var in EmailChangeCompleteRequest
	if !httpapi.Decode(c, &in) {
		return
	}
	rev, err := strconv.ParseInt(in.ExpectedRevision, 10, 64)
	if err != nil || rev < 1 {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	if err := h.service.Complete(c.Request.Context(), Principal(c), id, rev); err != nil {
		emailChangeProblem(c, err)
		return
	}
	clearSessionCookies(c, h.secure)
	c.Status(204)
}

// @Summary cancelEmailChange
// @ID cancelEmailChange
// @Tags identity
// @Param id path string true "Operation UUID"
// @Success 204
// @Failure 401 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/me/email-change/{id} [delete]
func (h *EmailChangeHandler) cancel(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok || id == uuid.Nil {
		return
	}
	if err := h.service.Cancel(c.Request.Context(), Principal(c), id); err != nil {
		emailChangeProblem(c, err)
		return
	}
	c.Status(204)
}
