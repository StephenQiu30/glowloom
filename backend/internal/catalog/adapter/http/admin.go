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
	"github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
	identityhttp "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/http"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
)

// AdminDependencies injects existing audited catalog commands and safe queries.
type AdminDependencies struct {
	Providers         *application.ListProvidersQuery
	Provider          *application.ProviderDetailQuery
	CreateProvider    *application.CreateProviderCommand
	UpdateProvider    *application.UpdateProviderCommand
	SetCredential     *application.SetCredentialCommand
	DisableCredential *application.DisableCredentialCommand
	CredentialTest    *application.RequestCredentialTestCommand
	Models            *application.AdminModelsQuery
	CreateModel       *application.CreateModelCommand
	PublishVersion    *application.PublishModelVersionCommand
	PublishPrice      *application.PublishPriceRuleCommand
	SetModelStatus    *application.SetModelStatusCommand
}

// AdminHandler exposes management without elevating the local workspace actor.
type AdminHandler struct{ deps AdminDependencies }

// NewAdminHandler injects the catalog's real management use cases.
func NewAdminHandler(deps AdminDependencies) *AdminHandler { return &AdminHandler{deps: deps} }

// Register installs administrator reads, immutable publications, and guarded updates.
func (h *AdminHandler) Register(group *gin.RouterGroup) {
	admin := group.Group("/admin")
	admin.Use(func(c *gin.Context) {
		actor := identityhttp.Principal(c)
		if actor.ID == uuid.Nil || actor.OrgID == uuid.Nil || actor.Role != identitydomain.RoleAdmin || actor.MustChangePassword {
			httpapi.WriteProblem(c, 403, "forbidden", nil)
			c.Abort()
			return
		}
		c.Header("Cache-Control", "private, no-store")
	})
	admin.GET("/providers", h.ListProviders)
	admin.POST("/providers", h.CreateProvider)
	admin.GET("/providers/:id", h.Provider)
	admin.PATCH("/providers/:id", h.UpdateProvider)
	admin.PUT("/providers/:id/credentials", h.SetCredential)
	admin.POST("/providers/:id/credentials/:credential_id/disable", h.DisableCredential)
	admin.POST("/providers/:id/credentials/:credential_id/test", h.TestCredential)
	admin.GET("/capabilities", h.Capabilities)
	admin.GET("/models", h.ListModels)
	admin.POST("/models", h.CreateModel)
	admin.GET("/models/:id", h.Model)
	admin.POST("/models/:id/versions", h.PublishVersion)
	admin.POST("/models/:id/prices", h.PublishPrice)
	admin.PATCH("/models/:id/status", h.SetModelStatus)
}

// ProviderPage is a bounded page of safe channel settings.
type ProviderPage struct {
	Items      []application.ProviderListItem `json:"items"`
	NextCursor *string                        `json:"next_cursor" extensions:"x-nullable"`
}

// AdminModelPage is the public management list, including disabled models.
type AdminModelPage struct {
	Items      []application.CreatedModel `json:"items"`
	NextCursor *string                    `json:"next_cursor" extensions:"x-nullable"`
}

type adminCursor struct {
	ID         uuid.UUID `json:"id"`
	Key        string    `json:"key,omitempty"`
	CreateTime time.Time `json:"create_time,omitempty"`
	Binding    string    `json:"binding"`
}

func adminPage(c *gin.Context, kind string) (int, *adminCursor, string, bool) {
	limit := 50
	if raw := c.Query("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 200 {
			httpapi.WriteProblem(c, 422, "invalid_request", nil)
			return 0, nil, "", false
		}
		limit = value
	}
	actor := identityhttp.Principal(c)
	digest := sha256.Sum256([]byte(actor.ID.String() + "/" + actor.OrgID.String() + "/" + kind + "/" + strconv.Itoa(limit)))
	binding := hex.EncodeToString(digest[:])
	if raw := c.Query("cursor"); raw != "" {
		body, err := base64.RawURLEncoding.DecodeString(raw)
		var cursor adminCursor
		if len(raw) > 1024 || err != nil || json.Unmarshal(body, &cursor) != nil || cursor.ID == uuid.Nil || cursor.Binding != binding || (kind == "models" && cursor.Key == "") || (kind == "providers" && cursor.CreateTime.IsZero()) {
			httpapi.WriteProblem(c, 422, "invalid_request", nil)
			return 0, nil, "", false
		}
		return limit, &cursor, binding, true
	}
	return limit, nil, binding, true
}

