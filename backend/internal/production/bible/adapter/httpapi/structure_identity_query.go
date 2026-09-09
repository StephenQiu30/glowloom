package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"

	platformhttp "github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
)

type StructureIdentityQuery interface {
	GetCurrent(
		context.Context,
		application.Actor,
		string,
	) (application.StructureIdentitySnapshot, error)
}

type StructureIdentityHandler struct {
	query         StructureIdentityQuery
	authenticator Authenticator
}

func NewStructureIdentityHandler(
	query StructureIdentityQuery,
	authenticator Authenticator,
) *StructureIdentityHandler {
	return &StructureIdentityHandler{query: query, authenticator: authenticator}
}

func (handler *StructureIdentityHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/projects/{project_id}/structure-identity", handler.getCurrent)
}

func (handler *StructureIdentityHandler) getCurrent(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	writeProblem := func(code string, status int) {
		platformhttp.WriteProblem(writer, request, platformhttp.Problem{
			Code: code, Message: code, Status: status,
		})
	}
	claims, err := handler.authenticator.Authenticate(request)
	if err != nil {
		writeProblem("unauthenticated", http.StatusUnauthorized)
		return
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, 1))
	if err != nil || len(body) != 0 || request.URL.RawQuery != "" || request.URL.ForceQuery {
		writeProblem("validation_failed", http.StatusUnprocessableEntity)
		return
	}
	result, err := handler.query.GetCurrent(
		request.Context(),
		application.Actor{UserID: claims.UserID, TokenVersion: claims.TokenVersion},
		request.PathValue("project_id"),
	)
	if err != nil {
		var problem *application.Error
		if errors.As(err, &problem) {
			writeProblem(problem.Code, problem.Status)
		} else {
			writeProblem("internal_error", http.StatusInternalServerError)
		}
		return
	}
	platformhttp.WriteJSON(writer, http.StatusOK, map[string]any{"data": result})
}
