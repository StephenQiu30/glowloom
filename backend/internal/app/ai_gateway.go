package app

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	gatewayhttp "github.com/StephenQiu30/glowloom-server/backend/execution/gateway/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/app/aigateway"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

func registerAIGateway(api, session *gin.RouterGroup, database *gorm.DB, cfg config.Config) (*aigateway.Runtime, error) {
	runtime, err := aigateway.New(database, cfg)
	if err != nil {
		return nil, err
	}
	gatewayhttp.New(database, aigateway.Authority, runtime.Provider, runtime.Sealer).Register(api, session)
	return runtime, nil
}
