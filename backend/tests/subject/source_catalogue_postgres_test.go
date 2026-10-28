package subject_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// This fresh legacy-shaped SQL fixture intentionally permits metadata IDs to
// differ from asset IDs. It never weakens an existing schema or reads real data.
func sourceCataloguePostgres(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("GLOWLOOM_TEST_SUBJECT_SOURCE_DB_DSN")
	if dsn == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("CI requires the isolated subject-source PostgreSQL database")
		}
		t.Skip("requires a fresh loopback subject-source fixture database")
	}
	u, err := url.Parse(dsn)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || !strings.HasPrefix(u.Path, "/glowloom_subject_source_") || strings.Contains(strings.TrimPrefix(u.Path, "/"), "/") {
		t.Fatal("requires an isolated subject-source database")
	}
	host := u.Hostname()
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		t.Fatal("subject-source fixture requires loopback PostgreSQL")
	}
	for key := range u.Query() {
		if key != "sslmode" {
			t.Fatal("unexpected source fixture connection override")
		}
	}
	db, err := gorm.Open(gormpostgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("open isolated subject-source PostgreSQL")
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	var existing int64
	if err := db.Raw(`SELECT count(*) FROM information_schema.schemata WHERE schema_name IN ('workspace','media','operation')`).Scan(&existing).Error; err != nil || existing != 0 {
		t.Fatal("requires a fresh database with no source schemas")
	}
	const ddl = `CREATE SCHEMA workspace;CREATE SCHEMA media;CREATE SCHEMA operation;
 CREATE TABLE workspace.project(id uuid PRIMARY KEY,org_id uuid,owner_scope_id uuid,status text DEFAULT 'active',is_delete boolean DEFAULT false);
 CREATE TABLE media.library(id uuid PRIMARY KEY,kind text,org_id uuid,project_id uuid,personal_actor_id uuid,revision bigint DEFAULT 0);
 CREATE TABLE media.library_item(id uuid PRIMARY KEY,library_id uuid,asset_id uuid,plain_text text,folder_id uuid,title text,category text DEFAULT 'material',tags jsonb DEFAULT '[]',source_label text DEFAULT '',note text DEFAULT '',favorite boolean DEFAULT false,catalog_state text DEFAULT 'active',trashed_at timestamptz,position bigint DEFAULT 0,revision bigint DEFAULT 1,create_time timestamptz DEFAULT now(),update_time timestamptz DEFAULT now(),purged_at timestamptz);
 CREATE TABLE media.media_asset(id uuid PRIMARY KEY,project_id uuid,personal_org_id uuid,personal_actor_id uuid,kind text,origin text,status text,object_key text,file_name text,mime_type text,byte_size bigint,sha256 text,width int,height int,duration_ms int,source_operation_id uuid,provider_key text,model_key text,region text,moderation_status text,aigc_marked boolean DEFAULT false,contains_real_person boolean DEFAULT false,consent_record_id uuid,revision int DEFAULT 1,create_time timestamptz DEFAULT now(),update_time timestamptz DEFAULT now(),is_delete boolean DEFAULT false);
 CREATE TABLE media.purge_item(item_id uuid,status text);
 CREATE TABLE operation.operation(id uuid PRIMARY KEY,project_id uuid,capability text,mode text,status text,origin text DEFAULT 'canvas',confirmed_by uuid,is_delete boolean DEFAULT false);
 CREATE TABLE operation.operation_output(id uuid PRIMARY KEY,project_id uuid,operation_id uuid,media_asset_id uuid,kind text DEFAULT 'media',moderation_status text DEFAULT 'passed',create_time timestamptz DEFAULT now(),update_time timestamptz DEFAULT now(),is_delete boolean DEFAULT false);
 CREATE TABLE operation.operation_input(operation_id uuid,text_value text,seq_no int DEFAULT 0,is_delete boolean DEFAULT false);
 CREATE TABLE operation.prompt_submission(operation_id uuid,project_id uuid,actor_id uuid);
 CREATE TABLE operation.quote_request(org_id uuid,actor_id uuid,result jsonb);
 CREATE TABLE operation.personal_history_record(actor_id uuid,org_id uuid,scope_id uuid,operation_id uuid,deleted_at timestamptz);
 CREATE TABLE operation.personal_history_output(actor_id uuid,org_id uuid,scope_id uuid,operation_id uuid,output_id uuid,revision bigint DEFAULT 0,favorite boolean DEFAULT false,deleted_at timestamptz);`
	if err := db.Exec(ddl).Error; err != nil {
		t.Fatal(err)
	}
	return db
}

