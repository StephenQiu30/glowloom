package http

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// AccountDeletionHandler adapts authenticated self-deletion requests to the application.
type AccountDeletionHandler struct {
	service *application.AccountDeletion
	secure  bool
}

// NewAccountDeletionHandler constructs AccountDeletionHandler with explicit caller-supplied dependencies.
func NewAccountDeletionHandler(service *application.AccountDeletion, secure bool) *AccountDeletionHandler {
	return &AccountDeletionHandler{service: service, secure: secure}
}

// Register requires email-session middleware on the supplied group.
func (h *AccountDeletionHandler) Register(group *gin.RouterGroup) {
	group.POST("/me/deletion/preview", h.preview)
	group.POST("/me/deletion/challenge", h.start)
	group.POST("/me/deletion", h.confirm)
}

// DeletionImpactView exposes account version and duties that prevent deletion.
type DeletionImpactView struct {
	Revision       string      `json:"revision" binding:"required"`
	TeamIDs        []uuid.UUID `json:"team_ids" binding:"required"`
	BlockingDuties []string    `json:"blocking_duties" binding:"required"`
	PolicyVersion  string      `json:"policy_version" binding:"required"`
}

// DeletionConfirmRequest binds explicit consent to a version and an email operation.
type DeletionConfirmRequest struct {
	ExpectedRevision string    `json:"expected_revision" binding:"required"`
	OperationID      uuid.UUID `json:"operation_id" binding:"required"`
	Code             string    `json:"code" binding:"required"`
	PolicyVersion    string    `json:"policy_version" binding:"required"`
}

// @Summary previewAccountDeletion
// @ID previewAccountDeletion
// @Tags identity
// @Accept json
// @Produce json
// @Success 200 {object} DeletionImpactView
// @Failure 401 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Router /api/me/deletion/preview [post]
func (h *AccountDeletionHandler) preview(c *gin.Context) {
	impact, err := h.service.Preview(c.Request.Context(), Principal(c))
	if err != nil {
		emailAuthProblem(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(200, DeletionImpactView{Revision: strconv.FormatInt(impact.Revision, 10), TeamIDs: impact.TeamIDs, BlockingDuties: impact.BlockingDuties, PolicyVersion: application.SoftDeletionPolicy})
}

// @Summary startAccountDeletion
// @ID startAccountDeletion
// @Tags identity
// @Accept json
// @Produce json
// @Success 202 {object} AcceptedView
// @Failure 401 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Router /api/me/deletion/challenge [post]
func (h *AccountDeletionHandler) start(c *gin.Context) {
	result, err := h.service.Start(c.Request.Context(), Principal(c), requestSource(c))
	if err != nil {
		emailAuthProblem(c, err)
		return
	}
	c.JSON(202, AcceptedView{OperationID: result.OperationID, RetryAfterSeconds: result.RetryAfterSeconds})
}

// @Summary confirmAccountDeletion
// @ID confirmAccountDeletion
// @Tags identity
// @Accept json
// @Produce json
// @Param body body DeletionConfirmRequest true "Request"
// @Success 204
// @Failure 401 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Router /api/me/deletion [post]
func (h *AccountDeletionHandler) confirm(c *gin.Context) {
	var input DeletionConfirmRequest
	if !httpapi.Decode(c, &input) {
		return
	}
	revision, err := strconv.ParseInt(input.ExpectedRevision, 10, 64)
	if err != nil || revision < 1 || input.PolicyVersion != application.SoftDeletionPolicy || input.OperationID == uuid.Nil || len(input.Code) != 6 {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	for _, r := range input.Code {
		if r < '0' || r > '9' {
			httpapi.WriteProblem(c, 422, "invalid_request", nil)
			return
		}
	}
	if err := h.service.Confirm(c.Request.Context(), Principal(c), revision, input.OperationID, input.Code); err != nil {
		emailAuthProblem(c, err)
		return
	}
	http.SetCookie(c.Writer, &http.Cookie{Name: sessionCookie, Path: "/", HttpOnly: true, Secure: h.secure, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	c.Status(204)
}