func encodeAdminCursor(cursor adminCursor) (*string, error) {
	body, err := json.Marshal(cursor)
	if err != nil {
		return nil, err
	}
	encoded := base64.RawURLEncoding.EncodeToString(body)
	return &encoded, nil
}

func adminID(c *gin.Context, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(name))
	if err != nil || id == uuid.Nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return uuid.Nil, false
	}
	return id, true
}

func adminError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, identityapp.ErrForbidden):
		httpapi.WriteProblem(c, 403, "forbidden", nil)
	case errors.Is(err, application.ErrProviderNotFound), errors.Is(err, application.ErrCredentialNotFound), errors.Is(err, application.ErrModelNotFound):
		httpapi.WriteProblem(c, 404, "not_found", nil)
	case errors.Is(err, domain.ErrProviderRevisionConflict), errors.Is(err, domain.ErrModelRevisionConflict):
		httpapi.WriteProblem(c, 409, "revision_conflict", nil)
	case errors.Is(err, application.ErrProviderKeyExists), errors.Is(err, application.ErrModelKeyExists), errors.Is(err, application.ErrModelVersionConflict), errors.Is(err, application.ErrPriceVersionConflict), errors.Is(err, application.ErrCredentialTestKeyReused):
		httpapi.WriteProblem(c, 409, "catalog_conflict", nil)
	case errors.Is(err, application.ErrProviderUnavailable), errors.Is(err, application.ErrModelSourceUnavailable), errors.Is(err, domain.ErrProviderDisabled), errors.Is(err, domain.ErrModelNotPublishable):
		httpapi.WriteProblem(c, 409, "catalog_unavailable", nil)
	case errors.Is(err, application.ErrInvalidProviderList), errors.Is(err, application.ErrInvalidProviderDetail), errors.Is(err, application.ErrInvalidAdminModels), errors.Is(err, application.ErrInvalidCreateProvider), errors.Is(err, application.ErrInvalidUpdateProvider), errors.Is(err, application.ErrInvalidSetCredential), errors.Is(err, application.ErrInvalidDisableCredential), errors.Is(err, application.ErrInvalidCredentialTest), errors.Is(err, application.ErrInvalidCreateModel), errors.Is(err, application.ErrInvalidPublishModelVersion), errors.Is(err, application.ErrInvalidPublishPriceRule), errors.Is(err, application.ErrInvalidSetModelStatus), errors.Is(err, domain.ErrInvalidProvider), errors.Is(err, domain.ErrInvalidModel), errors.Is(err, domain.ErrInvalidModelVersion), errors.Is(err, domain.ErrInvalidPriceRule):
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
	default:
		httpapi.WriteProblem(c, 503, "dependency_unavailable", nil)
	}
}

// ListProviders lists safe channel and credential metadata.
// @Summary 管理员渠道列表
// @ID listAdminProviders
// @Tags settings
// @Produce json
// @Param limit query integer false "1..200"
// @Param cursor query string false "绑定管理员与分页的不透明游标"
// @Success 200 {object} ProviderPage
// @Failure 403 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Router /api/admin/providers [get]
func (h *AdminHandler) ListProviders(c *gin.Context) {
	limit, cursor, binding, ok := adminPage(c, "providers")
	if !ok {
		return
	}
	input := application.ListProvidersInput{Limit: limit}
	if cursor != nil {
		input.After = &application.ProviderListCursor{ID: cursor.ID, CreateTime: cursor.CreateTime}
	}
	page, err := h.deps.Providers.Execute(c.Request.Context(), identityhttp.Principal(c), input)
	if err != nil {
		adminError(c, err)
		return
	}
	result := ProviderPage{Items: page.Providers}
	if page.Next != nil {
		result.NextCursor, err = encodeAdminCursor(adminCursor{ID: page.Next.ID, CreateTime: page.Next.CreateTime, Binding: binding})
		if err != nil {
			adminError(c, err)
			return
		}
	}
	c.JSON(200, result)
}