type catalogueAccess struct {
	tx               *gorm.DB
	target, personal identity.Principal
}

func (a catalogueAccess) CheckActor(_ context.Context, actor identity.Principal) error {
	if actor != a.target && actor != a.personal {
		return identity.ErrForbidden
	}
	return nil
}
func (a catalogueAccess) AuthorizePersonal(_ context.Context, actor identity.Principal, _ bool) error {
	if actor != a.personal {
		return identity.ErrForbidden
	}
	return nil
}
func (a catalogueAccess) AuthorizeProject(_ context.Context, actor identity.Principal, id uuid.UUID, write bool) error {
	if actor != a.target {
		return identity.ErrForbidden
	}
	var found int64
	err := a.tx.Raw(`SELECT count(*) FROM workspace.project WHERE id=? AND org_id=? AND owner_scope_id=? AND NOT is_delete AND (status='active' OR status='archived' AND NOT ?)`, id, actor.OrgID, actor.ScopeID, write).Scan(&found).Error
	if err != nil {
		return err
	}
	if found != 1 {
		return media.ErrNotFound
	}
	return nil
}
func (a catalogueAccess) AuthorizeCopyTarget(ctx context.Context, actor identity.Principal, id uuid.UUID) error {
	return a.AuthorizeProject(ctx, actor, id, true)
}
func (a catalogueAccess) Authorize(ctx context.Context, actor identity.Principal, id uuid.UUID, write bool) (media.LibraryProjectFacts, error) {
	err := a.AuthorizeProject(ctx, actor, id, write)
	return media.LibraryProjectFacts{ProjectID: id, OrgID: actor.OrgID, Revision: 1}, err
}
func (catalogueAccess) TouchContent(context.Context, identity.Principal, uuid.UUID, int64) (int64, error) {
	return 0, media.ErrUnavailable
}

