// Package http exposes project-authorized model configuration without credentials.
package http

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/catalog/application"
	identityhttp "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/http"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
)

// Handler adapts the current-project model query to HTTP.
type Handler struct{ models *application.ListModelsQuery }

// NewHandler injects the query that rechecks current actor and project rights.
func NewHandler(models *application.ListModelsQuery) *Handler { return &Handler{models: models} }

// Register installs project-scoped model reads; administrator writes are separate.
func (h *Handler) Register(group *gin.RouterGroup) { group.GET("/models", h.ListModels) }

// ModelVersion contains the immutable form configuration.
type ModelVersion struct {
	ID          uuid.UUID       `json:"id"`
	VersionNo   int             `json:"version_no"`
	Modes       []string        `json:"modes"`
	Limits      json.RawMessage `json:"limits" swaggertype:"object"`
	ParamSchema json.RawMessage `json:"param_schema" swaggertype:"array,object"`
}

// ModelPrice exposes the currently effective price for estimates.
type ModelPrice struct {
	ID            uuid.UUID       `json:"id"`
	VersionNo     int             `json:"version_no"`
	Unit          string          `json:"unit"`
	Rule          json.RawMessage `json:"rule" swaggertype:"object"`
	Currency      string          `json:"currency"`
	FXRateToCNY   *string         `json:"fx_rate_to_cny" extensions:"x-nullable"`
	EffectiveFrom time.Time       `json:"effective_from"`
}

// ModelResponse is safe display data, excluding provider URLs and credentials.
type ModelResponse struct {
	ID                   uuid.UUID     `json:"id"`
	Key                  string        `json:"key"`
	DisplayName          string        `json:"display_name"`
	Capability           string        `json:"capability"`
	InputRoles           []string      `json:"input_roles"`
	Status               string        `json:"status"`
	ProviderID           uuid.UUID     `json:"provider_id"`
	ProviderName         string        `json:"provider_name"`
	Region               string        `json:"region"`
	ProviderStatus       string        `json:"provider_status"`
	CredentialPresent    bool          `json:"credential_present"`
	CredentialTestResult *string       `json:"credential_test_result" extensions:"x-nullable"`
	CurrentVersion       *ModelVersion `json:"current_version" extensions:"x-nullable"`
	CurrentPrice         *ModelPrice   `json:"current_price" extensions:"x-nullable"`
}

// ModelPage is one bounded catalog page.
type ModelPage struct {
	Items      []ModelResponse `json:"items"`
	NextCursor *string         `json:"next_cursor" extensions:"x-nullable"`
}

type modelCursor struct {
	Key     string    `json:"key"`
	ID      uuid.UUID `json:"id"`
	Binding string    `json:"binding"`
}

// ListModels returns models available to the project's organization and region.
// @Summary 项目模型目录与参数表单
// @ID listProjectModels
// @Tags models
// @Produce json
// @Param project_id query string true "项目UUID"
// @Param capability query string false "能力"
// @Param mode query string false "生成模式"
// @Param limit query integer false "1..200，默认50"
// @Param cursor query string false "绑定项目与筛选的不透明游标"
// @Success 200 {object} ModelPage
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/models [get]
func (h *Handler) ListModels(c *gin.Context) {
	project, err := uuid.Parse(c.Query("project_id"))
	if err != nil || project == uuid.Nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	input := application.ListModelsInput{ProjectID: project, Capability: c.Query("capability"), Mode: c.Query("mode"), Limit: 50}
	if raw := c.Query("limit"); raw != "" {
		input.Limit, err = strconv.Atoi(raw)
		if err != nil || input.Limit < 1 || input.Limit > 200 {
			httpapi.WriteProblem(c, 422, "invalid_request", nil)
			return
		}
	}
	actor := identityhttp.Principal(c)
	encoded, err := json.Marshal(struct {
		Actor, Organization, Project uuid.UUID
		Capability, Mode             string
		Limit                        int
	}{actor.ID, actor.OrgID, project, input.Capability, input.Mode, input.Limit})
	if err != nil {
		httpapi.WriteProblem(c, 503, "dependency_unavailable", nil)
		return
	}
	digest := sha256.Sum256(encoded)
	binding := hex.EncodeToString(digest[:])
	if raw := c.Query("cursor"); raw != "" {
		var cur modelCursor
		body, decodeErr := base64.RawURLEncoding.DecodeString(raw)
		if len(raw) > 1024 || decodeErr != nil || len(body) > 768 || json.Unmarshal(body, &cur) != nil || cur.ID == uuid.Nil || cur.Key == "" || cur.Binding != binding {
			httpapi.WriteProblem(c, 422, "invalid_request", nil)
			return
		}
		input.After = &application.ModelCatalogCursor{Key: cur.Key, ID: cur.ID}
	}
	page, err := h.models.Execute(c.Request.Context(), actor, input)
	if err != nil {
		switch {
		case errors.Is(err, identityapp.ErrForbidden):
			httpapi.WriteProblem(c, 403, "forbidden", nil)
		case errors.Is(err, application.ErrModelCatalogProjectNotFound):
			httpapi.WriteProblem(c, 404, "not_found", nil)
		case errors.Is(err, application.ErrInvalidModelCatalog):
			httpapi.WriteProblem(c, 422, "invalid_request", nil)
		default:
			httpapi.WriteProblem(c, 503, "dependency_unavailable", nil)
		}
		return
	}
	response := ModelPage{Items: make([]ModelResponse, 0, len(page.Models))}
	for _, model := range page.Models {
		item := ModelResponse{ID: model.ID, Key: model.Key, DisplayName: model.DisplayName, Capability: model.Capability, Status: string(model.Status), ProviderID: model.ProviderID,
			InputRoles:   append([]string{}, model.InputRoles...),
			ProviderName: model.ProviderName, Region: string(model.ProviderRegion), ProviderStatus: string(model.ProviderStatus), CredentialPresent: model.CredentialPresent}
		if model.CredentialTestResult != nil {
			result := string(*model.CredentialTestResult)
			item.CredentialTestResult = &result
		}
		if v := model.CurrentVersion; v != nil {
			item.CurrentVersion = &ModelVersion{ID: v.ID, VersionNo: v.VersionNo, Modes: v.Modes, Limits: v.Limits, ParamSchema: v.ParamSchema}
		}
		if p := model.CurrentPrice; p != nil {
			item.CurrentPrice = &ModelPrice{ID: p.ID, VersionNo: p.VersionNo, Unit: p.Unit, Rule: p.Rule, Currency: p.Currency, FXRateToCNY: p.FXRateToCNY, EffectiveFrom: p.EffectiveFrom}
		}
		response.Items = append(response.Items, item)
	}
	if page.Next != nil {
		body, encodeErr := json.Marshal(modelCursor{Key: page.Next.Key, ID: page.Next.ID, Binding: binding})
		if encodeErr != nil {
			httpapi.WriteProblem(c, 503, "dependency_unavailable", nil)
			return
		}
		value := base64.RawURLEncoding.EncodeToString(body)
		response.NextCursor = &value
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, response)
}