// CreateProviderRequest declares one supported channel, without credential values.
type CreateProviderRequest struct {
	Key              string        `json:"key"`
	Name             string        `json:"name"`
	AdapterKey       string        `json:"adapter_key"`
	Region           domain.Region `json:"region"`
	ConcurrencyLimit int           `json:"concurrency_limit"`
	RateLimitPerMin  int           `json:"rate_limit_per_min"`
}

// CreateProvider registers an audited channel.
// @Summary 创建渠道
// @ID createAdminProvider
// @Tags settings
// @Accept json
// @Produce json
// @Param Idempotency-Key header string true "请求UUID"
// @Param body body CreateProviderRequest true "渠道"
// @Success 201 {object} application.CreatedProvider
// @Failure 403 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Router /api/admin/providers [post]
func (h *AdminHandler) CreateProvider(c *gin.Context) {
	var body CreateProviderRequest
	if !httpapi.Decode(c, &body) {
		return
	}
	result, err := h.deps.CreateProvider.Execute(c.Request.Context(), identityhttp.Principal(c), application.CreateProviderInput{Key: body.Key, Name: body.Name, AdapterKey: body.AdapterKey, Region: body.Region, ConcurrencyLimit: body.ConcurrencyLimit, RateLimitPerMin: body.RateLimitPerMin, RequestID: c.GetHeader("Idempotency-Key")})
	if err != nil {
		adminError(c, err)
		return
	}
	c.JSON(201, result)
}

// Provider reads saved safe summaries and declared credential fields.
// @Summary 渠道详情与凭据字段
// @ID getAdminProvider
// @Tags settings
// @Produce json
// @Param id path string true "渠道UUID"
// @Success 200 {object} application.ProviderDetail
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Router /api/admin/providers/{id} [get]
func (h *AdminHandler) Provider(c *gin.Context) {
	id, ok := adminID(c, "id")
	if !ok {
		return
	}
	result, err := h.deps.Provider.Execute(c.Request.Context(), identityhttp.Principal(c), id)
	if err != nil {
		adminError(c, err)
		return
	}
	c.JSON(200, result)
}

// UpdateProviderRequest applies a guarded enable/disable or rate setting.
type UpdateProviderRequest struct {
	ExpectedRevision int64                  `json:"expected_revision"`
	Status           *domain.ProviderStatus `json:"status,omitempty"`
	ConcurrencyLimit *int                   `json:"concurrency_limit,omitempty"`
	RateLimitPerMin  *int                   `json:"rate_limit_per_min,omitempty"`
}

// UpdateProvider commits settings with optimistic revision and safe audit.
// @Summary 更新或禁用渠道
// @ID updateAdminProvider
// @Tags settings
// @Accept json
// @Produce json
// @Param id path string true "渠道UUID"
// @Param Idempotency-Key header string true "请求UUID"
// @Param body body UpdateProviderRequest true "设置与预期revision"
// @Success 200 {object} application.CreatedProvider
// @Failure 403 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Router /api/admin/providers/{id} [patch]
func (h *AdminHandler) UpdateProvider(c *gin.Context) {
	id, ok := adminID(c, "id")
	if !ok {
		return
	}
	var body UpdateProviderRequest
	if !httpapi.Decode(c, &body) {
		return
	}
	result, err := h.deps.UpdateProvider.Execute(c.Request.Context(), identityhttp.Principal(c), application.UpdateProviderInput{ProviderID: id, ExpectedRevision: body.ExpectedRevision, Status: body.Status, ConcurrencyLimit: body.ConcurrencyLimit, RateLimitPerMin: body.RateLimitPerMin, RequestID: c.GetHeader("Idempotency-Key")})
	if err != nil {
		adminError(c, err)
		return
	}
	c.JSON(200, result)
}

// SetCredentialRequest is a one-time plaintext input; responses contain only last4.
type SetCredentialRequest struct {
	Label  string          `json:"label"`
	Secret json.RawMessage `json:"secret" swaggertype:"object"`
}

