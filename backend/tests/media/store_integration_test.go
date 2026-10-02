package media_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

func mediaStoreDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("LV_TEST_MEDIA_STORE_DB_DSN")
	if dsn == "" {
		t.Skip("set LV_TEST_MEDIA_STORE_DB_DSN to an isolated PostgreSQL database initialized from db/schema.sql")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatalf("open media store database: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn.DB.WithContext(ctx)
}

func mediaStoreProject(t *testing.T, database *gorm.DB) (identityapp.Principal, uuid.UUID) {
	t.Helper()
	orgID, userID, projectID := uuid.New(), uuid.New(), uuid.New()
	if err := database.Exec(`INSERT INTO workspace.organization (id, name) VALUES (?::uuid, ?)`,
		orgID.String(), "media-"+orgID.String()).Error; err != nil {
		t.Fatalf("create organization: %v", err)
	}
	if err := database.Exec(`
		INSERT INTO identity."user" (id, org_id, login_name, display_name, role, password_hash, must_change_password)
		VALUES (?::uuid, ?::uuid, ?, 'Media Producer', 'producer', 'test-hash', false)
	`, userID.String(), orgID.String(), "media-"+userID.String()).Error; err != nil {
		t.Fatalf("create actor: %v", err)
	}
	if err := database.Exec(`
		INSERT INTO workspace.project (id, org_id, name, aspect_ratio, style_type)
		VALUES (?::uuid, ?::uuid, '媒体测试', '16:9', 'realistic')
	`, projectID.String(), orgID.String()).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	return identityapp.Principal{ID: userID, OrgID: orgID, Role: identitydomain.RoleProducer}, projectID
}

func TestMediaStoreScopesAssetsAndRenditionsToLiveProject(t *testing.T) {
	database := mediaStoreDB(t)
	actor, projectID := mediaStoreProject(t, database)
	otherActor, otherProjectID := mediaStoreProject(t, database)
	store := pgmedia.NewStore(database)
	assetID, renditionID := uuid.New(), uuid.New()
	now := time.Now().UTC()
	asset := domain.MediaAsset{
		ID: assetID, ProjectID: projectID, Kind: domain.KindImage,
		Origin: domain.OriginUpload, Status: domain.StatusUploading,
		ObjectKey: "projects/" + projectID.String() + "/image/2026/09/" + assetID.String() + ".png",
		FileName:  "reference.png", MimeType: "image/png", ByteSize: 12,
		ModerationStatus: domain.ModerationPending, Revision: 1,
		CreateTime: now, UpdateTime: now,
	}
	if err := store.CreateAsset(t.Context(), actor, asset); err != nil {
		t.Fatalf("create asset: %v", err)
	}
	r := domain.Rendition{
		ID: renditionID, MediaAssetID: assetID, Kind: domain.RenditionThumb256,
		ObjectKey:  "projects/" + projectID.String() + "/image/2026/09/" + assetID.String() + "/thumb_256.webp",
		CreateTime: now, UpdateTime: now,
	}
	if err := store.AddRendition(t.Context(), actor, projectID, r); err != nil {
		t.Fatalf("add rendition: %v", err)
	}
	wrongKey := domain.Rendition{
		ID: uuid.New(), MediaAssetID: assetID, Kind: domain.RenditionThumb640,
		ObjectKey:  "projects/" + otherProjectID.String() + "/image/2026/09/" + assetID.String() + "/thumb_640.webp",
		CreateTime: now, UpdateTime: now,
	}
	if err := store.AddRendition(t.Context(), actor, projectID, wrongKey); !errors.Is(err, domain.ErrInvalidRendition) {
		t.Fatalf("foreign object key accepted: %v", err)
	}
	got, err := store.FindAsset(t.Context(), actor, projectID, assetID)
	if err != nil || got.ID != assetID || got.ProjectID != projectID || got.ObjectKey != asset.ObjectKey {
		t.Fatalf("find asset = %+v: %v", got, err)
	}
	rends, err := store.FindRenditions(t.Context(), actor, projectID, assetID)
	if err != nil || len(rends) != 1 || rends[0].ID != renditionID {
		t.Fatalf("find renditions = %+v: %v", rends, err)
	}
	for _, scope := range []struct {
		name      string
		actor     identityapp.Principal
		projectID uuid.UUID
	}{
		{"foreign organization", otherActor, projectID},
		{"wrong project", actor, otherProjectID},
	} {
		t.Run(scope.name, func(t *testing.T) {
			if _, err := store.FindAsset(t.Context(), scope.actor, scope.projectID, assetID); !errors.Is(err, pgmedia.ErrNotFound) {
				t.Fatalf("asset scope error = %v", err)
			}
			if _, err := store.FindRenditions(t.Context(), scope.actor, scope.projectID, assetID); !errors.Is(err, pgmedia.ErrNotFound) {
				t.Fatalf("rendition scope error = %v", err)
			}
			if err := store.AddRendition(t.Context(), scope.actor, scope.projectID, domain.Rendition{
				ID: uuid.New(), MediaAssetID: assetID, Kind: domain.RenditionThumb640,
				ObjectKey: "should-not-exist", CreateTime: now, UpdateTime: now,
			}); !errors.Is(err, pgmedia.ErrNotFound) && !errors.Is(err, identityapp.ErrForbidden) {
				t.Fatalf("cross-project rendition write error = %v", err)
			}
		})
	}
	if err := database.Exec(`UPDATE identity."user" SET status = 'disabled' WHERE id = ?::uuid`, actor.ID.String()).Error; err != nil {
		t.Fatalf("disable actor: %v", err)
	}
	if _, err := store.FindAsset(t.Context(), actor, projectID, assetID); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatalf("revoked actor read error = %v", err)
	}
	if err := database.Exec(`UPDATE identity."user" SET status = 'active' WHERE id = ?::uuid`, actor.ID.String()).Error; err != nil {
		t.Fatalf("enable actor: %v", err)
	}
	if err := database.Exec(`UPDATE workspace.project SET is_delete = true WHERE id = ?::uuid`, projectID.String()).Error; err != nil {
		t.Fatalf("delete project: %v", err)
	}
	if _, err := store.FindAsset(t.Context(), actor, projectID, assetID); !errors.Is(err, pgmedia.ErrNotFound) {
		t.Fatalf("deleted project read error = %v", err)
	}
}

