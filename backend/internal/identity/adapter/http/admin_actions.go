package http

import (
	"github.com/gin-gonic/gin"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// AdminRevisionRequest identifies the exact account state the administrator reviewed.
type AdminRevisionRequest struct {
	ExpectedRevision int64 `json:"expected_revision"`
}

// Disable revokes account access through the existing transactional command.
// @ID disableAdminUser
// @Accept json
// @Produce json
// @Param id path string true "User UUID"
// @Param body body AdminRevisionRequest true "Expected revision"
// @Success 200 {object} AdminMutationReceipt
// @Failure 409 {object} httpapi.Problem
// @Router /api/admin/users/{id}/disable [post]
func (h *AdminHandler) Disable(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var body AdminRevisionRequest
	if !httpapi.Decode(c, &body) {
		return
	}
	if id == Principal(c).ID {
		httpapi.WriteProblem(c, 409, "self_management_forbidden", nil)
		return
	}
	u, err := h.deps.Disable.Execute(c.Request.Context(), Principal(c), application.DisableUserInput{TargetID: id, ExpectedRevision: body.ExpectedRevision, RequestID: c.GetString("request_id")})
	if err != nil {
		adminProblem(c, err)
		return
	}
	c.JSON(200, AdminMutationReceipt{ID: u.ID, Revision: u.Revision, Status: string(u.Status)})
}

// Enable restores availability while preserving the existing session invalidation behavior.
// @ID enableAdminUser
// @Accept json
// @Produce json
// @Param id path string true "User UUID"
// @Param body body AdminRevisionRequest true "Expected revision"
// @Success 200 {object} AdminMutationReceipt
// @Failure 409 {object} httpapi.Problem
// @Router /api/admin/users/{id}/enable [post]
func (h *AdminHandler) Enable(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var body AdminRevisionRequest
	if !httpapi.Decode(c, &body) {
		return
	}
	u, err := h.deps.Enable.Execute(c.Request.Context(), Principal(c), application.EnableUserInput{TargetID: id, ExpectedRevision: body.ExpectedRevision, RequestID: c.GetString("request_id")})
	if err != nil {
		adminProblem(c, err)
		return
	}
	c.JSON(200, AdminMutationReceipt{ID: u.ID, Revision: u.Revision, Status: string(u.Status)})
}

// AdminPasswordRequest carries a temporary replacement only for the current request.
type AdminPasswordRequest struct {
	ExpectedRevision int64  `json:"expected_revision"`
	NewPassword      string `json:"new_password"`
}

// ResetPassword invalidates target sessions and requires a password change at next login.
// @ID resetAdminPassword
// @Accept json
// @Produce json
// @Param id path string true "User UUID"
// @Param body body AdminPasswordRequest true "Temporary password and expected revision"
// @Success 200 {object} AdminMutationReceipt
// @Failure 409 {object} httpapi.Problem
// @Router /api/admin/users/{id}/password [post]
func (h *AdminHandler) ResetPassword(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var body AdminPasswordRequest
	if !httpapi.Decode(c, &body) {
		return
	}
	if id == Principal(c).ID {
		httpapi.WriteProblem(c, 409, "self_management_forbidden", nil)
		return
	}
	u, err := h.deps.Reset.Execute(c.Request.Context(), Principal(c), application.ResetPasswordInput{TargetID: id, ExpectedRevision: body.ExpectedRevision, NewPassword: body.NewPassword, RequestID: c.GetString("request_id")})
	if err != nil {
		adminProblem(c, err)
		return
	}
	c.JSON(200, AdminMutationReceipt{ID: u.ID, Revision: u.Revision, MustChangePassword: u.MustChangePassword})
}

// Unlock clears only the failed-login lock.
// @ID unlockAdminUser
// @Accept json
// @Produce json
// @Param id path string true "User UUID"
// @Param body body AdminRevisionRequest true "Expected revision"
// @Success 200 {object} AdminMutationReceipt
// @Failure 409 {object} httpapi.Problem
// @Router /api/admin/users/{id}/unlock [post]
func (h *AdminHandler) Unlock(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var body AdminRevisionRequest
	if !httpapi.Decode(c, &body) {
		return
	}
	u, err := h.deps.Unlock.Execute(c.Request.Context(), Principal(c), application.UnlockUserInput{TargetID: id, ExpectedRevision: body.ExpectedRevision, RequestID: c.GetString("request_id")})
	if err != nil {
		adminProblem(c, err)
		return
	}
	c.JSON(200, AdminMutationReceipt{ID: u.ID, Revision: u.Revision, Status: string(u.Status), LoginUnlocked: true})
}
