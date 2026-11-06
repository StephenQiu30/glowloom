package media_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

func TestStorageEmptyDeploymentMeasuresDatabaseAndCannotInventLegacy(t *testing.T) {
	tx := storageSchemaFixture(t)
	routes := pgmedia.NewStorageRoutes(tx)
	sha := strings.Repeat("a", 64)
	proof, err := routes.ProveEmptyStorageDeployment(t.Context(), sha)
	if err != nil || len(proof) != 64 {
		t.Fatal("empty database proof unavailable", err)
	}
	replay, err := routes.ProveEmptyStorageDeployment(t.Context(), sha)
	if err != nil || proof != replay {
		t.Fatal("proof replay changed", err)
	}
	if err := routes.FinalizeStorageSchema(t.Context(), sha); err != nil {
		t.Fatal("schema pipeline version missing", err)
	}
	var count int
	if err := tx.Raw(`SELECT count(*) FROM media.storage_profile`).Scan(&count).Error; err != nil || count != 0 {
		t.Fatal("empty proof invented a profile", count, err)
	}
	if err := tx.Exec(`DELETE FROM media.storage_schema_version WHERE version=2`).Error; err != nil {
		t.Fatal(err)
	}
	if err := routes.FinalizeStorageSchema(t.Context(), sha); !errors.Is(err, media.ErrStorageNotReady) {
		t.Fatal("runtime invented absent schema pipeline version", err)
	}
}

func TestStorageEmptyProofRejectsInvalidConfigAndWorkCreatedAfterMeasurement(t *testing.T) {
	tx := storageSchemaFixture(t)
	routes := pgmedia.NewStorageRoutes(tx)
	sha := strings.Repeat("d", 64)
	if _, err := routes.ProveEmptyStorageDeployment(t.Context(), "invalid"); !errors.Is(err, media.ErrStorageNotReady) {
		t.Fatal("invalid config proof accepted", err)
	}
	if _, err := routes.ProveEmptyStorageDeployment(t.Context(), sha); err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`INSERT INTO media.storage_profile(id,provider,endpoint,signing_endpoint,region,bucket,addressing_style,resource_identity) VALUES('empty-proof-fixture','cos','https://example.invalid','https://example.invalid','cn-test','fixture-proof','',repeat('c',64))`).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`INSERT INTO media.storage_work_intent(id,kind,owner_id,work_id,profile_id,bucket,config_revision) VALUES(gen_random_uuid(),'package',gen_random_uuid(),gen_random_uuid(),'empty-proof-fixture','fixture-proof','1')`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := routes.ProveEmptyStorageDeployment(t.Context(), sha); !errors.Is(err, media.ErrStorageNotReady) {
		t.Fatal("queued work claimed empty", err)
	}
	if _, err := routes.ProveEmptyStorageDeployment(t.Context(), strings.Repeat("e", 64)); !errors.Is(err, media.ErrStorageNotReady) {
		t.Fatal("foreign config claimed nonempty schema", err)
	}
}

func TestStorageNativeEmptyBootstrapAllowsLaterGuardedRevisionWithRealWork(t *testing.T) {
	tx := storageSchemaFixture(t)
	routes := pgmedia.NewStorageRoutes(tx)
	sha := strings.Repeat("8", 64)
	if _, err := routes.ProveEmptyStorageDeployment(t.Context(), sha); err != nil {
		t.Fatal(err)
	}
	if err := routes.FinalizeStorageSchema(t.Context(), sha); err != nil {
		t.Fatal(err)
	}
	profiles := []media.StorageProfile{{ID: "native-first", Provider: "cos", Endpoint: "https://fixture-first-1250000000.cos.ap-beijing.myqcloud.com", Bucket: "fixture-first-1250000000", Region: "ap-beijing"}, {ID: "native-second", Provider: "cos", Endpoint: "https://fixture-second-1250000000.cos.ap-shanghai.myqcloud.com", Bucket: "fixture-second-1250000000", Region: "ap-shanghai"}}
	for i := range profiles {
		profiles[i].SigningEndpoint = profiles[i].Endpoint
	}
	if err := routes.RegisterProfiles(t.Context(), profiles); err != nil {
		t.Fatal(err)
	}
	in := media.StorageEnvironment{Revision: "1", ConfigSHA256: sha, DefaultWriteProfile: profiles[0].ID, UploadPolicy: uploadRoutingPolicy(), UploadSessionTTL: time.Hour, Profiles: []media.StorageDeploymentProfile{{StorageProfile: profiles[0], Mode: "read_write"}, {StorageProfile: profiles[1], Mode: "read_write"}}}
	if _, err := routes.ApplyEnvironment(t.Context(), in); err != nil {
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
	in.Revision, in.ConfigSHA256, in.DefaultWriteProfile = storageEnvironmentRevision(tx, "native-next"), strings.Repeat("9", 64), profiles[1].ID
	if _, err := routes.ApplyEnvironment(t.Context(), in); err != nil {
		t.Fatal("guarded native bootstrap could not change revision", err)
	}
	if err := routes.FinalizeStorageSchema(t.Context(), in.ConfigSHA256); err != nil {
		t.Fatal("safe finalization replay rejected guarded lineage", err)
	}
	var legacy int
	if err := tx.Raw(`SELECT count(*) FROM information_schema.tables WHERE table_schema='media' AND table_name='storage_legacy_binding'`).Scan(&legacy).Error; err != nil || legacy != 0 {
		t.Fatal("a fake original COS was manufactured", err)
	}
}
