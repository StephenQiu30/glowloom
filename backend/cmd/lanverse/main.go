// Command lanverse runs backend roles and installs Temporal maintenance schedules.
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"go.uber.org/zap"
	"golang.org/x/term"

	"github.com/StephenQiu30/lanverse/backend/internal/app"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
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
	bootstrap := len(os.Args) > 1 && os.Args[1] == "admin"
	partitions := len(os.Args) > 1 && os.Args[1] == "partitions"
	var bootstrapInput identityapp.BootstrapAdminInput
	switch {
	case bootstrap:
		if len(os.Args) < 3 || os.Args[2] != "bootstrap" {
			return fmt.Errorf("usage: lanverse admin bootstrap --login-name <name> [--display-name=value] [--org-name=value]")
		}
		bootstrapFlags := flag.NewFlagSet("admin bootstrap", flag.ContinueOnError)
		loginName := bootstrapFlags.String("login-name", "", "first administrator login name")
		displayName := bootstrapFlags.String("display-name", "", "administrator display name")
		orgName := bootstrapFlags.String("org-name", "", "MVP organization name")
		if err := bootstrapFlags.Parse(os.Args[3:]); err != nil {
			return err
		}
		if bootstrapFlags.NArg() != 0 || strings.TrimSpace(*loginName) == "" {
			return fmt.Errorf("usage: lanverse admin bootstrap --login-name <name> [--display-name=value] [--org-name=value]")
		}
		bootstrapInput = identityapp.BootstrapAdminInput{
			LoginName: *loginName, DisplayName: *displayName, OrganizationName: *orgName,
		}
	case setup:
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
	case partitions:
		if len(os.Args) != 3 || os.Args[2] != "ensure" {
			return fmt.Errorf("usage: lanverse partitions ensure")
		}
	default:
		roleFlag := flag.String("role", string(app.RoleAPI), "process role: api|worker|relay|all")
		queuesFlag := flag.String("queues", "", "worker task queues: flow|media|flow,media (default: flow)")
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
		if err := app.InstallCleanupSchedules(ctx, cfg, logger, schedulePrefix); err != nil {
			return err
		}
		return app.InstallQuoteExpirySchedule(ctx, cfg, logger, schedulePrefix)
	}
	if partitions {
		return app.EnsurePartitions(ctx, cfg, logger, time.Now().UTC())
	}
	if bootstrap {
		password, err := readBootstrapPassword()
		if err != nil {
			return err
		}
		bootstrapInput.InitialPassword = password
		created, err := app.BootstrapAdmin(ctx, cfg, logger, bootstrapInput)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(os.Stdout, "First administrator %s created in organization %s; password change required at first login.\n", created.LoginName, created.OrgID); err != nil {
			return fmt.Errorf("report administrator bootstrap: %w", err)
		}
		return nil
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

func readBootstrapPassword() (string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return "", fmt.Errorf("administrator bootstrap requires an interactive terminal for the password")
	}
	fmt.Fprint(os.Stderr, "Initial administrator password: ")
	first, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("read administrator password: %w", err)
	}
	defer clear(first)
	fmt.Fprint(os.Stderr, "Confirm administrator password: ")
	confirmation, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("confirm administrator password: %w", err)
	}
	defer clear(confirmation)
	if len(first) == 0 || !bytes.Equal(first, confirmation) {
		return "", fmt.Errorf("administrator passwords are empty or do not match")
	}
	return string(first), nil
}
