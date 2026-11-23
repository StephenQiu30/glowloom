package canvas

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	canvashttp "github.com/StephenQiu30/glowloom-server/backend/creation/canvas/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/creation/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/creation/canvas/domain"
	operationhttp "github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/http"
	operation "github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
	operationdomain "github.com/StephenQiu30/glowloom-server/backend/execution/operation/domain"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/http"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
	media "github.com/StephenQiu30/glowloom-server/backend/media/application"
)

// StoryboardGeneration reuses the operation authority for quote and delivery.
type StoryboardGeneration interface {
	Quote(context.Context, identity.Principal, uuid.UUID, string, operationhttp.QuoteItemRequest) (operation.CreateFreeQuoteResult, error)
	Task(context.Context, identity.Principal, uuid.UUID, uuid.UUID) (operation.TaskDetail, error)
	StoryboardCandidates(context.Context, identity.Principal, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) ([]operation.TaskSummary, error)
}

// StoryboardMedia reads a project candidate through the media authority.
type StoryboardMedia interface {
	Reference(context.Context, identity.Principal, uuid.UUID, uuid.UUID) (media.AssetSummary, error)
}

type storyboardDelivery struct {
	service        *application.YjsService
	gateway        StoryboardGeneration
	media          StoryboardMedia
	notify         func(uuid.UUID)
	gatewayProblem func(*gin.Context, error)
}

// RegisterStoryboardDelivery composes canvas, generation and media for the public workbench.
func RegisterStoryboardDelivery(g *gin.RouterGroup, service *application.YjsService, notify func(uuid.UUID), gateway StoryboardGeneration, media StoryboardMedia, gatewayProblem func(*gin.Context, error)) {
	d := storyboardDelivery{service: service, gateway: gateway, media: media, notify: notify, gatewayProblem: gatewayProblem}
	g.POST("/canvases/:id/storyboard/quote", d.Quote)
	g.GET("/canvases/:id/storyboard/candidates", d.Candidates)
	g.POST("/canvases/:id/storyboard/adopt", d.Adopt)
}

// StoryboardQuoteRequest pins the chosen model and current shot contents.
type StoryboardQuoteRequest struct {
	domain.StoryboardShotTarget
	ModelKey string `json:"model_key"`
}

// StoryboardQuoteResponse carries the ordinary operation quote for a saved shot.
type StoryboardQuoteResponse operationhttp.QuoteResponse

// Quote freezes the saved storyboard shot in the normal video operation quote.
// @Summary 为当前镜头报价
// @ID quoteCanvasStoryboardShot
// @Tags canvases
// @Accept json
// @Produce json
// @Param id path string true "画布 UUID"
// @Param Idempotency-Key header string true "UUID"
// @Param body body StoryboardQuoteRequest true "镜头及模型"
// @Success 201 {object} StoryboardQuoteResponse
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Router /api/canvases/{id}/storyboard/quote [post]
func (h storyboardDelivery) Quote(c *gin.Context) {
	canvasID, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	key, err := uuid.Parse(c.GetHeader("Idempotency-Key"))
	if err != nil || key == uuid.Nil {
		canvashttp.WriteStoryboardError(c, domain.ErrInvalidCommand)
		return
	}
	var in StoryboardQuoteRequest
	if !httpapi.Decode(c, &in) {
		return
	}
	if strings.TrimSpace(in.ModelKey) == "" {
		canvashttp.WriteStoryboardError(c, domain.ErrInvalidCommand)
		return
	}
	ctx, actor := c.Request.Context(), identityhttp.Principal(c)
	view, err := h.service.ReadStoryboard(ctx, actor, canvasID)
	if err != nil {
		canvashttp.WriteStoryboardError(c, err)
		return
	}
	shot, ok := view.FindShot(in.BoardID, in.ShotID)
	if !ok || view.Revision != in.ExpectedRevision || shot.Revision != in.ShotRevision {
		canvashttp.WriteStoryboardError(c, domain.ErrStoryboardConflict)
		return
	}
	params, err := json.Marshal(map[string]int64{"duration_ms": shot.DurationMS})
	if err != nil {
		canvashttp.WriteStoryboardError(c, err)
		return
	}
	quote, err := h.gateway.Quote(ctx, actor, view.ProjectID, "storyboard/"+key.String(), operationhttp.QuoteItemRequest{
		ModelKey: in.ModelKey, Capability: operationdomain.CapabilityVideoGenerate, Mode: "text2video",
		Params: params, OutputCount: 1, ForceRegenerate: true,
		Prompt: operation.StoryboardShotPrompt(shot.Title, shot.Action, shot.Dialogue, shot.Camera, shot.Sound),
		Source: &operationdomain.CanvasSource{CanvasID: canvasID, NodeID: in.BoardID, Revision: view.Revision, ShotID: &shot.ID, ShotRevision: &shot.Revision},
	})
	if err != nil {
		h.gatewayProblem(c, err)
		return
	}
	c.JSON(http.StatusCreated, StoryboardQuoteResponse(operationhttp.NewQuoteResponse(quote)))
}

