package http

import (
	"context"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityhttp "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/http"
	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
)

// PackageHandler owns bounded multipart imports and complete ZIP attachments.
type PackageHandler struct {
	service *application.LibraryPackageService
	slots   chan struct{}
}

// NewPackageHandler caps simultaneous large file commands in this API process.
func NewPackageHandler(service *application.LibraryPackageService) *PackageHandler {
	return &PackageHandler{service: service, slots: make(chan struct{}, 2)}
}

// Register installs owning package reads and explicit recovery commands.
func (h *PackageHandler) Register(group *gin.RouterGroup) {
	group.POST("/media/library/packages/imports", h.Import)
	group.GET("/media/library/packages/imports", h.List)
	group.GET("/media/library/packages/imports/:job_id", h.Get)
	group.POST("/media/library/packages/imports/:job_id/reconcile", h.Reconcile)
	group.POST("/media/library/packages/imports/:job_id/cancel", h.Cancel)
	group.GET("/media/library/packages/export", h.Export)
}

func (h *PackageHandler) begin(c *gin.Context) (func(), bool) {
	if h == nil || h.service == nil {
		writePackageError(c, application.ErrUnavailable)
		return nil, false
	}
	select {
	case h.slots <- struct{}{}:
	default:
		httpapi.WriteProblem(c, 429, "package_busy", nil)
		return nil, false
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Minute)
	c.Request = c.Request.WithContext(ctx)
	controller := http.NewResponseController(c.Writer)
	deadline, _ := ctx.Deadline()
	if err := controller.SetReadDeadline(deadline); err != nil && !errors.Is(err, http.ErrNotSupported) {
		cancel()
		<-h.slots
		writePackageError(c, application.ErrUnavailable)
		return nil, false
	}
	stopped := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { _ = controller.SetReadDeadline(time.Now()); close(stopped) })
	return func() {
		if !stop() {
			<-stopped
		}
		_ = controller.SetReadDeadline(time.Time{})
		cancel()
		<-h.slots
	}, true
}

