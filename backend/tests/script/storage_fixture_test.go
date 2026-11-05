package script_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	scriptwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/script"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
	scriptobjects "github.com/StephenQiu30/glowloom-server/backend/internal/script/adapter/objects"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/script/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/domain"
)

type scriptStorageSettings struct {
	Endpoint, Bucket, Region string
	AccessKey                string `json:"access_key"`
	SecretKey                string `json:"secret_key"`
}

func scriptObjectSettings(t *testing.T, required bool) scriptStorageSettings {
	t.Helper()
	file := os.Getenv("GLOWLOOM_TEST_RECEIPT_STORAGE_CONFIG")
	if file == "" {
		if required {
			t.Skip("set isolated GLOWLOOM_TEST_RECEIPT_STORAGE_CONFIG; no business configuration is read")
		}
		return scriptStorageSettings{Endpoint: "http://127.0.0.1:1", Bucket: "script-metadata-fixture", Region: "us-east-1"}
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal("read explicitly selected script object fixture")
	}
	var settings scriptStorageSettings
	if json.Unmarshal(raw, &settings) != nil || settings.Endpoint == "" || settings.Bucket == "" || settings.AccessKey == "" || settings.SecretKey == "" {
		t.Fatal("invalid explicit script object fixture")
	}
	return settings
}

func scriptFixtureProfile(settings scriptStorageSettings) media.StorageProfile {
	sum := sha256.Sum256([]byte(settings.Endpoint + "/" + settings.Bucket))
	return media.StorageProfile{ID: "script-fixture-" + hex.EncodeToString(sum[:6]), Provider: "cos", Endpoint: settings.Endpoint, SigningEndpoint: settings.Endpoint, Bucket: settings.Bucket, Region: settings.Region}
}

func admitScriptFixtureProfile(t *testing.T, owner *gorm.DB) {
	t.Helper()
	profile := scriptFixtureProfile(scriptObjectSettings(t, false))
	applyScriptFixtureEnvironment(t, owner, []media.StorageProfile{profile}, profile.ID)
}

// nextScriptFixtureRevision follows the deployment contract even when a fixture
// database retains earlier test runs. Invalid stored revisions fail the fixture;
// they are never translated into a compatibility revision.
func nextScriptFixtureRevision(t *testing.T, db *gorm.DB) string {
	t.Helper()
	var revisions []string
	if err := db.WithContext(t.Context()).Raw(`SELECT revision FROM media.storage_config_revision UNION SELECT revision FROM media.storage_route`).Scan(&revisions).Error; err != nil {
		t.Fatal("read script fixture storage revisions", err)
	}
	var highest uint64
	for _, value := range revisions {
		revision, err := config.ParseStorageRevision(value)
		if err != nil {
			t.Fatal("invalid stored script fixture storage revision", err)
		}
		highest = max(highest, revision)
	}
	if highest == math.MaxUint64 {
		t.Fatal("script fixture storage revision exhausted")
	}
	return strconv.FormatUint(highest+1, 10)
}

func applyScriptFixtureEnvironment(t *testing.T, db *gorm.DB, profiles []media.StorageProfile, selected string) string {
	t.Helper()
	revision := nextScriptFixtureRevision(t, db)
	environment := media.StorageEnvironment{Revision: revision, DefaultWriteProfile: selected, UploadSessionTTL: time.Hour,
		UploadPolicy: media.UploadPolicy{MaxImageBytes: media.MaxUploadImageBytes, MaxVideoBytes: media.MaxUploadVideoBytes, MaxAudioBytes: media.MaxUploadAudioBytes, MaxModelBytes: media.MaxUploadModelBytes, MaxDocumentBytes: media.MaxUploadDocumentBytes}}
	for _, profile := range profiles {
		environment.Profiles = append(environment.Profiles, media.StorageDeploymentProfile{StorageProfile: profile, Mode: media.StorageProfileModeReadWrite})
	}
	raw, err := json.Marshal(environment)
	if err != nil {
		t.Fatal(err)
	}
	environment.ConfigSHA256 = domain.ContentSHA(raw)
	if _, err := pgmedia.NewStorageRoutes(db).ApplyEnvironment(t.Context(), environment); err != nil {
		t.Fatal(err)
	}
	return revision
}