// SetCredential seals the submitted fields before persistence.
// @Summary 替换渠道凭据
// @ID setAdminCredential
// @Tags settings
// @Accept json
// @Produce json
// @Param id path string true "渠道UUID"
// @Param Idempotency-Key header string true "请求UUID"
// @Param body body SetCredentialRequest true "凭据字段"
// @Success 200 {object} application.SavedCredential
// @Failure 403 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/admin/providers/{id}/credentials [put]
func (h *AdminHandler) SetCredential(c *gin.Context) {
	id, ok := adminID(c, "id")
	if !ok {
		return
	}
	var body SetCredentialRequest
	if !httpapi.Decode(c, &body) {
		return
	}
	defer clear(body.Secret)
	if h.deps.SetCredential == nil {
		httpapi.WriteProblem(c, 503, "credential_sealing_unavailable", nil)
		return
	}
	result, err := h.deps.SetCredential.Execute(c.Request.Context(), identityhttp.Principal(c), application.SetCredentialInput{ProviderID: id, Label: body.Label, Secret: body.Secret, RequestID: c.GetHeader("Idempotency-Key")})
	if err != nil {
		adminError(c, err)
		return
	}
	c.JSON(200, result)
}

// DisableCredential revokes one still-current credential with an audit event.
// @Summary 禁用渠道当前凭据
// @ID disableAdminCredential
// @Tags settings
// @Produce json
// @Param id path string true "渠道UUID"
// @Param credential_id path string true "凭据UUID"
// @Param Idempotency-Key header string true "请求UUID"
// @Success 200 {object} application.SavedCredential
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Router /api/admin/providers/{id}/credentials/{credential_id}/disable [post]
func (h *AdminHandler) DisableCredential(c *gin.Context) {
	id, ok := adminID(c, "id")
	if !ok {
		return
	}
	credential, ok := adminID(c, "credential_id")
	if !ok {
		return
	}
	result, err := h.deps.DisableCredential.Execute(c.Request.Context(), identityhttp.Principal(c), application.DisableCredentialInput{ProviderID: id, CredentialID: credential, RequestID: c.GetHeader("Idempotency-Key")})
	if err != nil {
		adminError(c, err)
		return
	}
	c.JSON(200, result)
}

// TestCredential records a request consumed by the existing test workflow.
// @Summary 请求测试渠道凭据
// @ID testAdminCredential
// @Tags settings
// @Produce json
// @Param id path string true "渠道UUID"
// @Param credential_id path string true "凭据UUID"
// @Param Idempotency-Key header string true "请求UUID"
// @Success 202 {object} application.CredentialTestAccepted
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Router /api/admin/providers/{id}/credentials/{credential_id}/test [post]
func (h *AdminHandler) TestCredential(c *gin.Context) {
	id, ok := adminID(c, "id")
	if !ok {
		return
	}
	credential, ok := adminID(c, "credential_id")
	if !ok {
		return
	}
	result, err := h.deps.CredentialTest.Execute(c.Request.Context(), identityhttp.Principal(c), id, credential, c.GetHeader("Idempotency-Key"))
	if err != nil {
		adminError(c, err)
		return
	}
	c.JSON(202, result)
}

// Capabilities returns real capability declarations for model creation.
// @Summary 模型能力声明目录
// @ID listAdminCapabilities
// @Tags settings
// @Produce json
// @Success 200 {array} application.CapabilitySummary
// @Failure 403 {object} httpapi.Problem
// @Router /api/admin/capabilities [get]
func (h *AdminHandler) Capabilities(c *gin.Context) {
	result, err := h.deps.Models.Capabilities(c.Request.Context(), identityhttp.Principal(c))
	if err != nil {
		adminError(c, err)
		return
	}
	c.JSON(200, result)
}

