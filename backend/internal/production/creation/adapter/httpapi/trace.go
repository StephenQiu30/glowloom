package httpapi

import (
	"context"
	"io"
	"net/http"

	app "github.com/StephenQiu30/lanverse/backend/internal/production/creation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/creation/domain"
)

type TraceService interface {
	Manifest(context.Context, app.Actor, string) (domain.ManifestSnapshot, error)
	Attempts(context.Context, app.Actor, string, string) (domain.AttemptHistory, error)
}
type TraceHandler struct {
	service TraceService
	auth    Authenticator
}

func NewTraceHandler(service TraceService, auth Authenticator) *TraceHandler {
	return &TraceHandler{service: service, auth: auth}
}
func (h *TraceHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/creation-runs/{run_id}/manifest", h.manifest)
	mux.HandleFunc("GET /api/creation-runs/{run_id}/steps/{step_id}/attempts", h.attempts)
}
func (h *TraceHandler) actor(w http.ResponseWriter, r *http.Request) (app.Actor, bool) {
	w.Header().Set("Cache-Control", "no-store")
	claims, err := h.auth.Authenticate(r)
	if err != nil {
		writeError(w, r, app.Problem("unauthenticated", 401))
		return app.Actor{}, false
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1))
	if err != nil || len(body) != 0 || r.URL.RawQuery != "" || r.URL.ForceQuery {
		writeError(w, r, app.Problem("validation_failed", 422))
		return app.Actor{}, false
	}
	return app.Actor{UserID: claims.UserID, TokenVersion: claims.TokenVersion}, true
}
func (h *TraceHandler) manifest(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	v, e := h.service.Manifest(r.Context(), actor, r.PathValue("run_id"))
	proposalResponse(w, r, v, e)
}
func (h *TraceHandler) attempts(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	v, e := h.service.Attempts(r.Context(), actor, r.PathValue("run_id"), r.PathValue("step_id"))
	proposalResponse(w, r, v, e)
}
