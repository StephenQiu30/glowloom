// Package projectcopyoperator provides explicit historical primary-link repair.
package projectcopyoperator

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	canvaspg "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/adapter/postgres"
	platformdb "github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
	workspacepg "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

// Command is validated before any database connection is opened.
type Command struct {
	Action string
	Input  application.CopyPrimaryRepair
}

// ParseCommand rejects incomplete or ambiguous input before accessing the database.
func ParseCommand(args []string) (Command, error) {
	var command Command
	usage := errors.New("usage: glowloom project-copy-primary review --org UUID --job UUID | apply --org UUID --job UUID --source-canvas UUID --review-hash SHA256 --reason TEXT")
	if len(args) == 0 || args[0] != "review" && args[0] != "apply" {
		return command, usage
	}
	command.Action = args[0]
	flags := flag.NewFlagSet("project-copy-primary", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var org, job, source string
	flags.StringVar(&org, "org", "", "explicit organization")
	flags.StringVar(&job, "job", "", "completed copy job")
	if command.Action == "apply" {
		flags.StringVar(&source, "source-canvas", "", "reviewed frozen source document")
		flags.StringVar(&command.Input.ReviewHash, "review-hash", "", "exact review hash")
		flags.StringVar(&command.Input.Reason, "reason", "", "operator reason")
	}
	if flags.Parse(args[1:]) != nil || flags.NArg() != 0 {
		return Command{}, usage
	}
	var err error
	command.Input.OrgID, err = uuid.Parse(org)
	if err != nil || command.Input.OrgID == uuid.Nil {
		return Command{}, usage
	}
	command.Input.JobID, err = uuid.Parse(job)
	if err != nil || command.Input.JobID == uuid.Nil {
		return Command{}, usage
	}
	if command.Action == "apply" {
		command.Input.SourceDocumentID, err = uuid.Parse(source)
		if err != nil || command.Input.SourceDocumentID == uuid.Nil {
			return Command{}, usage
		}
		hash, err := hex.DecodeString(command.Input.ReviewHash)
		command.Input.Reason = strings.TrimSpace(command.Input.Reason)
		if err != nil || len(hash) != 32 || hex.EncodeToString(hash) != command.Input.ReviewHash || command.Input.Reason == "" || utf8.RuneCountInString(command.Input.Reason) > 500 {
			return Command{}, usage
		}
	}
	return command, nil
}

// Run never loads a default application DSN or installs schema implicitly.
func Run(args []string) error {
	command, err := ParseCommand(args)
	if err != nil {
		return err
	}
	dsn := os.Getenv("GLOWLOOM_COPY_REPAIR_DB_DSN")
	if dsn == "" {
		return errors.New("set an explicit GLOWLOOM_COPY_REPAIR_DB_DSN for the reviewed operator database")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	conn, err := platformdb.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		return errors.New("connect explicit copy repair database failed")
	}
	defer func() { _ = conn.Close() }()
	store := workspacepg.NewCopyPrimaryRepairStore(conn.DB, func(tx *gorm.DB) application.CopyPrimaryInspector { return canvaspg.NewProjectCopyStore(tx, nil, nil) })
	var result any
	if command.Action == "review" {
		result, err = store.Review(ctx, command.Input.OrgID, command.Input.JobID)
	} else {
		result, err = store.Apply(ctx, command.Input)
	}
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}
