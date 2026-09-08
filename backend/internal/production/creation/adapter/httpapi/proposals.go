package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	platformhttp "github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	app "github.com/StephenQiu30/lanverse/backend/internal/production/creation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/creation/domain"
)

type ProposalService interface {
	Resume(context.Context, app.Actor, string, int64) (domain.ResumeReceipt, error)
	Sync(context.Context, app.Actor, string) (app.SyncResult, error)
	Execution(context.Context, app.Actor, string) (domain.ExecutionSnapshot, error)
	List(context.Context, app.Actor, string) ([]domain.Proposal, error)
	Get(context.Context, app.Actor, string, string) (domain.Proposal, error)
	Adopt(context.Context, app.Actor, string, string, app.AdoptCommand) (domain.AdoptionReceipt, error)
	Resolve(context.Context, string, string, app.ResolveCommand) (domain.GateResult, error)
}
type ProposalHandler struct {
	service ProposalService
	auth    Authenticator
	secret  []byte
	now     func() time.Time
}

func NewProposalHandler(service ProposalService, auth Authenticator, secret string, now func() time.Time) (*ProposalHandler, error) {
	if service == nil || auth == nil || len(secret) < 32 || now == nil {
		return nil, errors.New("creation proposal handler dependencies are incomplete")
	}
	return &ProposalHandler{service, auth, []byte(secret), now}, nil
}
func (h *ProposalHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/creation-runs/{run_id}/resume", h.resume)
	mux.HandleFunc("GET /api/creation-runs/{run_id}/execution", h.execution)
	mux.HandleFunc("POST /api/creation-runs/{run_id}/sync-proposals", h.sync)
	mux.HandleFunc("GET /api/creation-runs/{run_id}/proposals", h.list)
	mux.HandleFunc("GET /api/creation-runs/{run_id}/proposals/{proposal_id}", h.get)
	mux.HandleFunc("POST /api/creation-runs/{run_id}/proposals/{proposal_id}/adopt", h.adopt)
	mux.HandleFunc("POST /internal/creation/runs/{run_id}/gates/{gate}/resolve", h.resolve)
}

func (h *ProposalHandler) resume(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
	var input struct {
		ExpectedRevision int64 `json:"expected_revision"`
	}
	if err != nil || decodeSourceObject(body, &input) != nil {
		writeError(w, r, app.Problem("validation_failed", 422))
		return
	}
	value, err := h.service.Resume(r.Context(), actor, r.PathValue("run_id"), input.ExpectedRevision)
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	platformhttp.WriteJSON(w, http.StatusAccepted, map[string]any{"data": value})
}
func (h *ProposalHandler) actor(w http.ResponseWriter, r *http.Request) (app.Actor, bool) {
	claims, err := h.auth.Authenticate(r)
	if err != nil {
		writeError(w, r, app.Problem("unauthenticated", 401))
		return app.Actor{}, false
	}
	return app.Actor{UserID: claims.UserID, TokenVersion: claims.TokenVersion}, true
}
func proposalResponse(w http.ResponseWriter, r *http.Request, value any, err error) {
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	platformhttp.WriteJSON(w, http.StatusOK, map[string]any{"data": value})
}
func (h *ProposalHandler) execution(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	value, err := h.service.Execution(r.Context(), actor, r.PathValue("run_id"))
	proposalResponse(w, r, value, err)
}
func (h *ProposalHandler) sync(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1024))
	if err != nil {
		writeError(w, r, app.Problem("validation_failed", 422))
		return
	}
	var empty struct{}
	if len(body) > 0 && decodeSourceObject(body, &empty) != nil {
		writeError(w, r, app.Problem("validation_failed", 422))
		return
	}
	value, err := h.service.Sync(r.Context(), actor, r.PathValue("run_id"))
	proposalResponse(w, r, value, err)
}
func (h *ProposalHandler) list(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	value, err := h.service.List(r.Context(), actor, r.PathValue("run_id"))
	proposalResponse(w, r, value, err)
}
func (h *ProposalHandler) get(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	value, err := h.service.Get(r.Context(), actor, r.PathValue("run_id"), r.PathValue("proposal_id"))
	proposalResponse(w, r, value, err)
}
func (h *ProposalHandler) adopt(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1024*1024))
	var input app.AdoptCommand
	if err != nil || decodeSourceObject(body, &input) != nil {
		writeError(w, r, app.Problem("validation_failed", 422))
		return
	}
	value, err := h.service.Adopt(r.Context(), actor, r.PathValue("run_id"), r.PathValue("proposal_id"), input)
	proposalResponse(w, r, value, err)
}
func (h *ProposalHandler) resolve(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" || r.URL.ForceQuery {
		writeError(w, r, app.Problem("validation_failed", 422))
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1024*1024))
	if err != nil {
		writeError(w, r, app.Problem("request_too_large", 413))
		return
	}
	tokens := r.Header.Values(sourceAuthorizationHeader)
	if len(tokens) != 1 || !h.authorized(tokens[0], r.Method, r.URL.EscapedPath(), body) {
		writeError(w, r, app.Problem("unauthenticated", 401))
		return
	}
	var input app.ResolveCommand
	if decodeSourceObject(body, &input) != nil {
		writeError(w, r, app.Problem("validation_failed", 422))
		return
	}
	value, err := h.service.Resolve(r.Context(), r.PathValue("run_id"), r.PathValue("gate"), input)
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	platformhttp.WriteJSON(w, http.StatusOK, value)
}
func (h *ProposalHandler) authorized(token, method, path string, body []byte) bool {
	if len(token) > 4096 {
		return false
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return false
	}
	payload, err := base64.RawURLEncoding.Strict().DecodeString(parts[0])
	if err != nil {
		return false
	}
	signature, err := base64.RawURLEncoding.Strict().DecodeString(parts[1])
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, h.secret)
	_, _ = mac.Write([]byte(parts[0]))
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return false
	}
	var claims sourceAuthorization
	if decodeSourceObject(payload, &claims) != nil {
		return false
	}
	now := h.now().Unix()
	hash := sha256.Sum256(body)
	return claims.Audience == "lanverse.creation.platform" && claims.Method == method && claims.Path == path && claims.BodyHash == hex.EncodeToString(hash[:]) && claims.ExpiresAt > now && claims.ExpiresAt <= now+60
}
