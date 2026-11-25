package aigateway

import (
	"context"
	"encoding/json"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/domain"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

func resultText(record domain.Record, model domain.Model) (string, error) {
	result, err := agentResult(record, model.Name, model.Protocol)
	if err != nil {
		return "", err
	}
	var body struct {
		Output string `json:"output"`
	}
	if json.Unmarshal(result.Result, &body) != nil || strings.TrimSpace(body.Output) == "" {
		return "", domain.ErrUnavailable
	}
	return body.Output, nil
}
func textPreview(text string) (string, bool) {
	if len(text) <= 65536 {
		return text, false
	}
	// A long indentation prefix must not turn usable original text into an empty preview.
	text = strings.TrimLeftFunc(text, unicode.IsSpace)
	if len(text) <= 65536 {
		return text, true
	}
	end := 65536
	for !utf8.RuneStart(text[end]) {
		end--
	}
	return text[:end], true
}

// Text returns only the original business text under current project visibility.
// It does not expose native receipt metadata, purchase facts, or another SDK call.
func (c Creation) Text(ctx context.Context, a identity.Principal, p, id uuid.UUID) (string, error) {
	var text string
	err := c.Runtime.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		store := c.store(tx, a)
		record, err := store.ReadProjectResult(ctx, a.ScopeID, p, id)
		if err != nil {
			return err
		}
		if record.State != "settled" {
			return domain.ErrUnavailable
		}
		var row struct{ Contract []byte }
		if err := tx.Raw("SELECT contract FROM ai.request WHERE id=?", id).Scan(&row).Error; err != nil {
			return err
		}
		var model domain.Model
		if json.Unmarshal(row.Contract, &model) != nil || (model.Protocol != "chat" && model.Protocol != "responses" && model.Protocol != "messages") {
			return domain.ErrUnavailable
		}
		text, err = resultText(record, model)
		return err
	})
	return text, err
}

// downloadText returns the complete original text without the preview/adoption size limit.
// @Summary 下载原任务完整文本
// @Tags operations
// @ID getOperationText
// @Produce plain
// @Param id path string true "原任务 ID"
// @Param project_id query string true "项目 ID"
// @Success 200 {string} string
// @Router /api/operations/{id}/text [get]
func (c Creation) downloadText(ctx *gin.Context) {
	id, ok := httpapi.PathUUID(ctx, "id")
	if !ok {
		return
	}
	p, err := uuid.Parse(ctx.Query("project_id"))
	if err != nil {
		creationProblem(ctx, domain.ErrInvalid)
		return
	}
	text, err := c.Text(ctx.Request.Context(), identityhttp.Principal(ctx), p, id)
	if err != nil {
		creationProblem(ctx, err)
		return
	}
	ctx.Header("Content-Disposition", `attachment; filename="generated-text.txt"`)
	ctx.Header("X-Content-Type-Options", "nosniff")
	ctx.Data(200, "text/plain; charset=utf-8", []byte(text))
}

func textUnavailableCode(record domain.Record) string {
	var body struct {
		Choices []struct {
			Message struct {
				Refusal string `json:"refusal"`
			} `json:"message"`
		} `json:"choices"`
	}
	if record.Receipt != nil && json.Unmarshal(record.Receipt.Body, &body) == nil {
		for _, choice := range body.Choices {
			if strings.TrimSpace(choice.Message.Refusal) != "" {
				return "provider_output_refused"
			}
		}
	}
	return "provider_empty_text"
}
