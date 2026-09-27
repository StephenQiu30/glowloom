package app_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/StephenQiu30/lanverse/backend/internal/app"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/config"
)

func TestParseRole(t *testing.T) {
	tests := []struct {
		arg     string
		want    app.Role
		wantErr error
	}{
		{arg: "api", want: app.RoleAPI},
		{arg: "worker", want: app.RoleWorker},
		{arg: "relay", want: app.RoleRelay},
		{arg: "all", want: app.RoleAll},
		{arg: "scheduler", wantErr: app.ErrUnknownRole},
	}
	for _, tt := range tests {
		t.Run(tt.arg, func(t *testing.T) {
			got, err := app.ParseRole(tt.arg)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("parseRole(%q) error = %v, want %v", tt.arg, err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Errorf("parseRole(%q) = %q, want %q", tt.arg, got, tt.want)
			}
		})
	}
}

func TestRunAllStopsWhenARequiredDependencyCannotInitialize(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	err := app.RunAll(ctx, config.Config{}, zap.NewNop())
	if err == nil || !strings.Contains(err.Error(), "role:") {
		t.Fatalf("RunAll error = %v, want a named role failure", err)
	}
	if errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
		t.Fatalf("RunAll did not stop promptly after initialization failure: %v", err)
	}
}

func TestWorkerQueue(t *testing.T) {
	for _, test := range []struct{ value, want string }{
		{"", "flow"}, {"flow", "flow"}, {"media", "media"},
		{"flow,media", "flow,media"}, {"media,flow", "flow,media"},
	} {
		if got, err := app.WorkerQueue(test.value); err != nil || got != test.want {
			t.Fatalf("WorkerQueue(%q) = %q, %v; want %q", test.value, got, err, test.want)
		}
	}
	for _, value := range []string{"agent", "flow,flow", "flow,agent", "media,media"} {
		if _, err := app.WorkerQueue(value); !errors.Is(err, app.ErrRoleNotAvailable) {
			t.Fatalf("WorkerQueue(%q) error = %v, want unavailable", value, err)
		}
	}
}
