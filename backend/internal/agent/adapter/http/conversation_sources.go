package http

import (
	"github.com/gin-gonic/gin"

	app "github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	prompt "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
)

// WithInputSources injects current resource authority for every model egress.
func (h *BusinessHandler) WithInputSources(sources app.ConversationSourceReader) *BusinessHandler {
	h.inputSources = sources
	return h
}
func (h *BusinessHandler) registerInputSources(g *gin.RouterGroup) {
	g.POST("/input-sources/validate", h.validateInputSources)
}

// @Summary validateInputSources
// @ID validateInputSources
// @Tags business
// @Accept json
// @Produce json
// @Security ServiceIdentity
// @Param X-Agent-Audience header string true "Agent audience"
// @Param body body businessInputSources true "Request"
// @Success 200 {object} businessValid
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 409 {object} map[string]interface{}
// @Failure 422 {object} map[string]interface{}
// @Failure 503 {object} map[string]interface{}
// @Router /internal/agent-business/input-sources/validate [post]
func (h *BusinessHandler) validateInputSources(c *gin.Context) {
	var in businessInputSources
	if !decodeConversation(c, &in) {
		return
	}
	grant, err := h.store.ValidateGrant(c.Request.Context(), in.Grant)
	if err != nil {
		send(c, nil, err)
		return
	}
	if h.inputSources == nil {
		send(c, nil, domain.ErrUnavailable)
		return
	}
	draft, err := prompt.DecodeDraft(in.Draft)
	if err != nil {
		send(c, nil, err)
		return
	}
	err = h.inputSources.CheckConversationSources(c.Request.Context(), grant.Scope.Principal(), grant.Scope.ProjectID, draft)
	send(c, businessValid{Valid: true}, err)
}
