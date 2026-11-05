package http

import (
	"encoding/json"

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
	g.POST("/input-sources/validate", func(c *gin.Context) {
		var in struct {
			Grant domain.Grant    `json:"grant"`
			Draft json.RawMessage `json:"draft"`
		}
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
		send(c, gin.H{"valid": true}, err)
	})
}
