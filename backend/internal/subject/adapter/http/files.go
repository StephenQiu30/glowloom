package http

import (
	"mime"

	"github.com/gin-gonic/gin"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
	"github.com/StephenQiu30/glowloom-server/backend/internal/subject/application"
)

// FileHandler streams only complete, verified and currently authorized files.
type FileHandler struct{ files application.Files }

// NewFileHandler injects verified attachment and package access.
func NewFileHandler(files application.Files) *FileHandler { return &FileHandler{files: files} }

// Register installs explicit subject preview and export endpoints.
func (h *FileHandler) Register(group *gin.RouterGroup) {
	group.GET("/subjects/:id/versions/:version/media/:binding", h.Media)
	group.POST("/subjects/export", h.Export)
}

// Media returns an exact frozen subject media binding as a private inline file.
// @Summary 读取主体版本的正式参考素材
// @ID getSubjectMedia
// @Tags subjects
// @Produce application/octet-stream
// @Param id path string true "主体UUID"
// @Param version path string true "不可变版本UUID"
// @Param binding path string true "该版本正式媒体绑定UUID"
// @Success 200 {file} binary
// @Failure 404 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/subjects/{id}/versions/{version}/media/{binding} [get]
func (h *FileHandler) Media(c *gin.Context) {
	if h == nil || h.files == nil {
		writeError(c, application.ErrUnavailable)
		return
	}
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	version, ok := pathID(c, "version")
	if !ok {
		return
	}
	binding, ok := pathID(c, "binding")
	if !ok {
		return
	}
	file, err := h.files.Media(c.Request.Context(), currentActor(c), id, version, binding)
	if err != nil {
		writeError(c, err)
		return
	}
	streamSubjectFile(c, file, "inline")
}

// Export returns a complete portable package for an explicit selection.
// @Summary 下载选中主体的版本及正式媒体
// @ID exportSubjects
// @Tags subjects
// @Accept json
// @Produce application/zip
// @Param body body application.ExportInput true "明确选中的主体和不可变版本"
// @Success 200 {file} binary
// @Failure 404 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/subjects/export [post]
func (h *FileHandler) Export(c *gin.Context) {
	if h == nil || h.files == nil {
		writeError(c, application.ErrUnavailable)
		return
	}
	var in application.ExportInput
	if !httpapi.Decode(c, &in) {
		return
	}
	file, err := h.files.Export(c.Request.Context(), currentActor(c), in)
	if err != nil {
		writeError(c, err)
		return
	}
	streamSubjectFile(c, file, "attachment")
}

func streamSubjectFile(c *gin.Context, file application.File, disposition string) {
	if file.Reader == nil || file.Size < 1 {
		writeError(c, application.ErrUnavailable)
		return
	}
	defer func() { _ = file.Reader.Close() }()
	header := mime.FormatMediaType(disposition, map[string]string{"filename": file.Name})
	if header == "" {
		writeError(c, application.ErrUnavailable)
		return
	}
	c.Header("Content-Disposition", header)
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.DataFromReader(200, file.Size, file.MIMEType, file.Reader, nil)
}
