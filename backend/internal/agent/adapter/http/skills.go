package http

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// SkillStore owns private, versioned skill admission and reads.
type SkillStore interface {
	InstallSkill(context.Context, identity.Principal, uuid.UUID, domain.SkillInput) (domain.SkillVersion, error)
	ListSkills(context.Context, identity.Principal, uuid.UUID) ([]domain.SkillVersion, error)
	DiscoverSkills(context.Context, domain.Grant) ([]domain.SkillVersion, error)
	SetSkillEnabled(context.Context, identity.Principal, uuid.UUID, uuid.UUID, string, bool) (domain.SkillVersion, error)
	ReadSkill(context.Context, domain.Grant, uuid.UUID, string, string, string) (domain.SkillContent, error)
}

// WithSkills injects the owning skill store for internal business routes.
func (h *BusinessHandler) WithSkills(store SkillStore) *BusinessHandler { h.skills = store; return h }
func (h *BusinessHandler) registerSkills(g *gin.RouterGroup) {
	g.POST("/skills/discover", func(c *gin.Context) {
		var in struct {
			Grant domain.Grant `json:"grant"`
		}
		if !httpapi.Decode(c, &in) {
			return
		}
		if h.skills == nil {
			send(c, nil, domain.ErrUnavailable)
			return
		}
		out, err := h.skills.DiscoverSkills(c.Request.Context(), in.Grant)
		send(c, out, err)
	})
	g.POST("/skills/:id/load", func(c *gin.Context) {
		id, ok := httpapi.PathUUID(c, "id")
		if !ok {
			return
		}
		var in struct {
			Grant        domain.Grant `json:"grant"`
			Version      string       `json:"version"`
			ResourceHash string       `json:"resource_hash"`
			ResourcePath string       `json:"resource_path,omitempty"`
		}
		if !httpapi.Decode(c, &in) {
			return
		}
		if h.skills == nil {
			send(c, nil, domain.ErrUnavailable)
			return
		}
		out, err := h.skills.ReadSkill(c.Request.Context(), in.Grant, id, in.Version, in.ResourceHash, in.ResourcePath)
		send(c, out, err)
	})
}

// SkillManagementHandler manages private immutable package versions.
type SkillManagementHandler struct{ store SkillStore }

// SkillEnabledRequest requests a versioned skill availability change.
type SkillEnabledRequest struct {
	Enabled bool `json:"enabled"`
}

// RegisterSkillManagement keeps install/version/enable authority in Go.
func RegisterSkillManagement(g *gin.RouterGroup, store SkillStore) {
	h := &SkillManagementHandler{store: store}
	g.GET("/projects/:pid/agent/skills", h.List)
	g.POST("/projects/:pid/agent/skills", h.Install)
	g.POST("/projects/:pid/agent/skills/:id/versions/:version/enabled", h.SetEnabled)
}

// List reads the authenticated actor's private installed Skill versions.
// @Summary 列出项目创作空间内我的 Skill 版本
// @ID listAgentSkills
// @Tags Agent
// @Produce json
// @Param pid path string true "项目UUID"
// @Success 200 {array} domain.SkillVersion
// @Router /api/projects/{pid}/agent/skills [get]
func (h *SkillManagementHandler) List(c *gin.Context) {
	pid, ok := httpapi.PathUUID(c, "pid")
	if !ok {
		return
	}
	out, err := h.store.ListSkills(c.Request.Context(), identityhttp.Principal(c), pid)
	send(c, out, err)
}

// Install validates and installs a private immutable Skill package version.
// @Summary 导入私有 Skill 不可变版本
// @ID installAgentSkill
// @Tags Agent
// @Accept json
// @Produce json
// @Param pid path string true "项目UUID"
// @Param body body domain.SkillInput true "完整 Skill 包"
// @Success 200 {object} domain.SkillVersion
// @Router /api/projects/{pid}/agent/skills [post]
func (h *SkillManagementHandler) Install(c *gin.Context) {
	pid, ok := httpapi.PathUUID(c, "pid")
	if !ok {
		return
	}
	var in domain.SkillInput
	if !httpapi.Decode(c, &in) {
		return
	}
	out, err := h.store.InstallSkill(c.Request.Context(), identityhttp.Principal(c), pid, in)
	send(c, out, err)
}

// SetEnabled changes live package authority without changing immutable content.
// @Summary 启停指定 Skill 版本
// @ID setAgentSkillEnabled
// @Tags Agent
// @Accept json
// @Produce json
// @Param pid path string true "项目UUID"
// @Param id path string true "Skill UUID"
// @Param version path string true "Skill 版本"
// @Param body body SkillEnabledRequest true "启停设置"
// @Success 200 {object} domain.SkillVersion
// @Router /api/projects/{pid}/agent/skills/{id}/versions/{version}/enabled [post]
func (h *SkillManagementHandler) SetEnabled(c *gin.Context) {
	pid, ok := httpapi.PathUUID(c, "pid")
	if !ok {
		return
	}
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var in SkillEnabledRequest
	if !httpapi.Decode(c, &in) {
		return
	}
	out, err := h.store.SetSkillEnabled(c.Request.Context(), identityhttp.Principal(c), pid, id, c.Param("version"), in.Enabled)
	send(c, out, err)
}
