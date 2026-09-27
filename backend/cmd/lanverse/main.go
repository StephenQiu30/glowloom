// Command lanverse runs backend roles and installs Temporal maintenance schedules.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	"github.com/StephenQiu30/lanverse/backend/internal/app"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/config"
	lvlog "github.com/StephenQiu30/lanverse/backend/internal/platform/log"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/redisconn"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "lanverse:", err)
		os.Exit(1)
	}
}

func run() error {
	var role app.Role
	var queues, schedulePrefix string
	setup := len(os.Args) > 1 && os.Args[1] == "temporal"
	if setup {
		if len(os.Args) < 3 || os.Args[2] != "setup" {
			return fmt.Errorf("usage: lanverse temporal setup [--prefix=value]")
		}
		setupFlags := flag.NewFlagSet("temporal setup", flag.ContinueOnError)
		prefixFlag := setupFlags.String("prefix", "", "optional prefix for isolated schedule verification")
		if err := setupFlags.Parse(os.Args[3:]); err != nil {
			return err
		}
		if setupFlags.NArg() != 0 {
			return fmt.Errorf("unexpected temporal setup argument: %q", setupFlags.Arg(0))
		}
		schedulePrefix = *prefixFlag
	} else {
		roleFlag := flag.String("role", string(app.RoleAPI), "process role: api|worker|relay|all")
		queuesFlag := flag.String("queues", "", "worker task queue: flow")
		flag.Parse()
		if flag.NArg() != 0 {
			return fmt.Errorf("unexpected command: %q", flag.Arg(0))
		}
		var err error
		role, err = app.ParseRole(*roleFlag)
		if err != nil {
			return err
		}
		if role != app.RoleWorker && *queuesFlag != "" {
			return fmt.Errorf("--queues requires --role=worker")
		}
		queues = *queuesFlag
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
	redisconn.ConfigureLogging(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if setup {
		logger.Info("starting Temporal setup")
		return app.InstallCleanupSchedules(ctx, cfg, logger, schedulePrefix)
	}
	logger.Info("starting", zap.String("role", string(role)))
	switch role {
	case app.RoleAPI:
		return app.RunAPI(ctx, cfg, logger)
	case app.RoleWorker:
		return app.RunWorker(ctx, cfg, logger, queues)
	case app.RoleRelay:
		return app.RunRelay(ctx, cfg, logger)
	case app.RoleAll:
		return app.RunAll(ctx, cfg, logger)
	default:
		return fmt.Errorf("%w: %q", app.ErrRoleNotAvailable, role)
	}
}
