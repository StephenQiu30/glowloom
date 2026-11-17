package agent

import (
	"gorm.io/gorm"

	app "github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/app/aigateway"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

// ConfigureModelGateway connects Agent model calls to the rebuilt AI authority.
// Agent grants remain workflow permissions; old operation/billing are not used.
func ConfigureModelGateway(database *gorm.DB, cfg config.Config) (app.ModelCapability, app.RecoverCapability, error) {
	runtime, err := aigateway.New(database, cfg)
	if err != nil {
		return nil, nil, err
	}
	if runtime.Provider == nil {
		return nil, nil, nil
	}
	return runtime.AgentModel, runtime.AgentRecover, nil
}
