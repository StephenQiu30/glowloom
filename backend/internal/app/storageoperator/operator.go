package storageoperator

import (
	"context"
	"encoding/json"
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

	mediaobjects "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/objectstorage"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

// Run executes an explicit maintenance command. It never runs a migration, edits
// environment configuration or infers the original bucket from the currently selected write profile.
func Run(args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return Execute(ctx, args, os.Stdout)
}

// Execute keeps parsing/validation separate from any database or provider I/O.
// Elevated maintenance access must be supplied explicitly, separately from the
// ordinary application DSN. Errors do not print DSNs, credentials or signed URLs.
func Execute(ctx context.Context, args []string, output io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: glowloom storage validate|register|prove-empty|check-schema|reconcile-work|probe")
	}
	command := args[0]
	switch command {
	case "validate", "register", "reconcile-work", "probe", "probe-cleanup", "accept-bucket", "prove-empty", "check-schema":
	default:
		return errors.New("unknown storage command")
	}
	flags := flag.NewFlagSet("storage "+command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	profile := flags.String("profile", "", "explicit resource profile")
	evidence := flags.String("evidence-sha256", "", "reviewed external evidence digest")
	probeID := flags.String("probe-id", "", "persisted failed diagnostic identity")
	readKey := flags.String("read-key", "", "retained sentinel for read-only probe")
	batch := flags.Int("batch", 100, "maximum storage work items to reconcile")
	if flags.Parse(args[1:]) != nil || flags.NArg() != 0 {
		return errors.New("invalid storage arguments")
	}
	visited := make(map[string]bool)
	flags.Visit(func(f *flag.Flag) { visited[f.Name] = true })
	allowed := map[string]bool{}
	switch command {
	case "accept-bucket":
		allowed["profile"], allowed["evidence-sha256"] = true, true
	case "probe-cleanup":
		allowed["probe-id"] = true
	case "probe":
		allowed["profile"], allowed["read-key"] = true, true
	case "reconcile-work":
		allowed["batch"] = true
	}
	for name := range visited {
		if !allowed[name] {
			return errors.New("flag is not supported by this storage command")
		}
	}
	if (command == "accept-bucket" || command == "probe") && *profile == "" || command == "reconcile-work" && (*batch < 1 || *batch > 1000) {
		return errors.New("required storage precondition missing")
	}
	process, err := config.Load()
	if err != nil {
		return err
	}
	if process.ObjectStorage == nil {
		return errors.New("native object storage environment is incomplete")
	}
	cfg := *process.ObjectStorage
	in, err := Configuration(cfg)
	if err != nil {
		return err
	}
	encode := func(v any) error { return json.NewEncoder(output).Encode(v) }
	if command == "validate" {
		return encode(struct{ Revision, ConfigSHA256, DefaultWriteProfile string }{in.Revision, in.ConfigSHA256, in.DefaultWriteProfile})
	}
	dsn := process.StorageMaintenanceDSN
	if dsn == "" {
		return errors.New("GLOWLOOM_STORAGE_MAINTENANCE_DSN is required for this maintenance command")
	}
	connection, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		return errors.New("storage maintenance database unavailable")
	}
	defer func() { _ = connection.Close() }()
	routes := pgmedia.NewStorageRoutes(connection.DB)
	profiles := make([]mediaapp.StorageProfile, 0, len(in.Profiles))
	for _, p := range in.Profiles {
		profiles = append(profiles, p.StorageProfile)
	}
	switch command {
	case "prove-empty":
		proof, proofErr := routes.ProveEmptyStorageDeployment(ctx, in.ConfigSHA256)
		if proofErr != nil {
			return proofErr
		}
		return encode(struct{ ConfigSHA256, EvidenceSHA256 string }{in.ConfigSHA256, proof})
	case "check-schema":
		err = routes.CheckStorageFinalSchema(ctx)
	case "probe-cleanup":
		id, parseErr := uuid.Parse(*probeID)
		if parseErr != nil {
			return errors.New("invalid probe identity")
		}
		err = CleanupProbe(ctx, cfg, id, routes)
	case "register":
		err = routes.RegisterProfiles(ctx, profiles)
	case "accept-bucket":
		err = routes.RecordStorageBucketEvidence(ctx, *profile, in.ConfigSHA256, *evidence, time.Hour)
	case "reconcile-work":
		if err := routes.RegisterProfiles(ctx, profiles); err != nil {
			return errors.New("storage resource identity rejected before cleanup")
		}
		registry, openErr := objectstorage.OpenProfiles(cfg)
		if openErr != nil {
			return openErr
		}
		defer registry.Close()
		removed, cleanupErr := mediaapp.NewStorageCleanup(routes, mediaobjects.NewRoutedReadObjects(registry)).ReconcileBatch(ctx, *batch)
		if encodeErr := encode(struct{ Removed int }{removed}); encodeErr != nil {
			return encodeErr
		}
		err = cleanupErr
	case "probe":
		if err := routes.RegisterProfiles(ctx, profiles); err != nil {
			return errors.New("storage profile registration rejected")
		}
		receipt, probeErr := Probe(ctx, cfg, *profile, *readKey, routes)
		if probeErr != nil {
			if receipt.ProbeID != uuid.Nil {
				if encodeErr := encode(receipt); encodeErr != nil {
					return encodeErr
				}
			}
			return probeErr
		}
		if err := routes.RecordStorageProbe(ctx, *profile, in.ConfigSHA256, receipt.EvidenceSHA256, time.Hour); err != nil {
			return errors.New("storage probe receipt not recorded")
		}
		return encode(receipt)
	}
	if err != nil {
		if errors.Is(err, mediaapp.ErrStorageNotReady) || errors.Is(err, mediaapp.ErrStorageIdentityConflict) || errors.Is(err, mediaapp.ErrStorageRouteConflict) || errors.Is(err, mediaapp.ErrObjectMismatch) {
			return err
		}
		return errors.New("storage maintenance operation rejected; no success receipt")
	}
	_, err = fmt.Fprintln(output, "storage "+command+" completed")
	return err
}
