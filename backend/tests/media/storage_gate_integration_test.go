package media_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

func TestStorageActivationRequiresMigrationProbesAndEveryLiveRole(t *testing.T) {
	db := storageRouteDB(t).Begin()
	t.Cleanup(func() { _ = db.Rollback().Error })
	routes := pgmedia.NewStorageRoutes(db)
	profiles := storageProfiles(t, routes)
	if err := routes.CheckStorageFinalSchema(t.Context()); !errors.Is(err, mediaapp.ErrStorageNotReady) {
		t.Fatal("expanded nullable schema advertised as finalized", err)
	}
	request := mediaapp.StorageActivation{UploadPolicy: uploadRoutingPolicy(), UploadSessionTTL: 24 * time.Hour, ExpectedRevision: "", Revision: "gated-" + uuid.NewString(), DefaultWriteProfile: "minio-main", ConfigSHA256: strings.Repeat("b", 64), Profiles: []mediaapp.StorageDeploymentProfile{{StorageProfile: profiles[0], Mode: "read_write"}, {StorageProfile: profiles[1], Mode: "read_only"}}}
	// Isolate the deployment gate fixtures inside this rollback-only transaction.
	if err := db.Exec(`TRUNCATE media.storage_route,media.storage_activation,media.storage_activation_guard,media.storage_runtime,media.storage_deployment_evidence,media.storage_probe_evidence CASCADE`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := routes.ActivateStorage(t.Context(), request); !errors.Is(err, mediaapp.ErrStorageNotReady) {
		t.Fatal("unproved deployment activated", err)
	}
	if err := routes.BindLegacyStorage(t.Context(), profiles[0], strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	if err := routes.RecordStorageDeployment(t.Context(), request.ConfigSHA256, strings.Repeat("c", 64)); err != nil {
		t.Fatal(err)
	}
	for _, p := range request.Profiles {
		if err := routes.RecordStorageProbe(t.Context(), p.ID, request.ConfigSHA256, strings.Repeat("d", 64), 24*time.Hour); err != nil {
			t.Fatal(err)
		}
	}
	capabilities := []string{"refs-v1", "work-v1", "legacy-v1", "operation-v1", "media-v1", "script-v1", "mediatool-v1", "cleanup-v1"}
	for _, role := range []string{"api", "worker", "relay", "setup"} {
		if err := routes.ReportStorageRuntime(t.Context(), mediaapp.StorageRuntime{InstanceID: uuid.New(), Role: role, Revision: request.Revision, ConfigSHA256: request.ConfigSHA256, Capabilities: capabilities}, time.Minute); err != nil {
			t.Fatal(err)
		}
	}
	// This database also contains unrelated legacy fixtures; no target may switch
	// while any owner column remains unresolved, even after readiness/probe reports.
	var remaining int64
	if err := db.Raw(`SELECT count(*) FROM media.media_asset WHERE object_location_id IS NULL`).Scan(&remaining).Error; err != nil {
		t.Fatal(err)
	}
	if remaining > 0 {
		if _, err := routes.ActivateStorage(t.Context(), request); !errors.Is(err, mediaapp.ErrStorageNotReady) {
			t.Fatal("unresolved historical references bypassed activation", err)
		}
	}
	// A currently live stale instance must block activation independently of counts.
	if err := routes.ReportStorageRuntime(t.Context(), mediaapp.StorageRuntime{InstanceID: uuid.New(), Role: "worker", Revision: "old", ConfigSHA256: strings.Repeat("e", 64), Capabilities: capabilities}, time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, err := routes.ActivateStorage(t.Context(), request); !errors.Is(err, mediaapp.ErrStorageNotReady) {
		t.Fatal("old worker was ignored", err)
	}
}

func TestStorageActivationSerializesConcurrentOldProfileAdmission(t *testing.T) {
	dsn := os.Getenv("LV_TEST_STORAGE_RACE_DSN")
	if dsn == "" {
		t.Skip("set a separate empty LV_TEST_STORAGE_RACE_DSN")
	}
	t.Setenv("LV_TEST_STORAGE_ROUTE_DSN", dsn)
	database := storageRouteDB(t)
	routes := pgmedia.NewStorageRoutes(database)
	if _, err := routes.ReadActive(t.Context()); !errors.Is(err, mediaapp.ErrStorageNotReady) {
		t.Fatal("race fixture must start empty", err)
	}
	profiles := storageProfiles(t, routes)
	if err := routes.BindLegacyStorage(t.Context(), profiles[0], strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	if _, err := routes.Activate(t.Context(), "", "race-initial", "minio-main"); err != nil {
		t.Fatal(err)
	}
	in := mediaapp.StorageActivation{ExpectedRevision: "race-initial", Revision: "race-cutover", DefaultWriteProfile: "cos-main", ConfigSHA256: strings.Repeat("5", 64), UploadPolicy: uploadRoutingPolicy(), UploadSessionTTL: time.Hour, Profiles: []mediaapp.StorageDeploymentProfile{{StorageProfile: profiles[0], Mode: "read_only"}, {StorageProfile: profiles[1], Mode: "read_write"}}}
	if err := routes.RecordStorageDeployment(t.Context(), in.ConfigSHA256, strings.Repeat("c", 64)); err != nil {
		t.Fatal(err)
	}
	for _, p := range profiles {
		if err := routes.RecordStorageProbe(t.Context(), p.ID, in.ConfigSHA256, strings.Repeat("d", 64), time.Hour); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range profiles {
		if err := routes.RecordStorageBucketEvidence(t.Context(), p.ID, in.ConfigSHA256, strings.Repeat("f", 64), time.Hour); err != nil {
			t.Fatal(err)
		}
	}
	caps := []string{"refs-v1", "work-v1", "legacy-v1", "operation-v1", "media-v1", "script-v1", "mediatool-v1", "cleanup-v1"}
	for _, role := range []string{"api", "worker", "relay", "setup"} {
		if err := routes.ReportStorageRuntime(t.Context(), mediaapp.StorageRuntime{InstanceID: uuid.New(), Role: role, Revision: in.Revision, ConfigSHA256: in.ConfigSHA256, Capabilities: caps}, time.Minute); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	blocker := database.WithContext(ctx).Begin()
	defer func() { _ = blocker.Rollback().Error }()
	if err := blocker.Exec(`SELECT 1 FROM media.storage_route WHERE singleton=1 FOR UPDATE`).Error; err != nil {
		t.Fatal(err)
	}
	activator := database.WithContext(ctx).Begin()
	defer func() { _ = activator.Rollback().Error }()
	var pid int
	if err := activator.Raw(`SELECT pg_backend_pid()`).Scan(&pid).Error; err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := pgmedia.NewStorageRoutes(activator).ActivateStorage(ctx, in); done <- err }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var waiting bool
		if err := database.Raw(`SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE pid=? AND wait_event_type='Lock')`, pid).Scan(&waiting).Error; err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		select {
		case err := <-done:
			t.Fatal("activation skipped route serialization", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("activation never waited for route owner")
		}
		time.Sleep(10 * time.Millisecond)
	}
	owner := mediaapp.StorageWorkOwner{Kind: "operation", OwnerID: uuid.New(), WorkID: uuid.New()}
	if _, err := pgmedia.NewStorageRoutes(blocker).PinStorageWork(ctx, owner); err != nil {
		t.Fatal(err)
	}
	if err := blocker.Commit().Error; err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, mediaapp.ErrStorageNotReady) {
		t.Fatal("concurrent admitted MinIO work lost write access", err)
	}
}

func TestStorageActivationLostReplyAndRollbackPreserveInflightTargets(t *testing.T) {
	dsn := os.Getenv("LV_TEST_STORAGE_GATE_DSN")
	if dsn == "" {
		t.Skip("set a separate empty LV_TEST_STORAGE_GATE_DSN")
	}
	t.Setenv("LV_TEST_STORAGE_ROUTE_DSN", dsn)
	db := storageRouteDB(t).Begin()
	t.Cleanup(func() { _ = db.Rollback().Error })
	routes := pgmedia.NewStorageRoutes(db)
	profiles := storageProfiles(t, routes)
	if err := routes.BindLegacyStorage(t.Context(), profiles[0], strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	required := []string{"refs-v1", "work-v1", "legacy-v1", "operation-v1", "media-v1", "script-v1", "mediatool-v1", "cleanup-v1"}
	ready := func(in mediaapp.StorageActivation, bucketProved bool) {
		t.Helper()
		if err := db.Exec(`TRUNCATE media.storage_runtime`).Error; err != nil {
			t.Fatal(err)
		}
		if err := routes.RecordStorageDeployment(t.Context(), in.ConfigSHA256, strings.Repeat("c", 64)); err != nil {
			t.Fatal(err)
		}
		for _, p := range in.Profiles {
			if err := routes.RecordStorageProbe(t.Context(), p.ID, in.ConfigSHA256, strings.Repeat("d", 64), time.Hour); err != nil {
				t.Fatal(err)
			}
			if bucketProved {
				if err := routes.RecordStorageBucketEvidence(t.Context(), p.ID, in.ConfigSHA256, strings.Repeat("f", 64), time.Hour); err != nil {
					t.Fatal(err)
				}
			}
		}
		for _, role := range []string{"api", "worker", "relay", "setup"} {
			if err := routes.ReportStorageRuntime(t.Context(), mediaapp.StorageRuntime{InstanceID: uuid.New(), Role: role, Revision: in.Revision, ConfigSHA256: in.ConfigSHA256, Capabilities: required}, time.Minute); err != nil {
				t.Fatal(err)
			}
		}
	}
	first := mediaapp.StorageActivation{Revision: "gate-first", DefaultWriteProfile: "minio-main", ConfigSHA256: strings.Repeat("1", 64), UploadPolicy: uploadRoutingPolicy(), UploadSessionTTL: 24 * time.Hour, Profiles: []mediaapp.StorageDeploymentProfile{{StorageProfile: profiles[0], Mode: "read_write"}, {StorageProfile: profiles[1], Mode: "read_write"}}}
	ready(first, false)
	if _, err := routes.ActivateStorage(t.Context(), first); !errors.Is(err, mediaapp.ErrStorageNotReady) {
		t.Fatal("functional probe substituted for bucket baseline", err)
	}
	for _, p := range first.Profiles {
		if err := routes.RecordStorageBucketEvidence(t.Context(), p.ID, first.ConfigSHA256, strings.Repeat("f", 64), time.Hour); err != nil {
			t.Fatal(err)
		}
	}
	route, err := routes.ActivateStorage(t.Context(), first)
	if err != nil || route.ProfileID != "minio-main" {
		t.Fatal("complete deployment refused", err)
	}
	actor, project := mediaStoreProject(t, db)
	admission := pgmedia.NewActivatedUploadAdmission(routingMediaStore(db))
	scope := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}
	key := uuid.New()
	admitted, err := admission.PinUpload(t.Context(), actor, scope, key)
	if err != nil || admitted.Policy != first.UploadPolicy {
		t.Fatal("active policy not loaded", err)
	}
	owner := mediaapp.StorageWorkOwner{Kind: "operation", OwnerID: uuid.New(), WorkID: uuid.New()}
	work, err := routes.PinStorageWork(t.Context(), owner)
	if err != nil {
		t.Fatal(err)
	}
	// A lost successful reply stays idempotent even after a stale process appears.
	if err := routes.ReportStorageRuntime(t.Context(), mediaapp.StorageRuntime{InstanceID: uuid.New(), Role: "worker", Revision: "old", ConfigSHA256: strings.Repeat("0", 64)}, time.Minute); err != nil {
		t.Fatal(err)
	}
	replay, err := routes.ActivateStorage(t.Context(), first)
	if err != nil || replay != route {
		t.Fatal("lost activation response not recovered", err)
	}
	next := first
	next.Profiles = append([]mediaapp.StorageDeploymentProfile(nil), first.Profiles...)
	next.ExpectedRevision = first.Revision
	next.Revision = "gate-next"
	next.ConfigSHA256 = strings.Repeat("2", 64)
	next.DefaultWriteProfile = "cos-main"
	next.UploadPolicy.MaxImageBytes = 1 << 20
	next.Profiles[0].Mode = "read_only"
	ready(next, true)
	if _, err := routes.ActivateStorage(t.Context(), next); !errors.Is(err, mediaapp.ErrStorageNotReady) {
		t.Fatal("inflight MinIO writer lost permission", err)
	}
	next.Profiles[0].Mode = "read_write"
	next.ConfigSHA256 = strings.Repeat("3", 64)
	ready(next, true)
	if _, err := routes.ActivateStorage(t.Context(), next); err != nil {
		t.Fatal(err)
	}
	recovered, err := routes.ReadStorageWork(t.Context(), owner)
	if err != nil || recovered != work {
		t.Fatal("switch rebound inflight job", err)
	}
	newer, err := routes.PinStorageWork(t.Context(), mediaapp.StorageWorkOwner{Kind: "operation", OwnerID: owner.OwnerID, WorkID: uuid.New()})
	if err != nil || newer.ProfileID != "cos-main" {
		t.Fatal("new work ignored switch", err)
	}
	sameAdmission, err := admission.PinUpload(t.Context(), actor, scope, key)
	if err != nil || sameAdmission != admitted {
		t.Fatal("inflight upload policy changed", err)
	}
	newAdmission, err := admission.PinUpload(t.Context(), actor, scope, uuid.New())
	if err != nil || newAdmission.ProfileID != "cos-main" || newAdmission.Policy != next.UploadPolicy {
		t.Fatal("new upload did not acquire activated policy", err)
	}
	legacy, err := routes.ResolveLegacyRef(t.Context(), "projects/old/image/file.png")
	if err != nil || legacy.ProfileID != "minio-main" {
		t.Fatal("history followed new route", err)
	}
}

func TestStorageFinalSchemaRejectsStandaloneBareKeyUniqueIndex(t *testing.T) {
	dsn := os.Getenv("LV_TEST_STORAGE_GATE_DSN")
	if dsn == "" {
		t.Skip("set a separate empty LV_TEST_STORAGE_GATE_DSN")
	}
	t.Setenv("LV_TEST_STORAGE_ROUTE_DSN", dsn)
	database := storageRouteDB(t).Begin()
	t.Cleanup(func() { _ = database.Rollback().Error })
	// Simulate the separately reviewed finalization only in this empty,
	// rollback-only fixture. No migration command automatically performs DDL.
	sql := `DO $$ DECLARE c record; r record; BEGIN
 FOR c IN SELECT conrelid::regclass AS tbl,conname FROM pg_constraint x WHERE conrelid IN ('media.media_asset'::regclass,'media.rendition'::regclass,'media.package_object'::regclass,'media.project_copy_object'::regclass,'media.transfer_object'::regclass,'media.purge_object'::regclass,'mediatool.depth_object'::regclass,'script.object_intent'::regclass,'script.object_state'::regclass) AND ((contype IN ('p','u') AND cardinality(conkey)=1 AND EXISTS(SELECT 1 FROM pg_attribute a WHERE a.attrelid=x.conrelid AND a.attnum=x.conkey[1] AND a.attname IN ('object_key','source_object_key','target_object_key'))) OR (contype='c' AND pg_get_constraintdef(x.oid) LIKE '%source_object_key%<>%target_object_key%')) LOOP EXECUTE format('ALTER TABLE %s DROP CONSTRAINT %I CASCADE',c.tbl,c.conname); END LOOP;
 FOR r IN SELECT * FROM (VALUES ('media.media_asset','object_location_id'),('media.rendition','object_location_id'),('media.package_object','object_location_id'),('media.project_copy_object','source_location_id'),('media.project_copy_object','target_location_id'),('media.transfer_object','source_location_id'),('media.transfer_object','target_location_id'),('media.purge_object','object_location_id'),('mediatool.depth_object','object_location_id'),('script.object_intent','object_location_id'),('script.object_state','object_location_id')) v(tbl,col) LOOP EXECUTE format('ALTER TABLE %s ALTER COLUMN %I SET NOT NULL',r.tbl,r.col); END LOOP;
 END $$;`
	if err := database.Exec(sql).Error; err != nil {
		t.Fatal(err)
	}
	routes := pgmedia.NewStorageRoutes(database)
	if err := routes.CheckStorageFinalSchema(t.Context()); err != nil {
		t.Fatal("finalized fixture rejected", err)
	}
	if err := database.Exec(`CREATE UNIQUE INDEX fixture_bare_key_unique ON media.media_asset(object_key)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := routes.CheckStorageFinalSchema(t.Context()); !errors.Is(err, mediaapp.ErrStorageNotReady) {
		t.Fatal("standalone global bare-key index passed finalization", err)
	}
}

func TestStorageFinalSchemaFenceBlocksConcurrentUniqueIndexCreation(t *testing.T) {
	dsn := os.Getenv("LV_TEST_STORAGE_GATE_DSN")
	if dsn == "" {
		t.Skip("set a separate empty LV_TEST_STORAGE_GATE_DSN")
	}
	t.Setenv("LV_TEST_STORAGE_ROUTE_DSN", dsn)
	database := storageRouteDB(t)
	transaction := database.Begin()
	defer func() { _ = transaction.Rollback().Error }()
	if err := pgmedia.NewStorageRoutes(transaction).CheckStorageFinalSchema(t.Context()); !errors.Is(err, mediaapp.ErrStorageNotReady) {
		t.Fatal("fixture finalization state", err)
	}
	pool, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	connection, err := pool.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = connection.Close() }()
	if _, err := connection.ExecContext(t.Context(), `SET lock_timeout='250ms'`); err != nil {
		t.Fatal(err)
	}
	name := "fixture_concurrent_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	defer func() {
		_ = transaction.Rollback().Error
		_, _ = connection.ExecContext(t.Context(), `DROP INDEX CONCURRENTLY IF EXISTS media.`+name)
	}()
	_, err = connection.ExecContext(t.Context(), `CREATE UNIQUE INDEX CONCURRENTLY `+name+` ON media.media_asset(object_key)`)
	var pg *pgconn.PgError
	if !errors.As(err, &pg) || pg.Code != "55P03" {
		t.Fatal("DDL changed uniqueness between gate check and activation commit", err)
	}
}
