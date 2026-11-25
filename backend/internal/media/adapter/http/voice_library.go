package http

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// VoiceLibraryHandler exposes private reference management without provider calls.
type VoiceLibraryHandler struct {
	resolve ActorResolver
	library application.VoiceLibrary
}

// NewVoiceLibraryHandler injects the transaction-owning voice library.
func NewVoiceLibraryHandler(library application.VoiceLibrary, resolve ActorResolver) *VoiceLibraryHandler {
	return &VoiceLibraryHandler{resolve: resolve, library: library}
}

// Register installs authenticated project routes.
func (h *VoiceLibraryHandler) Register(g *gin.RouterGroup) {
	g.GET("/projects/:pid/voice-library", h.list)
	g.GET("/projects/:pid/system-voices", h.system)
	g.PUT("/projects/:pid/system-voices/:speaker/favorite", h.favorite)
	g.POST("/projects/:pid/voice-library", h.create)
	g.PATCH("/projects/:pid/voice-library/:vid", h.update)
}
func voiceResponse(c *gin.Context, value any, err error) {
	if err == nil {
		c.JSON(200, value)
		return
	}
	status, code := 503, "voice_library_unavailable"
	switch {
	case errors.Is(err, application.ErrInvalidQuery):
		status, code = 422, "invalid_voice_reference"
	case errors.Is(err, application.ErrNotFound):
		status, code = 404, "voice_not_found"
	case errors.Is(err, application.ErrVoiceConflict):
		status, code = 409, "voice_revision_conflict"
	case errors.Is(err, application.ErrForbidden):
		status, code = 403, "forbidden"
	}
	httpapi.WriteProblem(c, status, code, nil)
}

// List saved reference voices.
// @Summary 我的参考音色
// @ID listReferenceVoices
// @Tags VoiceLibrary
// @Produce json
// @Param pid path string true "项目UUID"
// @Param cursor query string false "分页游标"
// @Success 200 {object} application.VoicePage
// @Router /api/projects/{pid}/voice-library [get]
func (h *VoiceLibraryHandler) list(c *gin.Context) {
	actor, actorOK := requestActor(c, h.resolve)
	if !actorOK {
		return
	}

	pid, ok := httpapi.PathUUID(c, "pid")
	if !ok {
		return
	}
	var cursor uuid.UUID
	if raw := c.Query("cursor"); raw != "" {
		var err error
		cursor, err = uuid.Parse(raw)
		if err != nil {
			voiceResponse(c, nil, application.ErrInvalidQuery)
			return
		}
	}
	out, err := h.library.ListVoices(c.Request.Context(), actor, pid, cursor)
	voiceResponse(c, out, err)
}

// Create a reference voice without supplier training.
// @Summary 保存参考音色
// @ID createReferenceVoice
// @Tags VoiceLibrary
// @Accept json
// @Produce json
// @Param pid path string true "项目UUID"
// @Param body body application.VoiceCreate true "音色"
// @Success 200 {object} application.VoiceProfile
// @Router /api/projects/{pid}/voice-library [post]
func (h *VoiceLibraryHandler) create(c *gin.Context) {
	actor, actorOK := requestActor(c, h.resolve)
	if !actorOK {
		return
	}

	pid, ok := httpapi.PathUUID(c, "pid")
	if !ok {
		return
	}
	var in application.VoiceCreate
	if !httpapi.Decode(c, &in) {
		return
	}
	out, err := h.library.CreateVoice(c.Request.Context(), actor, pid, in)
	voiceResponse(c, out, err)
}

// Update private voice metadata.
// @Summary 更新参考音色
// @ID updateReferenceVoice
// @Tags VoiceLibrary
// @Accept json
// @Produce json
// @Param pid path string true "项目UUID"
// @Param vid path string true "音色UUID"
// @Param body body application.VoiceUpdate true "修订"
// @Success 200 {object} application.VoiceProfile
// @Router /api/projects/{pid}/voice-library/{vid} [patch]
func (h *VoiceLibraryHandler) update(c *gin.Context) {
	actor, actorOK := requestActor(c, h.resolve)
	if !actorOK {
		return
	}

	pid, ok := httpapi.PathUUID(c, "pid")
	if !ok {
		return
	}
	id, ok := httpapi.PathUUID(c, "vid")
	if !ok {
		return
	}
	var in application.VoiceUpdate
	if !httpapi.Decode(c, &in) {
		return
	}
	out, err := h.library.UpdateVoice(c.Request.Context(), actor, pid, id, in)
	voiceResponse(c, out, err)
}

// Read documented system voices with private favorite flags.
// @Summary 系统音色
// @ID listSystemVoices
// @Tags VoiceLibrary
// @Produce json
// @Param pid path string true "项目UUID"
// @Success 200 {array} application.SystemVoice
// @Router /api/projects/{pid}/system-voices [get]
func (h *VoiceLibraryHandler) system(c *gin.Context) {
	actor, actorOK := requestActor(c, h.resolve)
	if !actorOK {
		return
	}

	pid, ok := httpapi.PathUUID(c, "pid")
	if !ok {
		return
	}
	out, err := h.library.ListSystemVoices(c.Request.Context(), actor, pid)
	voiceResponse(c, out, err)
}

// Save an absolute favorite flag.
// @Summary 收藏系统音色
// @ID favoriteSystemVoice
// @Tags VoiceLibrary
// @Accept json
// @Produce json
// @Param pid path string true "项目UUID"
// @Param speaker path string true "系统音色"
// @Param body body VoiceFavoriteRequest true "收藏"
// @Success 200 {object} VoiceFavoriteRequest
// @Router /api/projects/{pid}/system-voices/{speaker}/favorite [put]
func (h *VoiceLibraryHandler) favorite(c *gin.Context) {
	actor, actorOK := requestActor(c, h.resolve)
	if !actorOK {
		return
	}

	pid, ok := httpapi.PathUUID(c, "pid")
	if !ok {
		return
	}
	var in VoiceFavoriteRequest
	if !httpapi.Decode(c, &in) {
		return
	}
	err := h.library.FavoriteSystemVoice(c.Request.Context(), actor, pid, c.Param("speaker"), in.Favorite)
	voiceResponse(c, in, err)
}

// VoiceFavoriteRequest is an absolute private preference.
type VoiceFavoriteRequest struct {
	Favorite bool `json:"favorite"`
}
