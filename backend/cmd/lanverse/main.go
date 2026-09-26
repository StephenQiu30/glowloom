// Command lanverse runs one backend role: api, worker or relay (DES-01, OPS-01 §3).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	"github.com/StephenQiu30/lanverse/backend/internal/app"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/config"
	lvlog "github.com/StephenQiu30/lanverse/backend/internal/platform/log"
)

type role string

const (
	roleAPI    role = "api"
	roleWorker role = "worker"
	roleRelay  role = "relay"
	roleAll    role = "all"
)

var (
	errUnknownRole      = errors.New("unknown role")
	errRoleNotAvailable = errors.New("role not available yet")
)

func parseRole(s string) (role, error) {
	switch r := role(s); r {
	case roleAPI:
		return r, nil
	case roleWorker, roleRelay, roleAll:
		// worker lands with M1-09, relay with M1-08 (BACKLOG).
		return "", fmt.Errorf("%w: %q", errRoleNotAvailable, s)
	default:
		return "", fmt.Errorf("%w: %q, want api|worker|relay|all", errUnknownRole, s)
	}
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "lanverse:", err)
		os.Exit(1)
	}
}

func run() error {
	roleFlag := flag.String("role", string(roleAPI), "process role: api|worker|relay|all")
	flag.Parse()

	r, err := parseRole(*roleFlag)
	if err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger, err := lvlog.New(cfg.Env, cfg.LogLevel)
	if err != nil {
		return err
	}
	defer func() { _ = logger.Sync() }()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Info("starting", zap.String("role", string(r)))
	return app.RunAPI(ctx, cfg, logger)
}
