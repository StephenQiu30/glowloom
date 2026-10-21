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
	"gorm.io/gorm"

	mediaobjects "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/objectstorage"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

// Run executes an explicit maintenance command. It never runs a migration, edits
// YAML or infers the original bucket from the currently selected write profile.
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
		return errors.New("usage: glowloom storage validate|register|bind-legacy|backfill|backfill-work|reconcile-work|probe|record-deployment|activate --config <path>")
	}
	command := args[0]
	switch command {
	case "validate", "register", "bind-legacy", "backfill", "backfill-work", "reconcile-work", "probe", "probe-cleanup", "accept-bucket", "record-deployment", "activate":
	default:
		return errors.New("unknown storage command")
	}
	flags := flag.NewFlagSet("storage "+command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	path := flags.String("config", "", "reviewed storage YAML")
	environment := flags.String("env", "prod", "local|staging|prod")
	profile := flags.String("profile", "", "explicit resource profile")
	expected := flags.String("expected-revision", "", "CAS revision; empty only for first activation")
	evidence := flags.String("evidence-sha256", "", "reviewed external evidence digest")
	probeID := flags.String("probe-id", "", "persisted failed diagnostic identity")
	readKey := flags.String("read-key", "", "retained sentinel for read-only probe")
	batch := flags.Int("batch", 100, "maximum historical objects to verify")
	if flags.Parse(args[1:]) != nil || flags.NArg() != 0 || *path == "" {
		return errors.New("invalid storage arguments")
	}
	if *environment == "production" {
		*environment = "prod"
	}
	if *environment != "local" && *environment != "staging" && *environment != "prod" {
		return errors.New("invalid storage arguments")
	}
	visited := make(map[string]bool)
	flags.Visit(func(f *flag.Flag) { visited[f.Name] = true })
	allowed := map[string]bool{"config": true, "env": true}
	switch command {
	case "bind-legacy", "accept-bucket":
		allowed["profile"], allowed["evidence-sha256"] = true, true
	case "probe-cleanup":
		allowed["probe-id"] = true
	case "probe":
		allowed["profile"], allowed["read-key"] = true, true
	case "activate":
		allowed["expected-revision"] = true
	case "record-deployment":
		allowed["evidence-sha256"] = true
	case "backfill", "backfill-work", "reconcile-work":
		allowed["batch"] = true
	}
	for name := range visited {
		if !allowed[name] {
			return errors.New("flag is not supported by this storage command")
		}
	}
	if command == "activate" && !visited["expected-revision"] || (command == "bind-legacy" || command == "accept-bucket" || command == "probe") && *profile == "" || (command == "backfill" || command == "backfill-work" || command == "reconcile-work") && (*batch < 1 || *batch > 1000) {
		return errors.New("required storage precondition missing")
	}
	cfg, err := config.LoadObjectStorage(*path, *environment)
	if err != nil {
		return err
	}
	in, err := Configuration(cfg, *expected)
	if err != nil {
		return err
	}
	encode := func(v any) error { return json.NewEncoder(output).Encode(v) }
	if command == "validate" {
		return encode(struct{ Revision, ConfigSHA256, DefaultWriteProfile string }{in.Revision, in.ConfigSHA256, in.DefaultWriteProfile})
	}
	dsn, configured := os.LookupEnv("GLOWLOOM_STORAGE_MAINTENANCE_DSN")
	if !configured {
		dsn = os.Getenv("LV_STORAGE_MAINTENANCE_DSN")
	}
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
	case "probe-cleanup":
		id, parseErr := uuid.Parse(*probeID)
		if parseErr != nil {
			return errors.New("invalid probe identity")
		}
		err = CleanupProbe(ctx, cfg, id, routes)
	case "register":
		err = routes.RegisterProfiles(ctx, profiles)
	case "bind-legacy":
		var original *mediaapp.StorageProfile
		for i := range profiles {
			if profiles[i].ID == *profile {
				original = &profiles[i]
			}
		}
		if original == nil {
			return errors.New("legacy profile is absent from reviewed configuration")
		}
		err = routes.BindLegacyStorage(ctx, *original, *evidence)
	case "record-deployment":
		err = routes.RecordStorageDeployment(ctx, in.ConfigSHA256, *evidence)
	case "accept-bucket":
		err = routes.RecordStorageBucketEvidence(ctx, *profile, in.ConfigSHA256, *evidence, time.Hour)
	case "backfill-work":
		count, migrateErr := routes.BackfillLegacyOperationWork(ctx, in.ConfigSHA256, *batch)
		if encodeErr := encode(struct{ Bound int }{count}); encodeErr != nil {
			return encodeErr
		}
		err = migrateErr
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
	case "backfill":
		if err := routes.RegisterProfiles(ctx, profiles); err != nil {
			return errors.New("storage resource identity rejected before backfill")
		}
		registry, openErr := objectstorage.OpenProfiles(cfg)
		if openErr != nil {
			return openErr
		}
		defer registry.Close()
		migration := mediaapp.NewLegacyMigration(routes, mediaapp.NewLegacyByteVerifier(mediaobjects.NewRoutedReadObjects(registry)))
		report, backfillErr := migration.BackfillBatch(ctx, *batch)
		if encodeErr := encode(report); encodeErr != nil {
			return encodeErr
		}
		err = backfillErr
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
	case "activate":
		var route mediaapp.StorageRoute
		activateErr := connection.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			guarded := pgmedia.NewStorageRoutes(tx)
			if err := guarded.CheckStorageFinalSchema(ctx); err != nil {
				return err
			}
			var err error
			route, err = guarded.ActivateStorage(ctx, in)
			return err
		})
		if activateErr == nil {
			return encode(route)
		}
		err = activateErr
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