func scriptFixtureRegistry(t *testing.T) *objectstorage.Registry {
	t.Helper()
	settings := scriptObjectSettings(t, true)
	profile := scriptFixtureProfile(settings)
	input := config.ObjectStorageConfig{SchemaVersion: 1, Revision: "1", DefaultWriteProfile: profile.ID,
		Upload:   config.StorageUploadPolicy{Mode: config.StorageUploadRelay, SignedURLTTL: 5 * time.Minute, SessionTTL: time.Hour, MultipartThresholdBytes: 50 << 20, PartSizeBytes: 5 << 20, PartConcurrency: 2, StagingRetention: 24 * time.Hour},
		Access:   config.StorageAccessPolicy{PreviewTTL: 5 * time.Minute, MaxSignedURLTTL: 15 * time.Minute},
		Limits:   config.StorageLimits{MaxImageBytes: media.MaxUploadImageBytes, MaxVideoBytes: media.MaxUploadVideoBytes, MaxAudioBytes: media.MaxUploadAudioBytes, MaxModelBytes: media.MaxUploadModelBytes, MaxDocumentBytes: media.MaxUploadDocumentBytes},
		Profiles: map[string]config.StorageProfile{profile.ID: {Provider: config.StorageProviderCOS, Mode: config.StorageProfileReadWrite, Endpoint: profile.Endpoint, SigningEndpoint: profile.SigningEndpoint, Bucket: profile.Bucket, Region: profile.Region, Credentials: config.StorageCredentialsRef{Source: config.StorageCredentialsEnvironment, AccessKeyEnv: "SCRIPT_TEST_ACCESS", SecretKeyEnv: "SCRIPT_TEST_SECRET"}}}}
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadObjectStorageEnvironment(string(raw), "local", func(name string) string {
		switch name {
		case "SCRIPT_TEST_ACCESS":
			return settings.AccessKey
		case "SCRIPT_TEST_SECRET":
			return settings.SecretKey
		default:
			return ""
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := objectstorage.OpenProfiles(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registry.Close)
	return registry
}

func scriptPrivateObjects(t *testing.T, db *gorm.DB) *scriptobjects.RoutedStorage {
	t.Helper()
	return scriptobjects.NewRoutedStorage(scriptFixtureRegistry(t), pgmedia.NewStorageRoutes(db))
}

// These helpers inject corruption or remove test-owned bytes. They deliberately
// bypass the business write/removal fence, while preserving the saved physical
// profile/bucket/key; they are never installed as an application object port.
func scriptFixtureObjectRef(t *testing.T, db *gorm.DB, fact domain.ObjectFact) objectstorage.Ref {
	t.Helper()
	if fact.LocationID == uuid.Nil {
		t.Fatal("fixture object lacks a persisted location")
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 10*time.Second)
	defer cancel()
	location, err := pgmedia.NewStorageRoutes(db).ReadLocation(ctx, fact.LocationID)
	if err != nil || location.Ref.Key != fact.Key {
		t.Fatal("fixture physical identity mismatch", err)
	}
	return objectstorage.Ref{ProfileID: location.Ref.ProfileID, Bucket: location.Ref.Bucket, Key: location.Ref.Key}
}
func removeScriptFixtureBytes(t *testing.T, db *gorm.DB, fact domain.ObjectFact) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 10*time.Second)
	defer cancel()
	return scriptFixtureRegistry(t).Remove(ctx, scriptFixtureObjectRef(t, db, fact))
}
func putScriptFixtureBytes(t *testing.T, db *gorm.DB, fact domain.ObjectFact, body io.Reader, size int64, mime, sha string) error {
	t.Helper()
	return scriptFixtureRegistry(t).PutIfAbsent(t.Context(), scriptFixtureObjectRef(t, db, fact), body, size, mime, sha)
}

func writeScriptFixtureObject(t *testing.T, db *gorm.DB, project uuid.UUID, fact domain.ObjectFact, data []byte) domain.ObjectFact {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	locations := scriptwire.ProvideScriptObjectLocations(db)
	intent, facts, err := locations.Admit(ctx, project, uuid.New(), []domain.ObjectFact{fact})
	if err != nil {
		t.Fatal(err)
	}
	fact = facts[0]
	if err := locations.Begin(ctx, intent, fact); err != nil {
		t.Fatal(err)
	}
	if err := scriptPrivateObjects(t, db).PutObjectIfAbsent(ctx, fact, bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	if err := locations.Confirm(ctx, intent, fact); err != nil {
		t.Fatal(err)
	}
	if err := locations.Accept(ctx, intent, facts); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := removeScriptFixtureBytes(t, db, fact); err != nil {
			t.Error(err)
		}
	})
	return fact
}

