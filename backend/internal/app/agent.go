package app

import (
	"context"
	"fmt"

	canvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	script "github.com/StephenQiu30/glowloom-server/backend/internal/script/application"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"

	agenthttp "github.com/StephenQiu30/glowloom-server/backend/internal/agent/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/adapter/runtimeclient"
	agentapp "github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	agentwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/agent"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

// AgentBoundaryOptions contains only current process boundary configuration.
type AgentBoundaryOptions struct {
	SourceObjects                       script.PrivateObjects
	Origin, ServiceToken, BusinessToken string
	Model                               agentapp.ModelCapability
	ModelRecover                        agentapp.RecoverCapability
	CanvasRuntime                       canvas.YjsCommandRuntime
	MediaObjects                        media.ObjectReader
	DocumentExtractor                   script.SourceExtractor
	CreditOperationSagaEnabled          bool
}

// RegisterAgentBoundary installs current routes. Go never starts or supervises
// Node, and unavailable Agent configuration does not gate unrelated Go APIs.
func RegisterAgentBoundary(ctx context.Context, router *gin.Engine, protected *gin.RouterGroup, database *gorm.DB, options AgentBoundaryOptions, logger *zap.Logger) error {
	configured := options.Origin != "" || options.ServiceToken != "" || options.BusinessToken != ""
	if configured && (options.Origin == "" || len(options.ServiceToken) < 32 || len(options.BusinessToken) < 32 || options.ServiceToken == options.BusinessToken) {
		return fmt.Errorf("agent boundary requires origin and independent direction credentials")
	}
	boundary := agentwire.NewBoundary(database, options.Model)
	boundary.WithCreditOperationSaga(options.CreditOperationSagaEnabled)
	boundary.WithDocumentExtractor(options.DocumentExtractor)
	boundary.Capabilities.ModelRecover = options.ModelRecover
	if options.SourceObjects != nil {
		boundary.WithSourceObjects(options.SourceObjects)
	}
	if options.MediaObjects != nil {
		boundary.WithMediaObjects(options.MediaObjects)
	}
	if options.CanvasRuntime != nil {
		boundary.WithCanvasRuntime(options.CanvasRuntime)
	}
	var runtime *runtimeclient.RuntimeClient
	if options.Origin != "" && options.ServiceToken != "" {
		var err error
		runtime, err = runtimeclient.New(options.Origin, options.ServiceToken)
		if err != nil {
			return err
		}
	}
	var client agenthttp.Runtime
	if runtime != nil {
		client = runtime
	}
	var quotes agentapp.QuoteReader
	if runtime != nil {
		quotes = runtime
	}
	agenthttp.NewPublicHandler(boundary.Store, boundary.Store, client).WithCanvasAccess(boundary.CheckCanvas).WithControlStatus(boundary.Store).WithConversationInputs(boundary.ConversationInputs(quotes)).WithConversationHistory(boundary.ConversationHistory(), boundary.Store).Register(protected)
	boundary.BusinessHandler(options.BusinessToken).Register(router)
	agenthttp.RegisterSkillManagement(protected, boundary.Store)
	if runtime != nil {
		go runtime.DeliverControls(ctx, boundary.Store, func(err error) {
			if logger != nil {
				logger.Warn("Agent control delivery pending", zap.Error(err))
			}
		})
	}
	return nil
}

// ConfigureAgentBoundaryModel composes the credential-bearing Go model exit.
func ConfigureAgentBoundaryModel(database *gorm.DB, cfg config.Config) (agentapp.ModelCapability, agentapp.RecoverCapability, error) {
	return agentwire.ConfigureModelGateway(database, cfg)
}
