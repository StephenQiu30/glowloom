package http

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// RoutePoolHandler serves administrator reads and publications.
type RoutePoolHandler struct{ Store application.RoutePoolStore }

// Register installs both routes behind the identity middleware.
func (h RoutePoolHandler) Register(group *gin.RouterGroup) {
	group.PUT("/admin/providers/:id/credentials/:credential_id/pool", h.Key)
	group.GET("/admin/model-releases/:id/route-pool", h.Read)
	group.PUT("/admin/model-releases/:id/route-pool", h.Publish)
}

// Read lists the ordered backups for an active release.
// @Summary 查看模型备用线路
// @ID readAdminRoutePool
// @Tags settings
// @Param id path string true "主线路发布编号"
// @Success 200 {object} application.RoutePool
// @Router /api/admin/model-releases/{id}/route-pool [get]
func (h RoutePoolHandler) Read(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	v, err := h.Store.ReadRoutePool(c.Request.Context(), identityhttp.Principal(c), id)
	if err != nil {
		presetError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(200, v)
}

// Publish writes a new version, preserving quotes already issued.
// @Summary 发布模型备用线路
// @ID publishAdminRoutePool
// @Tags settings
// @Accept json
// @Param id path string true "主线路发布编号"
// @Param body body application.PoolPublication true "备用线路"
// @Success 200 {object} application.RoutePool
// @Router /api/admin/model-releases/{id}/route-pool [put]
func (h RoutePoolHandler) Publish(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var in application.PoolPublication
	if !httpapi.Decode(c, &in) {
		return
	}
	v, err := h.Store.PublishRoutePool(c.Request.Context(), identityhttp.Principal(c), id, in)
	if err != nil {
		presetError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(200, v)
}

// PoolKeyUpdate records an explicit admission or drain command.
type PoolKeyUpdate struct {
	Enabled   *bool     `json:"enabled"`
	RequestID uuid.UUID `json:"request_id"`
}

// Key changes admission on one previously rotated credential.
// @Summary 调整已验证 Key 的号池准入
// @ID updateAdminPoolKey
// @Tags settings
// @Accept json
// @Param id path string true "连接"
// @Param credential_id path string true "凭据"
// @Param body body PoolKeyUpdate true "准入"
// @Success 200 {object} application.ProviderCredentialSummary
// @Router /api/admin/providers/{id}/credentials/{credential_id}/pool [put]
func (h RoutePoolHandler) Key(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	key, ok := httpapi.PathUUID(c, "credential_id")
	if !ok {
		return
	}
	var in PoolKeyUpdate
	if !httpapi.Decode(c, &in) {
		return
	}
	if in.Enabled == nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	v, err := h.Store.SetPoolKey(c.Request.Context(), identityhttp.Principal(c), id, key, *in.Enabled, in.RequestID)
	if err != nil {
		presetError(c, err)
		return
	}
	c.JSON(200, v)
}