// This isolated database clock fixture models the natural ten-minute remote
// write protection expiring. It never changes work/location states or receipts.
func expireScriptFixtureDrain(t *testing.T, owner *gorm.DB, intent uuid.UUID) {
	t.Helper()
	var database string
	if err := owner.Raw(`SELECT current_database()`).Scan(&database).Error; err != nil || database != "glowloom_reference" && (!strings.HasPrefix(database, "glowloom_script_") || !strings.HasSuffix(database, "_test")) && database != "glowloom_subject_storage_script_test" {
		t.Fatal("clock fixture requires the explicitly selected isolated test database", database, err)
	}
	if intent == uuid.Nil {
		t.Fatal("drain fixture lacks original intent")
	}
	var row struct {
		Closed bool
		State  string
	}
	q := owner.Raw(`SELECT closed_at IS NOT NULL AS closed,state FROM media.storage_work_intent WHERE id=?`, intent).Scan(&row)
	if q.Error != nil || q.RowsAffected != 1 || !row.Closed {
		t.Fatal("cancellation clock was not durably recorded", intent, row.State, q.Error)
	}
	if err := owner.Transaction(func(tx *gorm.DB) error {
		// Timestamp immutability is a production invariant. This owner-only test
		// transaction advances no other column and restores both exact guards
		// before exercising the real recovery path.
		if err := tx.Exec(`ALTER TABLE media.object_location DISABLE TRIGGER keep_storage_write_deadline`).Error; err != nil {
			return err
		}
		if err := tx.Exec(`ALTER TABLE media.storage_work_intent DISABLE TRIGGER keep_storage_work`).Error; err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE media.object_location SET write_until=clock_timestamp()-interval '11 minutes' WHERE owner_intent_id=? AND write_until IS NOT NULL`, intent).Error; err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE media.storage_work_intent SET closed_at=clock_timestamp()-interval '11 minutes' WHERE id=? AND closed_at IS NOT NULL`, intent).Error; err != nil {
			return err
		}
		if err := tx.Exec(`ALTER TABLE media.object_location ENABLE TRIGGER keep_storage_write_deadline`).Error; err != nil {
			return err
		}
		return tx.Exec(`ALTER TABLE media.storage_work_intent ENABLE TRIGGER keep_storage_work`).Error
	}); err != nil {
		t.Fatal(err)
	}
}

func expireScriptFixtureSourceDrain(t *testing.T, owner *gorm.DB, plan app.WritePlan) {
	t.Helper()
	var exited bool
	q := owner.Raw(`SELECT io_state IN ('idle','ended') FROM script.command_state WHERE actor_id=? AND request_id=?`, plan.ActorID, plan.Command.Key).Scan(&exited)
	if q.Error != nil || q.RowsAffected != 1 || !exited {
		t.Fatal("cannot expire a source window without durable SDK exit", q.Error)
	}
	expireScriptFixtureDrain(t, owner, plan.StorageIntentID)
}

func assertScriptFixtureObjectPresent(t *testing.T, objects app.PrivateObjects, fact domain.ObjectFact) {
	t.Helper()
	r, err := objects.GetObject(t.Context(), fact)
	if err != nil {
		t.Fatal("protected object removed before drain expiry", err)
	}
	data, err := io.ReadAll(r)
	closeErr := r.Close()
	if err != nil || closeErr != nil || int64(len(data)) != fact.ByteSize || domain.ContentSHA(data) != fact.SHA256 {
		t.Fatal("protected original bytes changed", err, closeErr)
	}
}
