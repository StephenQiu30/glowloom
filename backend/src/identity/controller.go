package identity

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/stephenqiu30/lanverse/backend/src/platform/httpapi"
)

type IdentityController struct{ service *IdentityService }

func NewIdentityController(service *IdentityService) *IdentityController {
	return &IdentityController{service: service}
}

type createSessionRequest struct {
	IdentitySubject string    `json:"identity_subject"`
	WorkspaceID     uuid.UUID `json:"workspace_id"`
}

func (h *IdentityController) CreateSession(w http.ResponseWriter, r *http.Request) {
	var body createSessionRequest
	if !httpapi.DecodeJSON(w, r, &body, 64<<10) {
		return
	}
	session, err := h.service.CreateSession(r.Context(), body.IdentitySubject, body.WorkspaceID)
	if err != nil {
		httpapi.WriteError(w, r, httpapi.Wrap(err, httpapi.StatusUnprocessableEntity, httpapi.CodeSessionInvalid, err.Error(), "确认身份主体和 Workspace 后重试"))
		return
	}
	httpapi.WriteData(w, httpapi.StatusCreated, session)
}