func TestMediaStoreRequiresActiveProjectForCreation(t *testing.T) {
	database := mediaStoreDB(t)
	actor, projectID := mediaStoreProject(t, database)
	otherActor, _ := mediaStoreProject(t, database)
	store := pgmedia.NewStore(database)
	now := time.Now().UTC()
	makeAsset := func() domain.MediaAsset {
		assetID := uuid.New()
		return domain.MediaAsset{
			ID: assetID, ProjectID: projectID, Kind: domain.KindVideo,
			Origin: domain.OriginUpload, Status: domain.StatusUploading,
			ObjectKey: "projects/" + projectID.String() + "/video/2026/09/" + assetID.String() + ".mp4",
			FileName:  "clip.mp4", MimeType: "video/mp4", ByteSize: 100,
			ModerationStatus: domain.ModerationPending, Revision: 1,
			CreateTime: now, UpdateTime: now,
		}
	}
	if err := store.CreateAsset(t.Context(), otherActor, makeAsset()); !errors.Is(err, pgmedia.ErrNotFound) {
		t.Fatalf("foreign actor creation error = %v", err)
	}
	wrongKey := makeAsset()
	wrongKey.ObjectKey = "projects/" + otherActor.OrgID.String() + "/video/2026/09/" + wrongKey.ID.String() + ".mp4"
	if err := store.CreateAsset(t.Context(), actor, wrongKey); !errors.Is(err, domain.ErrInvalidMediaAsset) {
		t.Fatalf("foreign object key creation error = %v", err)
	}
	if err := database.Exec(`UPDATE workspace.project SET status = 'archived' WHERE id = ?::uuid`, projectID.String()).Error; err != nil {
		t.Fatalf("archive project: %v", err)
	}
	if err := store.CreateAsset(t.Context(), actor, makeAsset()); !errors.Is(err, pgmedia.ErrProjectStateConflict) {
		t.Fatalf("archived project creation error = %v", err)
	}
	var count int64
	if err := database.Raw(`SELECT count(*) FROM media.media_asset WHERE project_id = ?::uuid`, projectID.String()).Scan(&count).Error; err != nil || count != 0 {
		t.Fatalf("rejected writes persisted %d assets: %v", count, err)
	}
}