// ListModels returns all configured model identities for settings.
// @Summary 管理员模型目录
// @ID listAdminModels
// @Tags settings
// @Produce json
// @Param limit query integer false "1..200"
// @Param cursor query string false "分页游标"
// @Success 200 {object} AdminModelPage
// @Failure 403 {object} httpapi.Problem
// @Router /api/admin/models [get]
func (h *AdminHandler) ListModels(c *gin.Context) {
	limit, cursor, binding, ok := adminPage(c, "models")
	if !ok {
		return
	}
	input := application.AdminModelListInput{Limit: limit}
	if cursor != nil {
		input.After = &application.ModelCatalogCursor{ID: cursor.ID, Key: cursor.Key}
	}
	page, err := h.deps.Models.List(c.Request.Context(), identityhttp.Principal(c), input)
	if err != nil {
		adminError(c, err)
		return
	}
	result := AdminModelPage{Items: page.Items}
	if page.Next != nil {
		result.NextCursor, err = encodeAdminCursor(adminCursor{ID: page.Next.ID, Key: page.Next.Key, Binding: binding})
		if err != nil {
			adminError(c, err)
			return
		}
	}
	c.JSON(200, result)
}

// CreateModelRequest registers a disabled model before version publication.
type CreateModelRequest struct {
	Key         string    `json:"model_key"`
	ProviderID  uuid.UUID `json:"provider_id"`
	Capability  string    `json:"capability"`
	DisplayName string    `json:"display_name"`
}

// CreateModel registers a model with the current audit and administrator contract.
// @Summary 创建模型
// @ID createAdminModel
// @Tags settings
// @Accept json
// @Produce json
// @Param Idempotency-Key header string true "请求UUID"
// @Param body body CreateModelRequest true "模型"
// @Success 201 {object} application.CreatedModel
// @Failure 403 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Router /api/admin/models [post]
func (h *AdminHandler) CreateModel(c *gin.Context) {
	var body CreateModelRequest
	if !httpapi.Decode(c, &body) {
		return
	}
	result, err := h.deps.CreateModel.Execute(c.Request.Context(), identityhttp.Principal(c), application.CreateModelInput{Key: body.Key, ProviderID: body.ProviderID, Capability: body.Capability, DisplayName: body.DisplayName, RequestID: c.GetHeader("Idempotency-Key")})
	if err != nil {
		adminError(c, err)
		return
	}
	c.JSON(201, result)
}

// Model returns the full immutable version and price history needed by the editor.
// @Summary 模型配置与价格版本详情
// @ID getAdminModel
// @Tags settings
// @Produce json
// @Param id path string true "模型UUID"
// @Success 200 {object} application.AdminModelDetail
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Router /api/admin/models/{id} [get]
func (h *AdminHandler) Model(c *gin.Context) {
	id, ok := adminID(c, "id")
	if !ok {
		return
	}
	result, err := h.deps.Models.Detail(c.Request.Context(), identityhttp.Principal(c), id)
	if err != nil {
		adminError(c, err)
		return
	}
	c.JSON(200, result)
}

// PublishVersionRequest declares a reviewed replacement as an immutable version.
type PublishVersionRequest struct {
	ExpectedRevision int64             `json:"expected_revision"`
	VersionNo        int               `json:"version_no"`
	ProviderModelID  string            `json:"provider_model_id"`
	Modes            []string          `json:"modes"`
	Limits           json.RawMessage   `json:"limits" swaggertype:"object"`
	ParamSchema      json.RawMessage   `json:"param_schema" swaggertype:"array,object"`
	SupportsQuery    bool              `json:"supports_query"`
	SupportsCancel   bool              `json:"supports_cancel"`
	SupportsCallback bool              `json:"supports_callback"`
	ExpectedMaxMS    int               `json:"expected_max_ms"`
	Moderation       domain.Moderation `json:"moderation"`
	Queue            string            `json:"queue"`
}

