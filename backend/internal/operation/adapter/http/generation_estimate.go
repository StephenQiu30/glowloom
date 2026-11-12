package http

import (
	"context"
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
	promptdomain "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
)

// GenerationEstimator reads the current authorized input and price estimate.
type GenerationEstimator interface {
	EstimateGeneration(context.Context, identityapp.Principal, uuid.UUID, promptdomain.Draft, bool) (application.GenerationEstimate, error)
}

// GenerationEstimateRequest carries the editor draft and reuse preference.
type GenerationEstimateRequest struct {
	Draft           promptdomain.Draft `json:"draft"`
	ForceRegenerate bool               `json:"force_regenerate,omitempty"`
}

// EstimateGeneration returns a read-only preview for the current editor input.
// @Summary 预估生成积分
// @ID estimateGeneration
// @Tags operations
// @Accept json
// @Produce json
// @Param pid path string true "项目UUID"
// @Param body body GenerationEstimateRequest true "当前编辑输入"
// @Success 200 {object} application.GenerationEstimate
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Router /api/projects/{pid}/generation-estimate [post]
func (h *Handler) EstimateGeneration(c *gin.Context) {
	project, ok := httpapi.PathUUID(c, "pid")
	if !ok {
		return
	}
	var body GenerationEstimateRequest
	if !httpapi.Decode(c, &body) {
		return
	}
	result, err := h.dependencies.Estimate.EstimateGeneration(c.Request.Context(), identityhttp.Principal(c), project, body.Draft, body.ForceRegenerate)
	if errors.Is(err, promptdomain.ErrInvalidDraft) || errors.Is(err, promptdomain.ErrUnsupportedSchema) {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	if err != nil {
		writeError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(200, result)
}
