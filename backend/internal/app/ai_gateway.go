package app

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app/aigateway"
	gatewayhttp "github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/adapter/provider"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

func registerAIGateway(api, session *gin.RouterGroup, database *gorm.DB, cfg config.Config) (*aigateway.Runtime, error) {
	runtime, err := aigateway.New(database, cfg)
	if err != nil {
		return nil, err
	}
	macKey, err := hex.DecodeString(cfg.CredentialRequestMACKey)
	if cfg.CredentialRequestMACKey != "" && (err != nil || len(macKey) != sha256.Size) {
		return nil, errors.New("GLOWLOOM_CREDENTIAL_REQUEST_MAC_KEY must be 32 hexadecimal-encoded bytes")
	}
	// Read-only checks use the key from this admin request and require no
	// credential unsealing or worker private key.
	checker := provider.New(nil, nil)
	gatewayhttp.New(database, aigateway.Authority, runtime.Provider, runtime.Sealer, gatewayhttp.KeyVerification{
		Verify: checker.VerifyProviderKey,
		MACKey: macKey,
	}).Register(api, session)
	return runtime, nil
}
