package httpapi

import (
	"net/http"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/cost/application"
	"github.com/StephenQiu30/lanverse/backend/internal/cost/domain"
)

type setPriceQuoteRequest struct {
	UnitAmount       string `json:"unit_amount" validate:"required"`
	Currency         string `json:"currency" validate:"required,len=3,uppercase"`
	ExpectedRevision int64  `json:"expected_revision" validate:"gte=0"`
	IdempotencyKey   string `json:"idempotency_key" validate:"required,max=200"`
}

type priceQuoteResponse struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	ProjectID   string    `json:"project_id"`
	Metric      string    `json:"metric"`
	UnitAmount  string    `json:"unit_amount"`
	Currency    string    `json:"currency"`
	Revision    int64     `json:"revision"`
	CreatedBy   string    `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
}

func (handler *Handler) getPriceQuote(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	quote, err := handler.service.GetCurrentPriceQuote(
		request.Context(), actor, request.PathValue("project_id"), request.PathValue("metric"),
	)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"data": presentPriceQuote(quote)})
}

func (handler *Handler) setPriceQuote(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	var payload setPriceQuoteRequest
	if !handler.decode(writer, request, &payload) {
		return
	}
	result, err := handler.service.SetPriceQuote(request.Context(), actor, application.SetPriceQuoteCommand{
		ProjectID: request.PathValue("project_id"), Metric: request.PathValue("metric"),
		UnitAmount: payload.UnitAmount, Currency: payload.Currency,
		ExpectedRevision: payload.ExpectedRevision, IdempotencyKey: payload.IdempotencyKey,
	})
	if err != nil {
		writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"data": presentPriceQuote(result.Quote)})
}

func presentPriceQuote(quote domain.PriceQuote) priceQuoteResponse {
	return priceQuoteResponse{
		ID: quote.ID, WorkspaceID: quote.WorkspaceID, ProjectID: quote.ProjectID, Metric: quote.Metric,
		UnitAmount: quote.UnitAmount.StringFixed(6), Currency: quote.Currency, Revision: quote.Revision,
		CreatedBy: quote.CreatedBy, CreatedAt: quote.CreatedAt,
	}
}
