package agent

import (
	"context"
	"encoding/json"
	"time"

	canvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	script "github.com/StephenQiu30/glowloom-server/backend/internal/script/application"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	agenthttp "github.com/StephenQiu30/glowloom-server/backend/internal/agent/adapter/http"
	pg "github.com/StephenQiu30/glowloom-server/backend/internal/agent/adapter/postgres"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// Boundary owns the business services and formal command adapters.
type Boundary struct {
	Store                      *pg.Store
	Capabilities               app.Capabilities
	bindings                   []toolBinding
	db                         *gorm.DB
	canvasRuntime              canvas.YjsCommandRuntime
	mediaObjects               media.ObjectReader
	sourceObjects              script.PrivateObjects
	sourceOwner                *script.SourceService
	creditOperationSagaEnabled bool
}

// WithCreditOperationSaga enables event-driven freezes for Agent confirmations.
func (b *Boundary) WithCreditOperationSaga(enabled bool) *Boundary {
	if b != nil {
		b.creditOperationSagaEnabled = enabled
	}
	return b
}

// NewBoundary injects the owning business adapters for the Agent boundary.
func NewBoundary(database *gorm.DB, model app.ModelCapability) *Boundary {
	b := &Boundary{Store: pg.NewStore(database, ProvideControlAccess, time.Now), db: database, bindings: append(append(businessTools(), creationTools()...), exportTools()...)}
	b.Store.WithProposalValidator(func(tx *gorm.DB) app.DecisionValidator { return proposalValidator{tx} })
	b.Capabilities.Model = model
	for _, binding := range b.bindings {
		b.Capabilities.Tools = append(b.Capabilities.Tools, app.Tool{Descriptor: binding.descriptor})
	}
	b.Capabilities.Snapshot = func(ctx context.Context, s domain.Scope) (any, error) {
		var out map[string]any
		err := database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			canvases, err := canvasService(tx).List(ctx, s.Principal(), s.ProjectID)
			if err != nil {
				return err
			}
			script, err := scriptService(tx).Workspace(ctx, s.Principal(), s.ProjectID)
			if err != nil {
				return err
			}
			out = map[string]any{"canvases": canvases, "script": script}
			return nil
		})
		return out, err
	}
	b.Capabilities.RunSnapshot = func(ctx context.Context, g domain.Grant) (any, error) {
		out, err := b.Capabilities.Snapshot(ctx, g.Scope)
		if err != nil {
			return nil, err
		}
		deliveries, err := shotDeliverySnapshot(ctx, database.WithContext(ctx), g)
		if err != nil {
			return nil, err
		}
		view := out.(map[string]any)
		view["shot_deliveries"] = deliveries
		return view, nil
	}
	return b
}

// AtomicTool executes a business tool in the transaction that owns its receipt.
func (b *Boundary) AtomicTool(c *gin.Context, name string, in domain.CallInput) (domain.CallReceipt, error) {
	if name == "script_source_publish" {
		return b.executeSourcePublication(c.Request.Context(), in)
	}
	if name == "media_image_inspect" {
		return b.executeMediaImage(c.Request.Context(), in)
	}
	if name == "media_reference_inspect" {
		return b.executeMediaReference(c.Request.Context(), in)
	}
	if name == shotDeliveryCreateTool {
		return b.executeShotDelivery(c.Request.Context(), in)
	}
	if name == "canvas_commands" {
		return b.executeCanvasCommands(c.Request.Context(), in)
	}
	for _, binding := range b.bindings {
		if binding.descriptor.Name != name {
			continue
		}
		return b.Store.ExecuteAtomicTool(c.Request.Context(), name, in, func(tx *gorm.DB, s domain.Scope, key uuid.UUID) (domain.CallResult, error) {
			if name == shotDeliveryProposeTool {
				return b.proposeShotDelivery(c.Request.Context(), tx, s, key, in)
			}
			if name == "operation_propose" {
				return b.proposeOperation(c.Request.Context(), tx, s, key, in)
			}
			if name == "operation_confirm" {
				return b.confirmOperation(c.Request.Context(), tx, s, key, in)
			}
			out, err := binding.run(c.Request.Context(), tx, s, key, in.Input)
			if err != nil {
				return domain.CallResult{}, businessError(err)
			}
			raw, err := json.Marshal(out)
			if err != nil {
				return domain.CallResult{}, businessError(err)
			}
			return domain.CallResult{Status: "completed", Result: raw}, nil
		})
	}
	return domain.CallReceipt{}, domain.ErrNotFound
}

// BusinessHandler constructs the internal service-authenticated business handler.
func (b *Boundary) BusinessHandler(token string) *agenthttp.BusinessHandler {
	return agenthttp.NewBusinessHandler(b.Store, b.Capabilities, token).WithAtomicTools(b.AtomicTool).WithSkills(b.Store).WithInputSources(b.ConversationSources())
}

// CheckCanvas checks current access to the scoped canvas.
func (b *Boundary) CheckCanvas(ctx context.Context, a identity.Principal, project, id uuid.UUID) error {
	document, err := canvasService(b.db).Get(ctx, a, id)
	if err != nil {
		return err
	}
	if document.ProjectID != project {
		return domain.ErrNotFound
	}
	return nil
}
