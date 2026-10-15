package media_test

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

func TestStorageActivationRequiresMigrationProbesAndEveryLiveRole(t *testing.T) {
	db := storageRouteDB(t).Begin()
	t.Cleanup(func() { _ = db.Rollback().Error })
	routes := pgmedia.NewStorageRoutes(db)
	profiles := storageProfiles(t, routes)
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
	ready := func(in mediaapp.StorageActivation) {
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
		}
		for _, role := range []string{"api", "worker", "relay", "setup"} {
			if err := routes.ReportStorageRuntime(t.Context(), mediaapp.StorageRuntime{InstanceID: uuid.New(), Role: role, Revision: in.Revision, ConfigSHA256: in.ConfigSHA256, Capabilities: required}, time.Minute); err != nil {
				t.Fatal(err)
			}
		}
	}
	first := mediaapp.StorageActivation{Revision: "gate-first", DefaultWriteProfile: "minio-main", ConfigSHA256: strings.Repeat("1", 64), UploadPolicy: uploadRoutingPolicy(), UploadSessionTTL: 24 * time.Hour, Profiles: []mediaapp.StorageDeploymentProfile{{StorageProfile: profiles[0], Mode: "read_write"}, {StorageProfile: profiles[1], Mode: "read_write"}}}
	ready(first)
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
	ready(next)
	if _, err := routes.ActivateStorage(t.Context(), next); !errors.Is(err, mediaapp.ErrStorageNotReady) {
		t.Fatal("inflight MinIO writer lost permission", err)
	}
	next.Profiles[0].Mode = "read_write"
	next.ConfigSHA256 = strings.Repeat("3", 64)
	ready(next)
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
