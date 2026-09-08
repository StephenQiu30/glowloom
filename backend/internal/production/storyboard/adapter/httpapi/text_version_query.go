package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"

	platformhttp "github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	app "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/domain"
)

type TextIntentQuery interface {
	Get(context.Context, app.Actor, string, string) (domain.TextIntentVersion, error)
}
type TextIntentHandler struct {
	query TextIntentQuery
	auth  Authenticator
}

func NewTextIntentHandler(query TextIntentQuery, auth Authenticator) *TextIntentHandler {
	return &TextIntentHandler{query: query, auth: auth}
}
func (h *TextIntentHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/projects/{project_id}/text-intent-versions/{version_id}", h.get)
}
func (h *TextIntentHandler) get(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeProblem := func(code string, status int) {
		platformhttp.WriteProblem(w, r, platformhttp.Problem{Code: code, Message: code, Status: status})
	}
	claims, err := h.auth.Authenticate(r)
	if err != nil {
		writeProblem("unauthenticated", 401)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1))
	if err != nil || len(body) != 0 || r.URL.RawQuery != "" || r.URL.ForceQuery {
		writeProblem("validation_failed", 422)
		return
	}
	value, err := h.query.Get(r.Context(), app.Actor{UserID: claims.UserID, TokenVersion: claims.TokenVersion}, r.PathValue("project_id"), r.PathValue("version_id"))
	if err != nil {
		var problem *app.Error
		if errors.As(err, &problem) {
			writeProblem(problem.Code, problem.Status)
		} else {
			writeProblem("internal_error", 500)
		}
		return
	}
	platformhttp.WriteJSON(w, http.StatusOK, map[string]any{"data": value})
}
