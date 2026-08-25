package bootstrap

import (
	"context"
	"net/http"

	"github.com/StephenQiu30/lanverse/backend/internal/telemetry"
)

type BuildInfo struct {
	Service string `json:"service"`
	Version string `json:"version"`
	Commit  string `json:"commit"`
	BuiltAt string `json:"built_at"`
}

type RuntimeOptions struct {
	Build          BuildInfo
	Metrics        *telemetry.HTTPMetrics
	Ready          func(context.Context) error
	RegisterRoutes func(*http.ServeMux)
	AllowedOrigins []string
}

func (options RuntimeOptions) normalized() RuntimeOptions {
	if options.Build.Service == "" {
		options.Build.Service = "lanverse-api"
	}
	if options.Build.Version == "" {
		options.Build.Version = "development"
	}
	if options.Build.Commit == "" {
		options.Build.Commit = "unknown"
	}
	if options.Build.BuiltAt == "" {
		options.Build.BuiltAt = "unknown"
	}
	if options.Metrics == nil {
		options.Metrics = telemetry.NewHTTPMetrics()
	}
	return options
}