// PublishVersion validates the real parameter schema and publishes the next version.
// @Summary 发布模型配置版本
// @ID publishAdminModelVersion
// @Tags settings
// @Accept json
// @Produce json
// @Param id path string true "模型UUID"
// @Param Idempotency-Key header string true "请求UUID"
// @Param body body PublishVersionRequest true "配置"
// @Success 201 {object} application.PublishedModelVersion
// @Failure 403 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Router /api/admin/models/{id}/versions [post]
func (h *AdminHandler) PublishVersion(c *gin.Context) {
	id, ok := adminID(c, "id")
	if !ok {
		return
	}
	var b PublishVersionRequest
	if !httpapi.Decode(c, &b) {
		return
	}
	result, err := h.deps.PublishVersion.Execute(c.Request.Context(), identityhttp.Principal(c), application.PublishModelVersionInput{ModelID: id, ExpectedRevision: b.ExpectedRevision, VersionNo: b.VersionNo, ProviderModelID: b.ProviderModelID, Modes: b.Modes, Limits: b.Limits, ParamSchema: b.ParamSchema, SupportsQuery: b.SupportsQuery, SupportsCancel: b.SupportsCancel, SupportsCallback: b.SupportsCallback, ExpectedMaxMS: b.ExpectedMaxMS, Moderation: b.Moderation, Queue: b.Queue, RequestID: c.GetHeader("Idempotency-Key")})
	if err != nil {
		adminError(c, err)
		return
	}
	c.JSON(201, result)
}

// PublishPriceRequest publishes the exact price coefficients and effective date.
type PublishPriceRequest struct {
	ExpectedRevision int64            `json:"expected_revision"`
	VersionNo        int              `json:"version_no"`
	Unit             domain.PriceUnit `json:"unit"`
	Rule             json.RawMessage  `json:"rule" swaggertype:"object"`
	Currency         string           `json:"currency"`
	FXRateToCNY      string           `json:"fx_rate_to_cny"`
	EffectiveFrom    time.Time        `json:"effective_from"`
}

// PublishPrice appends a validated price and preserves existing operation snapshots.
// @Summary 发布模型价格版本
// @ID publishAdminModelPrice
// @Tags settings
// @Accept json
// @Produce json
// @Param id path string true "模型UUID"
// @Param Idempotency-Key header string true "请求UUID"
// @Param body body PublishPriceRequest true "价格"
// @Success 201 {object} application.PublishedPriceRule
// @Failure 403 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Router /api/admin/models/{id}/prices [post]
func (h *AdminHandler) PublishPrice(c *gin.Context) {
	id, ok := adminID(c, "id")
	if !ok {
		return
	}
	var b PublishPriceRequest
	if !httpapi.Decode(c, &b) {
		return
	}
	result, err := h.deps.PublishPrice.Execute(c.Request.Context(), identityhttp.Principal(c), application.PublishPriceRuleInput{ModelID: id, ExpectedRevision: b.ExpectedRevision, VersionNo: b.VersionNo, Unit: b.Unit, Rule: b.Rule, Currency: b.Currency, FXRateToCNY: b.FXRateToCNY, EffectiveFrom: b.EffectiveFrom, RequestID: c.GetHeader("Idempotency-Key")})
	if err != nil {
		adminError(c, err)
		return
	}
	c.JSON(201, result)
}

// SetModelStatusRequest toggles availability with a current revision.
type SetModelStatusRequest struct {
	ExpectedRevision int64              `json:"expected_revision"`
	Status           domain.ModelStatus `json:"status"`
}

// SetModelStatus enables or disables a configured model using existing preconditions.
// @Summary 启用或禁用模型
// @ID setAdminModelStatus
// @Tags settings
// @Accept json
// @Produce json
// @Param id path string true "模型UUID"
// @Param Idempotency-Key header string true "请求UUID"
// @Param body body SetModelStatusRequest true "状态"
// @Success 200 {object} application.ChangedModelStatus
// @Failure 403 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Router /api/admin/models/{id}/status [patch]
func (h *AdminHandler) SetModelStatus(c *gin.Context) {
	id, ok := adminID(c, "id")
	if !ok {
		return
	}
	var b SetModelStatusRequest
	if !httpapi.Decode(c, &b) {
		return
	}
	result, err := h.deps.SetModelStatus.Execute(c.Request.Context(), identityhttp.Principal(c), application.SetModelStatusInput{ModelID: id, ExpectedRevision: b.ExpectedRevision, Status: b.Status, RequestID: c.GetHeader("Idempotency-Key")})
	if err != nil {
		adminError(c, err)
		return
	}
	c.JSON(200, result)
}
