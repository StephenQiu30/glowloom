// Package http exposes authenticated billing read models.
package http

import (
	"context"
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/billing/application"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// WalletReader reads only the payer selected by the authenticated scope.
type WalletReader func(context.Context, identityapp.Principal, *uuid.UUID) (application.WalletView, error)

// WalletHandler serves account balances and ledger pages.
type WalletHandler struct{ read WalletReader }

// NewWalletHandler injects the transaction-scoped reader.
func NewWalletHandler(read WalletReader) *WalletHandler { return &WalletHandler{read: read} }

// Register mounts the read endpoint after creation-scope authorization.
func (h *WalletHandler) Register(group *gin.RouterGroup) { group.GET("/billing/wallet", h.Read) }

// Read serves the authorized scope's wallet.
// @Summary 创作空间积分钱包
// @ID getCreditWallet
// @Tags billing
// @Produce json
// @Param cursor query string false "上一页的游标"
// @Success 200 {object} application.WalletView
// @Router /api/billing/wallet [get]
func (h *WalletHandler) Read(c *gin.Context) {
	var cursor *uuid.UUID
	if raw := c.Query("cursor"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil || id == uuid.Nil || id.String() != raw {
			httpapi.WriteProblem(c, 422, "invalid_cursor", nil)
			return
		}
		cursor = &id
	}
	result, err := h.read(c.Request.Context(), identityhttp.Principal(c), cursor)
	switch {
	case errors.Is(err, identityapp.ErrForbidden):
		httpapi.WriteProblem(c, 404, "not_found", nil)
	case errors.Is(err, identityapp.ErrSessionInvalid):
		httpapi.WriteProblem(c, 401, "session_invalid", nil)
	case err != nil:
		httpapi.LogDependencyError(c, err)
		httpapi.WriteProblem(c, 503, "dependency_unavailable", nil)
	default:
		c.JSON(200, result)
	}
}