// Candidates lists attempts for the same stable shot across content versions.
// @Summary 查找镜头候选及运行任务
// @ID listCanvasStoryboardCandidates
// @Tags canvases
// @Produce json
// @Param id path string true "画布 UUID"
// @Param board_id query string true "故事板 UUID"
// @Param shot_id query string true "镜头 UUID"
// @Success 200 {array} operation.TaskSummary
// @Failure 404 {object} httpapi.Problem
// @Router /api/canvases/{id}/storyboard/candidates [get]
func (h storyboardDelivery) Candidates(c *gin.Context) {
	canvasID, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	board, boardErr := uuid.Parse(c.Query("board_id"))
	shot, shotErr := uuid.Parse(c.Query("shot_id"))
	if boardErr != nil || shotErr != nil {
		canvashttp.WriteStoryboardError(c, domain.ErrInvalidCommand)
		return
	}
	ctx, actor := c.Request.Context(), identityhttp.Principal(c)
	view, err := h.service.ReadStoryboard(ctx, actor, canvasID)
	if err != nil {
		canvashttp.WriteStoryboardError(c, err)
		return
	}
	if _, found := view.FindShot(board, shot); !found {
		canvashttp.WriteStoryboardError(c, application.ErrNotFound)
		return
	}
	items, err := h.gateway.StoryboardCandidates(ctx, actor, view.ProjectID, canvasID, board, shot)
	if err != nil {
		h.gatewayProblem(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, items)
}

// Adopt installs one ready candidate without creating a clip or an export.
// @Summary 采用当前镜头的已就绪候选
// @ID adoptCanvasStoryboardShot
// @Tags canvases
// @Accept json
// @Produce json
// @Param id path string true "画布 UUID"
// @Param Idempotency-Key header string true "UUID"
// @Param body body domain.StoryboardAdoption true "采用结果"
// @Success 200 {object} application.StoryboardView
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Router /api/canvases/{id}/storyboard/adopt [post]
func (h storyboardDelivery) Adopt(c *gin.Context) {
	canvasID, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	key, err := uuid.Parse(c.GetHeader("Idempotency-Key"))
	if err != nil || key == uuid.Nil {
		canvashttp.WriteStoryboardError(c, domain.ErrInvalidCommand)
		return
	}
	var in domain.StoryboardAdoption
	if !httpapi.Decode(c, &in) {
		return
	}
	if in.OperationID == uuid.Nil || in.OutputID == uuid.Nil {
		canvashttp.WriteStoryboardError(c, domain.ErrInvalidCommand)
		return
	}
	ctx, actor := c.Request.Context(), identityhttp.Principal(c)
	prepared, err := h.service.PrepareStoryboardAdoption(ctx, actor, canvasID, key, in, func(ctx context.Context, project uuid.UUID) (domain.StoryboardTake, error) {
		var take domain.StoryboardTake
		task, err := h.gateway.Task(ctx, actor, project, in.OperationID)
		if err != nil {
			return take, err
		}
		source := task.Source
		if task.Status != operationdomain.StatusCompleted || task.Capability != operationdomain.CapabilityVideoGenerate ||
			source == nil || source.CanvasID != canvasID || source.NodeID != in.BoardID || source.ShotID == nil ||
			*source.ShotID != in.ShotID || source.ShotRevision == nil || *source.ShotRevision != in.ShotRevision {
			return take, domain.ErrStoryboardConflict
		}
		for _, output := range task.Outputs {
			if output.ID == in.OutputID && output.Kind == "video" && output.ModerationStatus == "passed" && output.MediaAssetID != nil {
				asset, err := h.media.Reference(ctx, actor, project, *output.MediaAssetID)
				if err != nil {
					return take, err
				}
				return domain.StoryboardTake{OperationID: in.OperationID, OutputID: in.OutputID, AssetID: *output.MediaAssetID,
					AssetVersion: asset.Revision, ShotRevision: in.ShotRevision}, nil
			}
		}
		return take, application.ErrNotFound
	})
	if err != nil {
		canvashttp.WriteStoryboardError(c, err)
		return
	}
	view, err := h.service.CommitStoryboard(ctx, actor, prepared)
	if err != nil {
		canvashttp.WriteStoryboardError(c, err)
		return
	}
	if h.notify != nil && prepared.Command != nil {
		h.notify(canvasID)
	}
	c.JSON(http.StatusOK, view)
}
