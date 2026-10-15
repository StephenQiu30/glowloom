package http

import (
	"embed"
	"encoding/json"

	"github.com/gin-gonic/gin"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

//go:embed legal/manifest.json
var legalPackage embed.FS

// LegalPackageVersion returns the immutable package included in this build.
func LegalPackageVersion() string {
	raw, err := legalPackage.ReadFile("legal/manifest.json")
	if err != nil {
		return ""
	}
	var manifest struct {
		Version string `json:"version"`
	}
	if json.Unmarshal(raw, &manifest) != nil {
		return ""
	}
	return manifest.Version
}

// AuthPolicyView explains whether registration is currently available.
type AuthPolicyView struct {
	RegistrationEnabled   bool   `json:"registration_enabled" binding:"required"`
	TermsVersion          string `json:"terms_version" binding:"required"`
	AvailableTermsVersion string `json:"available_terms_version" binding:"required"`
	ClosedReason          string `json:"closed_reason" binding:"required"`
	TermsURL              string `json:"terms_url" binding:"required"`
	PrivacyURL            string `json:"privacy_url" binding:"required"`
}

// AuthPolicyHandler adapts the public capability query.
type AuthPolicyHandler struct{ query *application.AuthPolicyQuery }

// NewAuthPolicyHandler injects the read-only policy use case.
func NewAuthPolicyHandler(query *application.AuthPolicyQuery) *AuthPolicyHandler {
	return &AuthPolicyHandler{query}
}

// Register installs the public no-store policy read.
func (h *AuthPolicyHandler) Register(api *gin.RouterGroup) { api.GET("/auth-policy", h.read) }

// @Summary Read public registration and accepted agreement policy
// @ID getAuthPolicy
// @Tags identity
// @Produce json
// @Success 200 {object} AuthPolicyView
// @Failure 503 {object} httpapi.Problem
// @Router /api/auth-policy [get]
func (h *AuthPolicyHandler) read(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	p, err := h.query.Read(c.Request.Context())
	if err != nil {
		httpapi.WriteProblem(c, 503, "temporarily_unavailable", nil)
		return
	}
	c.JSON(200, AuthPolicyView{p.RegistrationEnabled, p.TermsVersion, p.AvailableTermsVersion, p.ClosedReason, "/terms", "/privacy"})
}

// AcceptedLegalVersion rejects prepared packages even if configuration names them.
func AcceptedLegalVersion(configured string) string {
	raw, err := legalPackage.ReadFile("legal/manifest.json")
	if err != nil {
		return ""
	}
	var manifest struct{ Version, Status string }
	if json.Unmarshal(raw, &manifest) != nil || manifest.Status != "accepted" || manifest.Version != configured {
		return ""
	}
	return manifest.Version
}