func TestMediaStoreReturnsEmptyRenditionListForVisibleAsset(t *testing.T) {
	database := mediaStoreDB(t)
	actor, projectID := mediaStoreProject(t, database)
	assetID := uuid.New()
	if err := database.Exec(`
		INSERT INTO media.media_asset (id, project_id, kind, origin, status, object_key, mime_type, byte_size)
		VALUES (?::uuid, ?::uuid, 'audio', 'upload', 'uploading', ?, 'audio/mpeg', 10)
	`, assetID.String(), projectID.String(), "projects/"+projectID.String()+"/audio/2026/09/"+assetID.String()+".mp3").Error; err != nil {
		t.Fatalf("seed asset: %v", err)
	}
	rends, err := pgmedia.NewStore(database).FindRenditions(t.Context(), actor, projectID, assetID)
	if err != nil || len(rends) != 0 {
		t.Fatalf("empty renditions = %+v: %v", rends, err)
	}
}

func TestMediaStorePreservesGeneratedProvenanceAndRejectsForeignOperation(t *testing.T) {
	database := mediaStoreDB(t)
	actor, projectID := mediaStoreProject(t, database)
	_, foreignProjectID := mediaStoreProject(t, database)
	createOperation := func(projectID uuid.UUID) uuid.UUID {
		t.Helper()
		id := uuid.New()
		if err := database.Exec(`
			INSERT INTO operation.operation
			  (id, project_id, target_type, capability, mode, input_hash,
			   origin, status, region, model_profile_version_id)
			VALUES (?::uuid, ?::uuid, 'free', 'video.generate', 'text_to_video', ?,
			        'canvas', 'draft', 'overseas', ?::uuid)
		`, id.String(), projectID.String(), "media-"+id.String(), uuid.NewString()).Error; err != nil {
			t.Fatalf("create operation: %v", err)
		}
		return id
	}
	localOperation := createOperation(projectID)
	foreignOperation := createOperation(foreignProjectID)
	provider, model, region := "openrouter", "video-model", "overseas"
	assetID := uuid.New()
	now := time.Now().UTC()
	asset := domain.MediaAsset{
		ID: assetID, ProjectID: projectID, Kind: domain.KindVideo,
		Origin: domain.OriginGenerated, Status: domain.StatusProcessing,
		ObjectKey: "projects/" + projectID.String() + "/video/2026/09/" + assetID.String() + ".mp4",
		MimeType:  "video/mp4", ByteSize: 120,
		SourceOperationID: &localOperation, ProviderKey: &provider, ModelKey: &model,
		Region: &region, ModerationStatus: domain.ModerationPending,
		Revision: 1, CreateTime: now, UpdateTime: now,
	}
	store := pgmedia.NewStore(database)
	if err := store.CreateAsset(t.Context(), actor, asset); err != nil {
		t.Fatalf("create generated asset: %v", err)
	}
	got, err := store.FindAsset(t.Context(), actor, projectID, assetID)
	if err != nil || got.Origin != domain.OriginGenerated || got.SourceOperationID == nil ||
		*got.SourceOperationID != localOperation || got.ProviderKey == nil || *got.ProviderKey != provider ||
		got.ModelKey == nil || *got.ModelKey != model || got.Region == nil || *got.Region != region {
		t.Fatalf("generated provenance = %+v: %v", got, err)
	}
	asset.ID = uuid.New()
	asset.ObjectKey = "projects/" + projectID.String() + "/video/2026/09/" + asset.ID.String() + ".mp4"
	asset.SourceOperationID = &foreignOperation
	if err := store.CreateAsset(t.Context(), actor, asset); err == nil {
		t.Fatal("cross-project operation was accepted as generated media source")
	}
	if _, err := store.FindAsset(t.Context(), actor, projectID, asset.ID); !errors.Is(err, pgmedia.ErrNotFound) {
		t.Fatalf("failed generated asset persisted: %v", err)
	}
}
