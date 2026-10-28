package media_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

func TestStorageEmptyDeploymentMeasuresDatabaseAndCannotInventLegacy(t *testing.T) {
	tx := storageLedgerMigrationFixture(t)
	for _, name := range []string{"storage_ledger_expand_incremental.sql", "script_storage_locations_incremental.sql", "storage_bootstrap_incremental.sql"} {
		raw, err := os.ReadFile(filepath.Join("..", "..", "db", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Exec(string(raw)).Error; err != nil {
			t.Fatal(name, err)
		}
	}
	routes := pgmedia.NewStorageRoutes(tx)
	sha := strings.Repeat("a", 64)
	if err := routes.FinalizeStorageSchema(t.Context(), sha); !errors.Is(err, media.ErrStorageNotReady) {
		t.Fatal("finalization lacked a measured migration proof", err)
	}
	proof, err := routes.ProveEmptyStorageDeployment(t.Context(), sha)
	if err != nil || len(proof) != 64 {
		t.Fatal("actual empty schema could not be proved", err)
	}
	if err := routes.FinalizeStorageSchema(t.Context(), sha); err != nil {
		t.Fatal("safe empty finalization failed", err)
	}
	if err := routes.CheckStorageFinalSchema(t.Context()); err != nil {
		t.Fatal("final location relationships incomplete", err)
	}
	var count int
	if err := tx.Raw(`SELECT count(*) FROM media.storage_legacy_binding`).Scan(&count).Error; err != nil || count != 0 {
		t.Fatal("empty proof invented MinIO", count, err)
	}
	if err := routes.FinalizeStorageSchema(t.Context(), sha); err != nil {
		t.Fatal("finalization was not repeatable", err)
	}
}

func TestStorageEmptyProofRejectsForeignConfigAndWorkCreatedAfterMeasurement(t *testing.T) {
	tx := storageLedgerMigrationFixture(t, "canonical")
	routes := pgmedia.NewStorageRoutes(tx)
	sha := strings.Repeat("d", 64)
	if _, err := routes.ProveEmptyStorageDeployment(t.Context(), sha); err != nil {
		t.Fatal(err)
	}
	if err := routes.FinalizeStorageSchema(t.Context(), strings.Repeat("e", 64)); !errors.Is(err, media.ErrStorageNotReady) {
		t.Fatal("proof crossed configuration", err)
	}
	// A producer row without any object is still old work and invalidates bootstrap.
	if err := tx.Exec(`INSERT INTO media.storage_profile(id,provider,endpoint,signing_endpoint,region,bucket,addressing_style,resource_identity) VALUES('empty-proof-fixture','tos','https://example.invalid','https://example.invalid','cn-test','fixture-proof','',repeat('c',64))`).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`INSERT INTO media.storage_work_intent(id,kind,owner_id,work_id,profile_id,bucket,config_revision) VALUES(gen_random_uuid(),'package',gen_random_uuid(),gen_random_uuid(),'empty-proof-fixture','fixture-proof','fixture')`).Error; err != nil {
		t.Fatal(err)
	}
	if err := routes.FinalizeStorageSchema(t.Context(), sha); !errors.Is(err, media.ErrStorageNotReady) {
		t.Fatal("queued work ignored by bootstrap", err)
	}
	if _, err := routes.ProveEmptyStorageDeployment(t.Context(), sha); !errors.Is(err, media.ErrStorageNotReady) {
		t.Fatal("nonempty deployment claimed empty", err)
	}
}

func TestStorageFinalizationPreservesPhysicalScriptIntentRelationships(t *testing.T) {
	tx := storageLedgerMigrationFixture(t, "canonical")
	routes := pgmedia.NewStorageRoutes(tx)
	sha := strings.Repeat("b", 64)
	if _, err := routes.ProveEmptyStorageDeployment(t.Context(), sha); err != nil {
		t.Fatal(err)
	}
	if err := routes.FinalizeStorageSchema(t.Context(), sha); err != nil {
		t.Fatal(err)
	}
	if err := routes.CheckStorageFinalSchema(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`ALTER TABLE script.object_state DROP CONSTRAINT object_state_intent_location_fkey`).Error; err != nil {
		t.Fatal(err)
	}
	if err := routes.CheckStorageFinalSchema(t.Context()); !errors.Is(err, media.ErrStorageNotReady) {
		t.Fatal("orphanable script state accepted", err)
	}
	a, err := os.ReadFile(filepath.Join("..", "..", "internal", "media", "adapter", "postgres", "storage_finalize.sql"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join("..", "..", "db", "storage_ledger_finalize_incremental.sql"))
	if err != nil || string(a) != string(b) {
		t.Fatal("operator and SQL finalization drift", err)
	}
}

func TestStorageNativeEmptyBootstrapAllowsLaterGuardedRevisionWithRealWork(t *testing.T) {
	tx := storageLedgerMigrationFixture(t, "canonical")
	routes := pgmedia.NewStorageRoutes(tx)
	sha := strings.Repeat("8", 64)
	if _, err := routes.ProveEmptyStorageDeployment(t.Context(), sha); err != nil {
		t.Fatal(err)
	}
	if err := routes.FinalizeStorageSchema(t.Context(), sha); err != nil {
		t.Fatal(err)
	}
	profiles := []media.StorageProfile{{ID: "native-first", Provider: "tos", Endpoint: "https://tos-cn-beijing.volces.com", Bucket: "fixture-first", Region: "cn-beijing"}, {ID: "native-second", Provider: "tos", Endpoint: "https://tos-cn-shanghai.volces.com", Bucket: "fixture-second", Region: "cn-shanghai"}}
	for i := range profiles {
		profiles[i].SigningEndpoint = profiles[i].Endpoint
	}
	if err := routes.RegisterProfiles(t.Context(), profiles); err != nil {
		t.Fatal(err)
	}
	in := media.StorageActivation{Revision: "native-initial", ConfigSHA256: sha, DefaultWriteProfile: profiles[0].ID, UploadPolicy: uploadRoutingPolicy(), UploadSessionTTL: time.Hour, Profiles: []media.StorageDeploymentProfile{{StorageProfile: profiles[0], Mode: "read_write"}, {StorageProfile: profiles[1], Mode: "read_write"}}}
	ready := func() {
		t.Helper()
		if err := routes.RecordStorageDeployment(t.Context(), in.ConfigSHA256, strings.Repeat("f", 64)); err != nil {
			t.Fatal(err)
		}
		for _, p := range profiles {
			if err := routes.RecordStorageProbe(t.Context(), p.ID, in.ConfigSHA256, strings.Repeat("1", 64), time.Hour); err != nil {
				t.Fatal(err)
			}
			if err := routes.RecordStorageBucketEvidence(t.Context(), p.ID, in.ConfigSHA256, strings.Repeat("2", 64), time.Hour); err != nil {
				t.Fatal(err)
			}
		}
		for _, role := range []string{"api", "worker", "relay", "setup"} {
			if err := routes.ReportStorageRuntime(t.Context(), media.StorageRuntime{InstanceID: uuid.New(), Role: role, Revision: in.Revision, ConfigSHA256: in.ConfigSHA256, Capabilities: []string{"refs-v1", "work-v1", "legacy-v1", "operation-v1", "media-v1", "script-v1", "mediatool-v1", "cleanup-v1"}}, time.Minute); err != nil {
				t.Fatal(err)
			}
		}
	}
	ready()
	if _, err := routes.ActivateStorage(t.Context(), in); err != nil {
		t.Fatal("first measured native deployment", err)
	}
	work, err := routes.PinStorageWork(t.Context(), media.StorageWorkOwner{Kind: "package", OwnerID: uuid.New(), WorkID: uuid.New()})
	if err != nil {
		t.Fatal(err)
	}
	if work.ProfileID != profiles[0].ID {
		t.Fatal("admission target drifted")
	}
	// A cancelled, drained task still makes this a nonempty deployment.
	if err := routes.CancelStorageWork(t.Context(), work.ID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`UPDATE media.storage_runtime SET expires_at=clock_timestamp()-interval '1 second'`).Error; err != nil {
		t.Fatal(err)
	}
	in.ExpectedRevision, in.Revision, in.ConfigSHA256, in.DefaultWriteProfile = in.Revision, "native-next", strings.Repeat("9", 64), profiles[1].ID
	ready()
	if _, err := routes.ActivateStorage(t.Context(), in); err != nil {
		t.Fatal("guarded native bootstrap could not change revision", err)
	}
	if err := routes.FinalizeStorageSchema(t.Context(), in.ConfigSHA256); err != nil {
		t.Fatal("safe finalization replay rejected guarded lineage", err)
	}
	var legacy int
	if err := tx.Raw(`SELECT count(*) FROM media.storage_legacy_binding`).Scan(&legacy).Error; err != nil || legacy != 0 {
		t.Fatal("a fake original MinIO was manufactured", err)
	}
}

func TestStorageFinalizationRejectsCompositeFKPointingAtWrongPhysicalFields(t *testing.T) {
	tx := storageLedgerMigrationFixture(t, "canonical")
	routes := pgmedia.NewStorageRoutes(tx)
	sha := strings.Repeat("6", 64)
	if _, err := routes.ProveEmptyStorageDeployment(t.Context(), sha); err != nil {
		t.Fatal(err)
	}
	if err := routes.FinalizeStorageSchema(t.Context(), sha); err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`ALTER TABLE media.object_location ADD CONSTRAINT fixture_wrong_identity UNIQUE(id,bucket); ALTER TABLE media.package_object DROP CONSTRAINT package_object_object_location_id_fkey; ALTER TABLE media.package_object ADD CONSTRAINT package_object_wrong_identity FOREIGN KEY(object_location_id,object_key) REFERENCES media.object_location(id,bucket)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := routes.CheckStorageFinalSchema(t.Context()); !errors.Is(err, media.ErrStorageNotReady) {
		t.Fatal("a composite FK to bucket was mistaken for exact key identity", err)
	}
	if err := routes.FinalizeStorageSchema(t.Context(), sha); !errors.Is(err, media.ErrStorageNotReady) {
		t.Fatal("wrong referenced physical identity bypassed finalization", err)
	}
}
