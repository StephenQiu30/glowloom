package identity_test

import (
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
	"github.com/StephenQiu30/glowloom-server/backend/tests/storagefixture"
)

func seedIdentityStorageRoute(t *testing.T, tx *gorm.DB) {
	t.Helper()
	if err := tx.Exec(`INSERT INTO media.storage_profile(id,provider,endpoint,signing_endpoint,region,bucket,addressing_style,resource_identity) VALUES('identity-fixture','minio','http://127.0.0.1:57924','http://127.0.0.1:57924','us-east-1','identity-fixture','path',repeat('b',64)) ON CONFLICT(id) DO NOTHING`).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`INSERT INTO media.storage_route(singleton,revision,profile_id) VALUES(1,'1','identity-fixture') ON CONFLICT(singleton) DO UPDATE SET profile_id=excluded.profile_id,revision=excluded.revision`).Error; err != nil {
		t.Fatal(err)
	}
}

func identityRouterStorage(t *testing.T, cfg *config.Config) *objectstorage.Registry {
	t.Helper()
	storage := storagefixture.Configuration(t, "http://127.0.0.1:57924", "glowloom-receipt-test")
	cfg.ObjectStorage = &storage
	registry, err := objectstorage.OpenProfiles(storage)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registry.Close)
	return registry
}

func seedIdentityLocation(t *testing.T, tx *gorm.DB, key string) uuid.UUID {
	t.Helper()
	seedIdentityStorageRoute(t, tx)
	id := uuid.New()
	if err := tx.Exec(`INSERT INTO media.object_location(id,profile_id,bucket,object_key,state) VALUES(?,'identity-fixture','identity-fixture',?,'accepted')`, id, key).Error; err != nil {
		t.Fatal(err)
	}
	return id
}