func TestSubjectCatalogueRealPostgresMixedPagingProvenanceAndLegacyMetadata(t *testing.T) {
	database := sourceCataloguePostgres(t)
	target := identity.Principal{ID: uuid.New(), OrgID: uuid.New(), ScopeID: uuid.New(), SessionID: uuid.New(), CredentialEpoch: 1}
	personal := target
	personal.OrgID = uuid.New()
	personal.ScopeID = uuid.New()
	access := func(tx *gorm.DB) catalogueAccess { return catalogueAccess{tx: tx, target: target, personal: personal} }
	library := pgmedia.NewScopedLibraryStore(database, func(tx *gorm.DB) media.LibraryProjectAccess { return access(tx) }, time.Now, func(tx *gorm.DB) media.ScopeAccess { return access(tx) })
	store := pgmedia.NewSubjectCatalogueStore(database, library, func(*gorm.DB) media.SubjectSourceActor { return &sourceActorFake{personal: personal} })
	personalID, _ := (mediadomain.LibraryScope{Kind: mediadomain.LibraryPersonal}).Identity(personal.OrgID, personal.ID)
	project, foreignProject := uuid.New(), uuid.New()
	if err := database.Exec(`INSERT INTO workspace.project(id,org_id,owner_scope_id) VALUES(?,?,?),(?,?,?)`, project, target.OrgID, target.ScopeID, foreignProject, target.OrgID, uuid.New()).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`INSERT INTO media.library(id,kind,org_id,personal_actor_id) VALUES(?,'personal',?,?)`, personalID, personal.OrgID, personal.ID).Error; err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	upload, metadata := uuid.New(), uuid.New()
	key := fmt.Sprintf("personal/%s/%s/image/2026/10/%s.png", personal.OrgID, personal.ID, upload)
	if err := database.Exec(`INSERT INTO media.media_asset(id,personal_org_id,personal_actor_id,kind,origin,status,object_key,file_name,mime_type,byte_size,sha256,width,height,moderation_status,create_time,update_time) VALUES(?,?,?,'image','upload','ready',?,'原生8K.png','image/png',20,?,8192,8192,'passed',?,?)`, upload, personal.OrgID, personal.ID, key, strings.Repeat("a", 64), base.Add(10*time.Minute), base.Add(10*time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`INSERT INTO media.library_item(id,library_id,asset_id,title,favorite,create_time,update_time) VALUES(?,?,?,'旧元数据标题',true,?,?)`, metadata, personalID, upload, base.Add(10*time.Minute), base.Add(10*time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	type output struct{ id, operation, asset uuid.UUID }
	seed := func(p, initiator uuid.UUID, channel, capability, mode, kind string, minute int) output {
		t.Helper()
		result := output{id: uuid.New(), operation: uuid.New(), asset: uuid.New()}
		if err := database.Exec(`INSERT INTO operation.operation(id,project_id,capability,mode,status,confirmed_by) VALUES(?,?,?,?,'completed',?)`, result.operation, p, capability, mode, target.ID).Error; err != nil {
			t.Fatal(err)
		}
		var receipt string
		switch channel {
		case "prompt":
			if err := database.Exec(`INSERT INTO operation.prompt_submission(operation_id,project_id,actor_id) VALUES(?,?,?)`, result.operation, p, initiator).Error; err != nil {
				t.Fatal(err)
			}
		case "batch":
			receipt = `{"items":[{"operation_id":"` + result.operation.String() + `"}]}`
		default:
			receipt = `{"operation_id":"` + result.operation.String() + `"}`
		}
		if receipt != "" {
			if err := database.Exec(`INSERT INTO operation.quote_request(org_id,actor_id,result)VALUES(?,?,?::jsonb)`, target.OrgID, initiator, receipt).Error; err != nil {
				t.Fatal(err)
			}
		}
		file := result.asset.String() + ".png"
		mime := "image/png"
		if kind == "video" {
			file = result.asset.String() + ".mp4"
			mime = "video/mp4"
		}
		key := fmt.Sprintf("projects/%s/%s/2026/10/%s", p, kind, file)
		at := base.Add(time.Duration(minute) * time.Minute)
		if err := database.Exec(`INSERT INTO media.media_asset(id,project_id,kind,origin,status,object_key,file_name,mime_type,byte_size,sha256,width,height,source_operation_id,provider_key,model_key,region,moderation_status,aigc_marked,create_time,update_time) VALUES(?,?,?,'generated','ready',?,?,?,20,?,2,2,?,'fixture','fixture','domestic','passed',true,?,?)`, result.asset, p, kind, key, file, mime, strings.Repeat("b", 64), result.operation, at, at).Error; err != nil {
			t.Fatal(err)
		}
		if err := database.Exec(`INSERT INTO operation.operation_output(id,project_id,operation_id,media_asset_id,create_time,update_time) VALUES(?,?,?,?,?,?)`, result.id, p, result.operation, result.asset, at, at).Error; err != nil {
			t.Fatal(err)
		}
		return result
	}
	normal := seed(project, target.ID, "prompt", "image.generate", "text2image", "image", 3)
	upscale := seed(project, target.ID, "single", "image.upscale", "upscale", "image", 8)
	modeUpscale := seed(project, target.ID, "batch", "image.generate", "super_resolution", "image", 6)
	video := seed(project, target.ID, "single", "video.super_resolution", "text2video", "video", 7)
	other := seed(project, uuid.New(), "single", "image.generate", "text2image", "image", 11)
	foreign := seed(foreignProject, target.ID, "single", "image.generate", "text2image", "image", 12)
	deleted := seed(project, target.ID, "single", "image.generate", "text2image", "image", 13)
	trashed := seed(project, target.ID, "single", "image.generate", "text2image", "image", 14)
	if err := database.Exec(`INSERT INTO operation.personal_history_output(actor_id,org_id,scope_id,operation_id,output_id,favorite) VALUES(?,?,?,?,?,true)`, target.ID, target.OrgID, target.ScopeID, upscale.operation, upscale.id).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`INSERT INTO operation.personal_history_record(actor_id,org_id,scope_id,operation_id,deleted_at) VALUES(?,?,?,?,now())`, target.ID, target.OrgID, target.ScopeID, deleted.operation).Error; err != nil {
		t.Fatal(err)
	}
	projectLibrary, _ := (mediadomain.LibraryScope{Kind: mediadomain.LibraryProject, ProjectID: &project}).Identity(target.OrgID, target.ID)
	if err := database.Exec(`INSERT INTO media.library(id,kind,org_id,project_id) VALUES(?,'project',?,?)`, projectLibrary, target.OrgID, project).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`INSERT INTO media.library_item(id,library_id,asset_id,title,catalog_state,trashed_at) VALUES(?,?,?,'回收站素材','trashed',now())`, trashed.asset, projectLibrary, trashed.asset).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`INSERT INTO operation.quote_request(org_id,actor_id,result) VALUES(?,?,?::jsonb)`, target.OrgID, target.ID, `{"operation_id":"not-a-uuid","items":false}`).Error; err != nil {
		t.Fatal(err)
	}
	query := media.SubjectCatalogueQuery{Kind: "image", Page: 1, PageSize: 2}
	page, err := store.List(t.Context(), target, query)
	if err != nil || page.Total != 4 || len(page.Items) != 2 || page.Items[0].ID != upload || page.Items[1].ID != upscale.id || page.Items[0].Revision != 1 || page.Items[0].Title != "旧元数据标题" || !page.Items[0].Favorite {
		t.Fatal("mixed global first page/legacy metadata", page, err)
	}
	query.Page = 2
	page, err = store.List(t.Context(), target, query)
	if err != nil || page.Total != 4 || len(page.Items) != 2 || page.Items[0].ID != modeUpscale.id || page.Items[1].ID != normal.id {
		t.Fatal("mixed global second page", page, err)
	}
	query.Page = 1
	query.Quality = "upscaled"
	page, err = store.List(t.Context(), target, query)
	if err != nil || page.Total != 2 || len(page.Items) != 2 || !page.Items[0].Upscaled || page.Items[0].ID == upload {
		t.Fatal("native resolution was mistaken for processed upscale", page, err)
	}
	query.Quality = ""
	query.FavoriteOnly = true
	page, err = store.List(t.Context(), target, query)
	if err != nil || page.Total != 2 || len(page.Items) != 2 {
		t.Fatal("mixed favorites were not filtered before paging", page, err)
	}
	query.FavoriteOnly = false
	query.Kind = "video"
	query.Quality = "upscaled"
	page, err = store.List(t.Context(), target, query)
	if err != nil || page.Total != 1 || page.Items[0].ID != video.id {
		t.Fatal("real video upscale source", page, err)
	}
	for _, hidden := range []output{other, foreign, deleted, trashed} {
		if _, err := store.Detail(t.Context(), target, hidden.id); !errors.Is(err, media.ErrNotFound) {
			t.Fatal("confirmer/foreign/deleted/trashed result exposed", hidden, err)
		}
	}
	item, err := store.Detail(t.Context(), target, upload)
	if err != nil || item.ID != upload || item.ID == metadata || item.Media.ID != upload || item.Source != "personal_asset" {
		t.Fatal("personal source identity used metadata id", item, err)
	}
	file, err := store.FindLibraryMedia(t.Context(), personal, mediadomain.LibraryScope{Kind: mediadomain.LibraryPersonal}, upload)
	if err != nil || file.Asset.ID != upload || file.Asset.Personal == nil || file.Asset.Personal.ActorID != target.ID {
		t.Fatal("legacy metadata preview could not resolve original by asset", err)
	}
	if err := database.Transaction(func(tx *gorm.DB) error {
		selected, err := pgmedia.NewSubjectSourceStore(tx, library).FindSubjectSource(t.Context(), personal, mediadomain.LibraryScope{Kind: mediadomain.LibraryPersonal}, upload)
		if err == nil && selected.Asset.ID != upload {
			return errors.New("selection loaded metadata identity")
		}
		return err
	}); err != nil {
		t.Fatal("legacy metadata original adoption failed", err)
	}
	if _, err := store.Detail(t.Context(), target, metadata); !errors.Is(err, media.ErrNotFound) {
		t.Fatal("metadata id became public source identity", err)
	}
	if err := database.Exec(`UPDATE workspace.project SET status='archived' WHERE id=?`, project).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := store.Detail(t.Context(), target, normal.id); err != nil {
		t.Fatal("archived read/source eligibility drifted", err)
	}
	if _, err := store.FindLibraryMedia(t.Context(), target, mediadomain.LibraryScope{Kind: mediadomain.LibraryProject, ProjectID: &project}, normal.asset); err != nil {
		t.Fatal("archived original read failed", err)
	}
	if err := database.Exec(`UPDATE operation.operation_output SET id=? WHERE id=?`, upload, normal.id).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := store.Detail(t.Context(), target, upload); !errors.Is(err, media.ErrUnavailable) {
		t.Fatal("colliding source id selected arbitrary owner", err)
	}
}
