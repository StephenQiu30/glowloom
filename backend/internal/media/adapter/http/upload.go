package http

import (
	"context"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityhttp "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/http"
	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	workspacedomain "github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

const uploadEnvelopeBytes int64 = 64 << 10

// UploadHandler admits at most two synchronous uploads in this local process.
// The route owns multipart parsing; ordinary JSON routes keep Decode's 1MiB cap.
type UploadHandler struct {
	service *application.UploadService
	slots   chan struct{}
}

// NewUploadHandler creates the local process's bounded multipart admission gate.
func NewUploadHandler(service *application.UploadService) *UploadHandler {
	return &UploadHandler{service: service, slots: make(chan struct{}, 2)}
}

// Register installs only the explicit multipart route in the protected API group.
func (h *UploadHandler) Register(group *gin.RouterGroup) {
	group.POST("/projects/:pid/media/uploads", h.Upload)
	group.POST("/media/library/uploads", h.UploadPersonal)
}

// Upload receives one local-owner-reviewed file and returns durable eligible media.
// @Summary 上传本地已人工确认的媒体
// @ID uploadMediaAsset
// @Tags media
// @Accept multipart/form-data
// @Produce json
// @Param pid path string true "项目UUID"
// @Param Idempotency-Key header string true "UUID幂等键"
// @Param file formData file true "图片20MiB，视频500MiB且60秒（静音VP8/VP9 WebM实际转为MP4），音频100MiB，包内GLB2模型64MiB，TXT/DOCX原件20MiB；图片/视频宽高均≤8192且≤4000万像素；按真实内容验证"
// @Param local_review_confirmed formData boolean true "已确认本地内容、素材使用权限且不含需要授权的真人素材"
// @Success 201 {object} application.UploadResult
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 413 {object} httpapi.Problem
// @Failure 415 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 429 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/projects/{pid}/media/uploads [post]
func (h *UploadHandler) Upload(c *gin.Context) {
	project, ok := id(c, "pid")
	if !ok {
		return
	}
	h.receiveUpload(c, project, false)
}

func (h *UploadHandler) receiveUpload(c *gin.Context, project uuid.UUID, personal bool) {
	key, err := uuid.Parse(c.GetHeader("Idempotency-Key"))
	if err != nil || key == uuid.Nil {
		httpapi.WriteProblem(c, 422, "invalid_idempotency_key", nil)
		return
	}
	actor := identityhttp.Principal(c)
	if h == nil || h.service == nil {
		uploadError(c, application.ErrUnavailable)
		return
	}
	if personal {
		err = h.service.AuthorizePersonal(c.Request.Context(), actor)
	} else {
		err = h.service.Authorize(c.Request.Context(), actor, project)
	}
	if err != nil {
		uploadError(c, err)
		return
	}
	select {
	case h.slots <- struct{}{}:
		defer func() { <-h.slots }()
	default:
		httpapi.WriteProblem(c, 429, "upload_busy", nil)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Minute)
	defer cancel()
	c.Request = c.Request.WithContext(ctx)
	controller := http.NewResponseController(c.Writer)
	deadline, _ := ctx.Deadline()
	if err := controller.SetReadDeadline(deadline); err != nil && !errors.Is(err, http.ErrNotSupported) {
		uploadError(c, application.ErrUnavailable)
		return
	}
	readStopped := make(chan struct{})
	stopRead := context.AfterFunc(ctx, func() { _ = controller.SetReadDeadline(time.Now()); close(readStopped) })
	defer func() {
		if !stopRead() {
			<-readStopped
		}
		_ = controller.SetReadDeadline(time.Time{})
	}()
	file, name, err := readMultipartUpload(c)
	if err != nil {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		uploadError(c, err)
		return
	}
	defer func() { _ = file.Close() }()
	requestID, err := uuid.Parse(c.GetString("request_id"))
	if err != nil || requestID == uuid.Nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	input := application.UploadInput{Actor: actor, Request: application.UploadRequest{ProjectID: project, Key: key, FileName: name, RequestID: requestID}, File: file, LocalReviewConfirmed: true}
	if personal {
		result, err := h.service.UploadPersonal(ctx, input)
		if err != nil {
			uploadError(c, err)
			return
		}
		c.Header("Cache-Control", "private, no-store")
		c.JSON(201, result)
	} else {
		result, err := h.service.Upload(ctx, input)
		if err != nil {
			uploadError(c, err)
			return
		}
		c.Header("Cache-Control", "private, no-store")
		c.JSON(201, result)
	}
}

func readMultipartUpload(c *gin.Context) (*application.Downloaded, string, error) {
	typeName, params, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || typeName != "multipart/form-data" || params["boundary"] == "" || len(params["boundary"]) > 70 {
		return nil, "", application.ErrUnsupportedUpload
	}
	const maxBody = application.MaxUploadVideoBytes + uploadEnvelopeBytes
	if c.Request.ContentLength > maxBody {
		return nil, "", application.ErrUploadTooLarge
	}
	counted := &uploadBodyReader{reader: http.MaxBytesReader(c.Writer, c.Request.Body, maxBody)}
	reader := multipart.NewReader(counted, params["boundary"])
	var file *application.Downloaded
	var name string
	confirmed, keep := false, false
	defer func() {
		if !keep && file != nil {
			_ = file.Close()
		}
	}()
	for parts := 0; ; parts++ {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, "", multipartError(err)
		}
		if parts >= 2 || len(part.Header) > 2 {
			return nil, "", application.ErrInvalidUpload
		}
		_, disposition, err := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
		if err != nil {
			return nil, "", application.ErrInvalidUpload
		}
		switch part.FormName() {
		case "file":
			if file != nil || disposition["filename"] == "" {
				return nil, "", application.ErrInvalidUpload
			}
			name, err = application.SafeUploadFileName(disposition["filename"])
			if err != nil {
				return nil, "", err
			}
			file, err = application.ReadUpload(c.Request.Context(), part, name)
			if err != nil {
				return nil, "", multipartError(err)
			}
		case "local_review_confirmed":
			if confirmed || disposition["filename"] != "" {
				return nil, "", application.ErrInvalidUpload
			}
			value, err := io.ReadAll(io.LimitReader(part, 5))
			if err != nil {
				return nil, "", multipartError(err)
			}
			if string(value) != "true" {
				return nil, "", application.ErrInvalidUpload
			}
			confirmed = true
		default:
			return nil, "", application.ErrInvalidUpload
		}
	}
	if file == nil || !confirmed {
		return nil, "", application.ErrInvalidUpload
	}
	// Bound MIME headers, delimiters and any epilogue independently of file size.
	n, err := io.Copy(io.Discard, io.LimitReader(counted, uploadEnvelopeBytes+1))
	if err != nil {
		return nil, "", multipartError(err)
	}
	if n > uploadEnvelopeBytes || counted.size-file.Size > uploadEnvelopeBytes {
		return nil, "", application.ErrUploadTooLarge
	}
	keep = true
	return file, name, nil
}

type uploadBodyReader struct {
	reader io.Reader
	size   int64
}

func (r *uploadBodyReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.size += int64(n)
	return n, err
}
func multipartError(err error) error {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return application.ErrUploadTooLarge
	}
	if errors.Is(err, application.ErrInvalidUpload) || errors.Is(err, application.ErrUnsupportedUpload) || errors.Is(err, application.ErrUploadTooLarge) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return application.ErrInvalidUpload
}
func uploadError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, application.ErrUploadConflict):
		httpapi.WriteProblem(c, 409, "idempotency_conflict", nil)
	case errors.Is(err, workspacedomain.ErrProjectStateConflict):
		httpapi.WriteProblem(c, 409, "project_state_conflict", nil)
	case errors.Is(err, application.ErrUploadTooLarge):
		httpapi.WriteProblem(c, 413, "request_too_large", nil)
	case errors.Is(err, application.ErrUnsupportedUpload):
		httpapi.WriteProblem(c, 415, "unsupported_media_type", nil)
	case errors.Is(err, application.ErrInvalidUpload):
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
	default:
		writeError(c, err)
	}
}
