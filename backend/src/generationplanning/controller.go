package generationplanning

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/stephenqiu30/lanverse/backend/src/platform/httpapi"
)

type GenerationPlanController struct{ service *GenerationPlanService }

func NewGenerationPlanController(service *GenerationPlanService) *GenerationPlanController {
	return &GenerationPlanController{service: service}
}

type createRequest struct {
	ProjectID  uuid.UUID `json:"project_id"`
	TargetType string    `json:"target_type"`
	TargetID   uuid.UUID `json:"target_id"`
	Prompt     string    `json:"prompt"`
	Capability string    `json:"capability_key"`
	Count      int       `json:"count"`
}
type approveRequest struct {
	ExecutionDisposition string      `json:"execution_disposition"`
	SelectedItemIDs      []uuid.UUID `json:"selected_item_ids"`
}

func (h *GenerationPlanController) Mount(r chi.Router) {
	r.Post("/api/generation-plans", h.create)
	r.Get("/api/generation-plans/{planID}", h.get)
	r.Post("/api/generation-plans/{planID}/preflight", h.preflight)
	r.Post("/api/generation-plans/{planID}/approve", h.approve)
}
func (h *GenerationPlanController) Router() http.Handler { r := chi.NewRouter(); h.Mount(r); return r }
func (h *GenerationPlanController) create(w http.ResponseWriter, r *http.Request) {
	var b createRequest
	if !decode(w, r, &b) {
		return
	}
	p, i, e := h.service.Create(r.Context(), CreateInput{ProjectID: b.ProjectID, TargetType: b.TargetType, TargetID: b.TargetID, Prompt: strings.TrimSpace(b.Prompt), Capability: strings.TrimSpace(b.Capability), Count: b.Count})
	if e != nil {
		httpapi.WriteError(w, r, e)
		return
	}
	httpapi.WriteData(w, httpapi.StatusCreated, map[string]any{"plan": p, "items": i})
}
func (h *GenerationPlanController) get(w http.ResponseWriter, r *http.Request) {
	id, e := uuid.Parse(chi.URLParam(r, "planID"))
	if e != nil {
		httpapi.WriteError(w, r, httpapi.NewError(httpapi.StatusBadRequest, httpapi.CodeInvalidID, "生成计划 ID 无效", "检查 URL 中的资源 ID"))
		return
	}
	p, i, e := h.service.Get(r.Context(), id)
	if e != nil {
		httpapi.WriteError(w, r, e)
		return
	}
	httpapi.WriteData(w, httpapi.StatusOK, map[string]any{"plan": p, "items": i})
}
func (h *GenerationPlanController) preflight(w http.ResponseWriter, r *http.Request) {
	id, e := uuid.Parse(chi.URLParam(r, "planID"))
	if e != nil {
		httpapi.WriteError(w, r, httpapi.NewError(httpapi.StatusBadRequest, httpapi.CodeInvalidID, "生成计划 ID 无效", "检查 URL 中的资源 ID"))
		return
	}
	p, i, e := h.service.Preflight(r.Context(), id)
	if e != nil {
		httpapi.WriteError(w, r, e)
		return
	}
	httpapi.WriteData(w, httpapi.StatusOK, map[string]any{"plan": p, "items": i})
}
func (h *GenerationPlanController) approve(w http.ResponseWriter, r *http.Request) {
	id, e := uuid.Parse(chi.URLParam(r, "planID"))
	if e != nil {
		httpapi.WriteError(w, r, httpapi.NewError(httpapi.StatusBadRequest, httpapi.CodeInvalidID, "生成计划 ID 无效", "检查 URL 中的资源 ID"))
		return
	}
	var b approveRequest
	if !decode(w, r, &b) {
		return
	}
	p, i, e := h.service.Approve(r.Context(), id, b.ExecutionDisposition, b.SelectedItemIDs)
	if e != nil {
		httpapi.WriteError(w, r, e)
		return
	}
	httpapi.WriteData(w, httpapi.StatusOK, map[string]any{"plan": p, "items": i})
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	return httpapi.DecodeJSON(w, r, v, 2<<20)
}
