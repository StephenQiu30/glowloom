// Package identityoperator implements explicit, operator-invoked identity maintenance.
package identityoperator

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"golang.org/x/term"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app/identityruntime"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/credentialproof"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
)

// Command is one validated, explicitly targeted operator request.
type Command struct {
	Action, Login, Email string
	Operation            uuid.UUID
	PlatformAdmin        bool
}

// ParseCommand validates arguments without loading configuration or changing resources.
func ParseCommand(args []string) (Command, error) {
	var command Command
	usage := errors.New("usage: glowloom identity key-init | bind-start --login-name NAME --email EMAIL [--platform-admin] | bind-complete --operation UUID [--platform-admin] | mail-run --operation UUID")
	if len(args) == 0 {
		return command, usage
	}
	command.Action = args[0]
	flags := flag.NewFlagSet("identity "+command.Action, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var operation string
	switch command.Action {
	case "key-init":
	case "bind-start":
		flags.StringVar(&command.Login, "login-name", "", "explicit legacy account")
		flags.StringVar(&command.Email, "email", "", "email to verify")
		flags.BoolVar(&command.PlatformAdmin, "platform-admin", false, "explicit first platform administrator")
	case "bind-complete":
		flags.StringVar(&operation, "operation", "", "binding operation UUID")
		flags.BoolVar(&command.PlatformAdmin, "platform-admin", false, "must match requested authority")
	case "mail-run":
		flags.StringVar(&operation, "operation", "", "one mail operation UUID")
	default:
		return command, usage
	}
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 {
		return command, usage
	}
	if command.Action == "bind-start" {
		if _, err := domain.NormalizeLogin(command.Login); err != nil {
			return command, usage
		}
		if _, err := domain.NormalizeEmail(command.Email); err != nil {
			return command, usage
		}
	}
	if command.Action == "bind-complete" || command.Action == "mail-run" {
		id, err := uuid.Parse(operation)
		if err != nil || id == uuid.Nil {
			return command, usage
		}
		command.Operation = id
	}
	return command, nil
}

// Run executes one explicit identity operator command against existing resources.
func Run(args []string) (runErr error) {
	command, err := ParseCommand(args)
	if err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if command.Action == "key-init" {
		if err := credentialproof.CreateFile(cfg.IdentityKeyFile); err != nil {
			return err
		}
		_, err = fmt.Fprintln(os.Stdout, "Identity key file created. Keep a private backup; startup never regenerates it.")
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	conn, err := db.Open(ctx, cfg.DBDSN, noop.NewTracerProvider())
	if err != nil {
		return errors.New("connect configured identity database failed")
	}
	defer func() {
		if err := conn.Close(); err != nil {
			runErr = errors.Join(runErr, errors.New("close configured identity database failed"))
		}
	}()
	// Operator commands never run schema migrations or turn on public routes.
	var ready bool
	if err := conn.DB.WithContext(ctx).Raw(`SELECT to_regclass('identity.auth_challenge') IS NOT NULL AND to_regclass('workspace.creation_scope') IS NOT NULL`).Scan(&ready).Error; err != nil || !ready {
		return errors.New("identity expansion is not installed; review and apply the incremental migration first")
	}
	if command.Action == "mail-run" {
		worker, err := identityruntime.NewMailDelivery(pgidentity.NewOperationMailStore(conn.DB, command.Operation), cfg, time.Now)
		if err != nil {
			return err
		}
		count, err := worker.RunOnce(ctx, 1)
		if err != nil {
			return err
		}
		var states []struct {
			Status string
			Count  int64
		}
		if err := conn.DB.WithContext(ctx).Raw(`SELECT m.status,count(*) FROM identity.mail_delivery m JOIN identity.auth_challenge c ON c.id=m.challenge_id WHERE c.operation_id=? GROUP BY m.status`, command.Operation).Scan(&states).Error; err != nil {
			return errors.New("read mail submission status failed")
		}
		if _, err := fmt.Fprintf(os.Stdout, "Processed %d mail job(s). SMTP submission is not proof of inbox delivery.\n", count); err != nil {
			return err
		}
		for _, state := range states {
			if _, err := fmt.Fprintf(os.Stdout, "%s: %d\n", state.Status, state.Count); err != nil {
				return err
			}
		}
		return nil
	}
	auth, err := identityruntime.NewAuth(conn.DB, cfg, time.Now)
	if err != nil {
		return err
	}
	if command.Action == "bind-start" {
		result, err := auth.StartMigrationBinding(ctx, command.Login, command.Email, command.PlatformAdmin)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(os.Stdout, "Binding operation: %s\nMail queued; run identity mail-run with this operation. Account remains unchanged until verification.\n", result.OperationID)
		return err
	}
	code, err := readIdentityCode()
	if err != nil {
		return err
	}
	defer clear(code)
	id, err := auth.CompleteMigrationBinding(ctx, command.Operation, string(code), command.PlatformAdmin)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(os.Stdout, "Verified email bound to original user ID %s; previous sessions revoked.\n", id)
	return err
}

func readIdentityCode() ([]byte, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return nil, errors.New("email verification requires an interactive terminal; do not pass proof in command arguments")
	}
	fmt.Fprint(os.Stderr, "Email verification code: ")
	code, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return nil, errors.New("read email verification code failed")
	}
	return code, nil
}
