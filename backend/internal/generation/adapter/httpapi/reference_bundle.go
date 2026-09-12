package httpapi

import (
	"context"
	"io"
	"net/http"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	platformhttp "github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
)

type ReferenceBundleQuery interface {
	Get(context.Context, application.Actor, string, string) (domain.ReferenceBundleInputCollection, error)
}
type ReferenceBundleHandler struct {
	query         ReferenceBundleQuery
	authenticator Authenticator
}

func NewReferenceBundleHandler(query ReferenceBundleQuery, authenticator Authenticator) *ReferenceBundleHandler {
	return &ReferenceBundleHandler{query, authenticator}
}
func (handler *ReferenceBundleHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/projects/{project_id}/reference-executions/{execution_id}/bundle-inputs", handler.get)
}
func (handler *ReferenceBundleHandler) get(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	claims, err := handler.authenticator.Authenticate(request)
	if err != nil {
		writeError(writer, request, &application.Error{Code: "unauthenticated", Message: "Invalid credentials", Status: 401})
		return
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, 1))
	if err != nil || len(body) != 0 || request.URL.RawQuery != "" || request.URL.ForceQuery {
		writeError(writer, request, &application.Error{Code: "validation_failed", Message: "Reference Bundle query does not accept a body or selector", Status: 422})
		return
	}
	value, err := handler.query.Get(request.Context(), application.Actor{UserID: claims.UserID, TokenVersion: claims.TokenVersion}, request.PathValue("project_id"), request.PathValue("execution_id"))
	if err != nil {
		writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, http.StatusOK, map[string]any{"data": value})
}