// Import retains a closed complete ZIP and publishes every item atomically.
// @Summary 导入完整素材包及目录与正文
// @ID importMediaLibraryPackage
// @Tags media
// @Accept multipart/form-data
// @Produce json
// @Param Idempotency-Key header string true "UUID永久幂等键"
// @Param request formData string true "首个part：PackageImportRequest闭合JSON，含scope、expected_revision、expected_project_revision、local_review_confirmed"
// @Param file formData file true "第二个part：完整ZIP，≤500MiB、2500entries、展开≤2GiB；原件实际验证与人工确认"
// @Success 202 {object} application.PackageJob
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 413 {object} httpapi.Problem
// @Failure 415 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 429 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/media/library/packages/imports [post]
func (h *PackageHandler) Import(c *gin.Context) {
	done, ok := h.begin(c)
	if !ok {
		return
	}
	defer done()
	in, file, err := h.readImport(c)
	if err != nil {
		writePackageError(c, err)
		return
	}
	defer func() { _ = file.Close() }()
	in.Key, err = uuid.Parse(c.GetHeader("Idempotency-Key"))
	if err != nil || in.Key == uuid.Nil {
		writePackageError(c, application.ErrInvalidPackage)
		return
	}
	in.RequestID, err = uuid.Parse(c.GetString("request_id"))
	if err != nil || in.RequestID == uuid.Nil {
		writePackageError(c, application.ErrInvalidPackage)
		return
	}
	job, err := h.service.Import(c.Request.Context(), identityhttp.Principal(c), in, file)
	if err != nil {
		writePackageError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(202, job)
}

func (h *PackageHandler) readImport(c *gin.Context) (application.PackageImportRequest, *application.Downloaded, error) {
	var in application.PackageImportRequest
	if len(c.Request.URL.Query()) != 0 {
		return in, nil, application.ErrInvalidPackage
	}
	typeName, params, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || typeName != "multipart/form-data" || params["boundary"] == "" || len(params["boundary"]) > 70 {
		return in, nil, application.ErrUnsupportedUpload
	}
	const maxBody = application.MaxPackageBytes + uploadEnvelopeBytes
	if c.Request.ContentLength > maxBody {
		return in, nil, application.ErrUploadTooLarge
	}
	r := multipart.NewReader(http.MaxBytesReader(c.Writer, c.Request.Body, maxBody), params["boundary"])
	part, err := r.NextPart()
	if err != nil || part.FormName() != "request" || part.FileName() != "" {
		return in, nil, application.ErrInvalidPackage
	}
	body, err := io.ReadAll(io.LimitReader(part, uploadEnvelopeBytes+1))
	if err != nil || int64(len(body)) > uploadEnvelopeBytes {
		return in, nil, application.ErrInvalidPackage
	}
	if err := part.Close(); err != nil {
		return in, nil, err
	}
	in, err = application.DecodePackageImportRequest(body)
	if err != nil || !in.LocalReviewConfirmed || in.Scope.Validate() != nil {
		return in, nil, application.ErrInvalidPackage
	}
	if err := h.service.Authorize(c.Request.Context(), identityhttp.Principal(c), in.Scope, false); err != nil {
		return in, nil, err
	}
	part, err = r.NextPart()
	if err != nil || part.FormName() != "file" || part.FileName() == "" {
		return in, nil, application.ErrInvalidPackage
	}
	file, err := application.ReadPackageUpload(c.Request.Context(), part)
	if err != nil {
		return in, nil, err
	}
	keep := false
	defer func() {
		if !keep {
			_ = file.Close()
		}
	}()
	if err := part.Close(); err != nil {
		return in, nil, err
	}
	if _, err := r.NextPart(); !errors.Is(err, io.EOF) {
		return in, nil, application.ErrInvalidPackage
	}
	keep = true
	return in, file, nil
}

func packageQuery(c *gin.Context, paging bool) (domain.LibraryScope, int, int, error) {
	q := c.Request.URL.Query()
	for key := range q {
		if len(q[key]) != 1 || key != "scope" && key != "project_id" && (!paging || key != "page" && key != "page_size") {
			return domain.LibraryScope{}, 0, 0, application.ErrInvalidPackage
		}
	}
	scope, err := libraryScope(q)
	if err != nil {
		return scope, 0, 0, err
	}
	page, size := 1, 40
	for key, destination := range map[string]*int{"page": &page, "page_size": &size} {
		if q.Has(key) {
			value, err := strconv.Atoi(q.Get(key))
			if err != nil {
				return scope, 0, 0, application.ErrInvalidPackage
			}
			*destination = value
		}
	}
	return scope, page, size, nil
}

// List returns current actor-scoped durable import history.
// @Summary 素材包导入历史分页
// @ID listMediaLibraryPackages
// @Tags media
// @Produce json
// @Param scope query string false "personal/project"
// @Param project_id query string false "项目UUID"
// @Param page query int false "页码1..100000"
// @Param page_size query int false "每页1..120"
// @Success 200 {object} application.PackagePage
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/media/library/packages/imports [get]
func (h *PackageHandler) List(c *gin.Context) {
	if h == nil || h.service == nil {
		writePackageError(c, application.ErrUnavailable)
		return
	}
	scope, page, size, err := packageQuery(c, true)
	if err != nil {
		writePackageError(c, err)
		return
	}
	result, err := h.service.List(c.Request.Context(), identityhttp.Principal(c), scope, page, size)
	if err != nil {
		writePackageError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, result)
}

// Get reads current durable state rather than a historic command receipt.
// @Summary 当前素材包导入状态
// @ID getMediaLibraryPackage
// @Tags media
// @Produce json
// @Param job_id path string true "导入批次UUID"
// @Success 200 {object} application.PackageJob
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/media/library/packages/imports/{job_id} [get]
func (h *PackageHandler) Get(c *gin.Context) {
	if h == nil || h.service == nil {
		writePackageError(c, application.ErrUnavailable)
		return
	}
	if len(c.Request.URL.Query()) != 0 {
		writePackageError(c, application.ErrInvalidPackage)
		return
	}
	id, ok := id(c, "job_id")
	if !ok {
		return
	}
	job, err := h.service.Get(c.Request.Context(), identityhttp.Principal(c), id)
	if err != nil {
		writePackageError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, job)
}

// Reconcile retries only the frozen, retained original input and current head.
// @Summary 核验并恢复素材包导入
// @ID reconcileMediaLibraryPackage
// @Tags media
// @Accept json
// @Produce json
// @Param job_id path string true "批次UUID"
// @Param Idempotency-Key header string true "UUID永久命令键"
// @Param body body application.PackageControl true "已展示的批次revision"
// @Success 202 {object} application.PackageJob
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/media/library/packages/imports/{job_id}/reconcile [post]
func (h *PackageHandler) Reconcile(c *gin.Context) { h.control(c, false) }

// Cancel retires an unpublished batch only after every reserved key is absent.
// @Summary 取消未发布素材包并核验清理
// @ID cancelMediaLibraryPackage
// @Tags media
// @Accept json
// @Produce json
// @Param job_id path string true "批次UUID"
// @Param Idempotency-Key header string true "UUID永久命令键"
// @Param body body application.PackageControl true "已展示的批次revision"
// @Success 202 {object} application.PackageJob
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/media/library/packages/imports/{job_id}/cancel [post]
func (h *PackageHandler) Cancel(c *gin.Context) { h.control(c, true) }

func (h *PackageHandler) control(c *gin.Context, cancel bool) {
	done, ok := h.begin(c)
	if !ok {
		return
	}
	defer done()
	if len(c.Request.URL.Query()) != 0 {
		writePackageError(c, application.ErrInvalidPackage)
		return
	}
	id, ok := id(c, "job_id")
	if !ok {
		return
	}
	contentType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || contentType != "application/json" {
		httpapi.WriteProblem(c, 415, "unsupported_media_type", nil)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, uploadEnvelopeBytes))
	if err != nil {
		writePackageError(c, err)
		return
	}
	command, err := application.DecodePackageControl(body)
	if err != nil {
		writePackageError(c, err)
		return
	}
	command.Key, err = uuid.Parse(c.GetHeader("Idempotency-Key"))
	if err != nil {
		writePackageError(c, application.ErrInvalidPackage)
		return
	}
	command.RequestID, err = uuid.Parse(c.GetString("request_id"))
	if err != nil {
		writePackageError(c, application.ErrInvalidPackage)
		return
	}
	var job application.PackageJob
	if cancel {
		job, err = h.service.Cancel(c.Request.Context(), identityhttp.Principal(c), id, command)
	} else {
		job, err = h.service.Reconcile(c.Request.Context(), identityhttp.Principal(c), id, command)
	}
	if err != nil {
		writePackageError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(202, job)
}

// Export returns complete originals, directories, editorial text and a SHA header.
// @Summary 下载完整素材库ZIP
// @ID exportMediaLibraryPackage
// @Tags media
// @Produce application/zip
// @Param scope query string false "personal/project"
// @Param project_id query string false "项目UUID"
// @Success 200 {file} binary
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/media/library/packages/export [get]
func (h *PackageHandler) Export(c *gin.Context) {
	done, ok := h.begin(c)
	if !ok {
		return
	}
	defer done()
	scope, _, _, err := packageQuery(c, false)
	if err != nil {
		writePackageError(c, err)
		return
	}
	file, err := h.service.Export(c.Request.Context(), identityhttp.Principal(c), scope)
	if err != nil {
		writePackageError(c, err)
		return
	}
	defer func() { _ = file.Close() }()
	c.Header("Cache-Control", "private, no-store")
	c.Header("Content-Disposition", `attachment; filename="media-library.zip"`)
	c.Header("X-Content-SHA256", file.SHA256)
	c.DataFromReader(200, file.Size, "application/zip", file.File, nil)
}

func writePackageError(c *gin.Context, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		httpapi.WriteProblem(c, 413, "request_too_large", nil)
		return
	}
	switch {
	case errors.Is(err, application.ErrInvalidPackage):
		httpapi.WriteProblem(c, 422, "invalid_media_package", nil)
	case errors.Is(err, application.ErrPackageIncomplete):
		httpapi.WriteProblem(c, 422, "media_package_incomplete", nil)
	case errors.Is(err, application.ErrPackageUnsupportedKind):
		httpapi.WriteProblem(c, 422, "media_package_unsupported_kind", nil)
	case errors.Is(err, application.ErrPackageConflict):
		httpapi.WriteProblem(c, 409, "media_package_conflict", nil)
	case errors.Is(err, application.ErrUploadTooLarge):
		httpapi.WriteProblem(c, 413, "request_too_large", nil)
	case errors.Is(err, application.ErrUnsupportedUpload):
		httpapi.WriteProblem(c, 415, "media_package_unsupported_bytes", nil)
	default:
		writeLibraryError(c, err)
	}
}
