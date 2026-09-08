package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"

	platformhttp "github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	app "github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
)

type TextWorldQuery interface {
	Get(context.Context, app.Actor, string, string) (domain.TextWorldVersion, error)
}
type TextWorldHandler struct {
	query TextWorldQuery
	auth  Authenticator
}

func NewTextWorldHandler(query TextWorldQuery, auth Authenticator) *TextWorldHandler {
	return &TextWorldHandler{query: query, auth: auth}
}
func (h *TextWorldHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/projects/{project_id}/text-world-versions/{version_id}", h.get)
}
func (h *TextWorldHandler) get(w http.ResponseWriter, r *http.Request) {
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
