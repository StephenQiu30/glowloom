// Command glowloom runs backend roles and operator subcommands.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	"github.com/StephenQiu30/glowloom-server/backend/internal/app/identityoperator"
	"github.com/StephenQiu30/glowloom-server/backend/internal/app/projectcopyoperator"
	"github.com/StephenQiu30/glowloom-server/backend/internal/app/storageoperator"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	lvlog "github.com/StephenQiu30/glowloom-server/backend/internal/platform/log"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/redisconn"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "glowloom:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) > 1 && os.Args[1] == "project-copy-primary" {
		return projectcopyoperator.Run(os.Args[2:])
	}
	if len(os.Args) > 1 && os.Args[1] == "storage" {
		return storageoperator.Run(os.Args[2:])
	}
	if len(os.Args) > 1 && os.Args[1] == "identity" {
		return identityoperator.Run(os.Args[2:])
	}
	var role app.Role
	var queues string
	if len(os.Args) > 1 && os.Args[1] == "admin" {
		return fmt.Errorf("legacy administrator bootstrap was removed; use glowloom identity")
	}
	partitions := len(os.Args) > 1 && os.Args[1] == "partitions"
	switch {
	case partitions:
		if len(os.Args) != 3 || os.Args[2] != "ensure" {
			return fmt.Errorf("usage: glowloom partitions ensure")
		}
	default:
		roleFlag := flag.String("role", string(app.RoleAPI), "process role: api|worker|relay|all")
		queuesFlag := flag.String("queues", "", "worker task queues: flow|media|provider|agent.codex (comma-separated; default: flow)")
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

	if partitions {
		return app.EnsurePartitions(ctx, cfg, logger, time.Now().UTC())
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
