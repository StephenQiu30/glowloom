package app

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/config"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

func TestRunAPIRequiresDatabase(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := RunAPI(ctx, config.Config{HTTPAddr: "127.0.0.1:0"}, zap.NewNop())
	if !errors.Is(err, db.ErrDSNRequired) {
		t.Fatalf("RunAPI() error = %v, want ErrDSNRequired", err)
	}
}
