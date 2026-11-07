package workspace_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	workspacewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/workspace"
	auditapp "github.com/StephenQiu30/glowloom-server/backend/internal/audit/application"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	inbox "github.com/StephenQiu30/glowloom-server/backend/internal/infra/inbox/application"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
	workspacehttp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/http"
	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/postgres"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

func styleLibraryService(db *gorm.DB) *app.StyleLibrary {
	return app.NewStyleLibrary(pgworkspace.NewStyleLibraryStore(db, workspaceRequestScope, nil))
}
func styleLibraryRouter(db *gorm.DB, actor identity.Principal) *gin.Engine {
	r := gin.New()
	r.Use(httpapi.Middleware(workspaceTestOrigin))
	g := r.Group("/api")
	g.Use(func(c *gin.Context) { c.Set("principal", actor); c.Next() })
	workspacehttp.NewStyleLibraryHandler(styleLibraryService(db)).Register(g)
	return r
}
func decodeStyleReceipt(t *testing.T, body []byte) app.StyleLibraryChangeResult {
	t.Helper()
	var out app.StyleLibraryChangeResult
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	return out
}
func TestStyleLibraryHTTPDurableCRUDAndPrivateScope(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	actor := insertWorkspaceActor(ctx, t, db, insertWorkspaceOrganization(ctx, t, db))
	router := styleLibraryRouter(db, actor)
	key := uuid.NewString()
	body := []byte(`{"name":"水墨 % 风格","category":"2d","prompt_fragment":"PRIVATE_STYLE_TEXT","reference_asset_ids":[]}`)
	first := projectHTTPRequest(ctx, router, http.MethodPost, "/api/style-library", key, body)
	if first.Code != 201 {
		t.Fatalf("create: %d %s", first.Code, first.Body.String())
	}
	created := decodeStyleReceipt(t, first.Body.Bytes())
	if created.RequestID.String() != key || created.Preset.Revision != 1 || !created.Preset.Owned || !created.Preset.ReferencesAvailable {
		t.Fatalf("invalid receipt: %+v", created)
	}
	replay := projectHTTPRequest(ctx, router, http.MethodPost, "/api/style-library", key, body)
	if replay.Code != 201 || !bytes.Equal(replay.Body.Bytes(), first.Body.Bytes()) {
		t.Fatalf("replay changed: %d %s", replay.Code, replay.Body.String())
	}
	conflict := projectHTTPRequest(ctx, router, http.MethodPost, "/api/style-library", key, bytes.ReplaceAll(body, []byte("PRIVATE_STYLE_TEXT"), []byte("changed")))
	if conflict.Code != 409 {
		t.Fatalf("different body accepted: %d", conflict.Code)
	}
	list := projectHTTPRequest(ctx, router, http.MethodGet, "/api/style-library?mine=true&q=%25&category=2d", "", nil)
	if list.Code != 200 || bytes.Contains(list.Body.Bytes(), []byte("PRIVATE_STYLE_TEXT")) {
		t.Fatalf("list leaked text: %d %s", list.Code, list.Body.String())
	}
	var page workspacehttp.StyleLibraryListResponse
	if json.Unmarshal(list.Body.Bytes(), &page) != nil || len(page.Items) != 1 || page.Items[0].ID != created.Preset.ID {
		t.Fatalf("literal percent search: %s", list.Body.String())
	}
	legacy := projectHTTPRequest(ctx, projectHTTPRouter(db, actor), http.MethodGet, "/api/style-presets", "", nil)
	if legacy.Code != 200 || bytes.Contains(legacy.Body.Bytes(), []byte(created.Preset.ID.String())) {
		t.Fatalf("owned library preset escaped legacy safe list: %d %s", legacy.Code, legacy.Body.String())
	}
	official := insertStylePreset(ctx, t, db, actor, "", false)
	editOfficial := projectHTTPRequest(ctx, router, http.MethodPatch, "/api/style-library/"+official.String(), uuid.NewString(), []byte(`{"expected_revision":1,"name":"edited","category":"2d","prompt_fragment":"no","reference_asset_ids":[]}`))
	if editOfficial.Code != 404 {
		t.Fatalf("official was editable: %d", editOfficial.Code)
	}
	other := insertWorkspaceActor(ctx, t, db, actor.OrgID.String())
	if got := projectHTTPRequest(ctx, styleLibraryRouter(db, other), http.MethodGet, "/api/style-library/"+created.Preset.ID.String(), "", nil); got.Code != 404 {
		t.Fatalf("other personal scope disclosed style: %d %s", got.Code, got.Body.String())
	}
	patch := []byte(`{"expected_revision":1,"name":"新名称","category":"3d","prompt_fragment":"SECOND_PRIVATE","reference_asset_ids":[]}`)
	updated := projectHTTPRequest(ctx, router, http.MethodPatch, "/api/style-library/"+created.Preset.ID.String(), uuid.NewString(), patch)
	if updated.Code != 200 || decodeStyleReceipt(t, updated.Body.Bytes()).Preset.Revision != 2 {
		t.Fatalf("patch: %d %s", updated.Code, updated.Body.String())
	}
	stale := projectHTTPRequest(ctx, router, http.MethodPatch, "/api/style-library/"+created.Preset.ID.String(), uuid.NewString(), patch)
	if stale.Code != 409 || !bytes.Contains(stale.Body.Bytes(), []byte("revision_conflict")) {
		t.Fatalf("stale edit: %d %s", stale.Code, stale.Body.String())
	}
	oldReplay := projectHTTPRequest(ctx, router, http.MethodPost, "/api/style-library", key, body)
	if oldReplay.Code != 201 || !bytes.Equal(oldReplay.Body.Bytes(), first.Body.Bytes()) {
		t.Fatal("later edit replaced first receipt")
	}
	deleted := projectHTTPRequest(ctx, router, http.MethodDelete, "/api/style-library/"+created.Preset.ID.String(), uuid.NewString(), []byte(`{"expected_revision":2}`))
	if deleted.Code != 200 || !decodeStyleReceipt(t, deleted.Body.Bytes()).Deleted {
		t.Fatalf("delete: %d %s", deleted.Code, deleted.Body.String())
	}
	if read := projectHTTPRequest(ctx, router, http.MethodGet, "/api/style-library/"+created.Preset.ID.String(), "", nil); read.Code != 404 {
		t.Fatal("deleted style still visible")
	}
	var receipts, audits int64
	if err := db.Raw(`SELECT count(*) FROM workspace.style_preset_command WHERE actor_id=?`, actor.ID).Scan(&receipts).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw(`SELECT count(*) FROM infra.outbox WHERE topic='glowloom.audit.recorded.v1' AND payload->'actor'->>'id'=?`, actor.ID.String()).Scan(&audits).Error; err != nil {
		t.Fatal(err)
	}
	if receipts != 3 || audits != 3 {
		t.Fatalf("replay/failed writes emitted extra facts: %d %d", receipts, audits)
	}
	var auditRows []struct{ Payload []byte }
	if err := db.Raw(`SELECT payload FROM infra.outbox WHERE topic='glowloom.audit.recorded.v1' AND payload->'actor'->>'id'=?`, actor.ID.String()).Scan(&auditRows).Error; err != nil {
		t.Fatal(err)
	}
	for _, row := range auditRows {
		record := inbox.Record{Topic: "glowloom.audit.recorded.v1", Key: []byte(actor.OrgID.String()), Value: row.Payload}
		if _, err := auditapp.NewRecordedActionParser().Parse(record); err != nil {
			t.Fatalf("actual style audit rejected: %v", err)
		}
		var event map[string]any
		if json.Unmarshal(row.Payload, &event) != nil {
			t.Fatal("invalid audit JSON")
		}
		event["data"].(map[string]any)["after"].(map[string]any)["prompt_fragment"] = "private"
		invalid, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		record.Value = invalid
		if _, err := auditapp.NewRecordedActionParser().Parse(record); !errors.Is(err, auditapp.ErrInvalidEvent) {
			t.Fatal("audit accepted private style text")
		}
	}
	var privateAudit bool
	if err := db.Raw(`SELECT EXISTS(SELECT 1 FROM infra.outbox WHERE payload::text LIKE '%PRIVATE_STYLE_TEXT%' OR payload::text LIKE '%SECOND_PRIVATE%')`).Scan(&privateAudit).Error; err != nil || privateAudit {
		t.Fatalf("private prompt entered audit: %v", err)
	}
}
func TestStyleLibraryConcurrentKeyAndCursorBinding(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	a := insertWorkspaceActor(ctx, t, db, insertWorkspaceOrganization(ctx, t, db))
	s := styleLibraryService(db)
	in := styleLibraryInput()
	const n = 6
	ids := make([]uuid.UUID, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() { out, err := s.Change(ctx, a, in); ids[i] = out.Preset.ID; errs[i] = err })
	}
	wg.Wait()
	for i := range n {
		if errs[i] != nil || ids[i] != ids[0] || ids[i] == uuid.Nil {
			t.Fatalf("concurrent result %d: %s %v", i, ids[i], errs[i])
		}
	}
	second := styleLibraryInput()
	second.Name = "第二风格"
	if _, err := s.Change(ctx, a, second); err != nil {
		t.Fatal(err)
	}
	r := styleLibraryRouter(db, a)
	first := projectHTTPRequest(ctx, r, http.MethodGet, "/api/style-library?limit=1&mine=true", "", nil)
	var page workspacehttp.StyleLibraryListResponse
	if json.Unmarshal(first.Body.Bytes(), &page) != nil || page.NextCursor == nil {
		t.Fatalf("missing cursor: %s", first.Body.String())
	}
	mixed := projectHTTPRequest(ctx, r, http.MethodGet, "/api/style-library?limit=1&mine=false&cursor="+*page.NextCursor, "", nil)
	if mixed.Code != 422 {
		t.Fatalf("cursor accepted changed ownership filter: %d", mixed.Code)
	}
}
func TestStyleLibraryTeamOwnershipAndSessionRecheck(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	owner := insertWorkspaceActor(ctx, t, db, insertWorkspaceOrganization(ctx, t, db))
	member := insertWorkspaceActor(ctx, t, db, owner.OrgID.String())
	owner = workspaceTeamActor(t, db, owner)
	var teamRow struct{ TeamID uuid.UUID }
	if err := db.Raw(`SELECT team_id FROM workspace.creation_scope WHERE id=?`, owner.ScopeID).Scan(&teamRow).Error; err != nil {
		t.Fatal(err)
	}
	lifecycleFixtureSQL(t, db, `INSERT INTO workspace.team_member(team_id,user_id,role,status,joined_at,source) VALUES(?,?,'member','active',now(),'invited')`, teamRow.TeamID, member.ID)
	member.ScopeID = owner.ScopeID
	s := styleLibraryService(db)
	out, err := s.Change(ctx, owner, styleLibraryInput())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Read(ctx, member, out.Preset.ID); !errors.Is(err, app.ErrStylePresetNotFound) {
		t.Fatalf("same-scope peer read owned prompt: %v", err)
	}
	page, err := s.List(ctx, member, app.StyleLibraryFilter{})
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("same-scope peer saw private card: %+v %v", page, err)
	}
	lifecycleFixtureSQL(t, db, `UPDATE workspace.team SET status='archived' WHERE id=?`, teamRow.TeamID)
	readOnly, err := s.Read(ctx, owner, out.Preset.ID)
	if err != nil || readOnly.CanEdit {
		t.Fatalf("archived team advertised editing: %+v %v", readOnly, err)
	}
	if _, err := s.Change(ctx, owner, styleLibraryInput()); err == nil {
		t.Fatal("archived team allowed mutation")
	}
	lifecycleFixtureSQL(t, db, `UPDATE workspace.team SET status='active' WHERE id=?`, teamRow.TeamID)
	legacyBody := []byte(fmt.Sprintf(`{"name":"not allowed","aspect_ratio":"16:9","style_type":"realistic","style_preset_id":"%s"}`, out.Preset.ID))
	blocked := projectHTTPRequest(ctx, projectHTTPRouter(db, member), http.MethodPost, "/api/projects", uuid.NewString(), legacyBody)
	if blocked.Code != 404 {
		t.Fatalf("peer adopted a private library preset through legacy project binding: %d %s", blocked.Code, blocked.Body.String())
	}
	lifecycleFixtureSQL(t, db, `UPDATE workspace.team_member SET status='removed' WHERE team_id=? AND user_id=?`, teamRow.TeamID, owner.ID)
	if _, err := s.Change(ctx, owner, styleLibraryInput()); err == nil {
		t.Fatal("removed owner could write")
	}
	if _, err := s.Read(ctx, owner, out.Preset.ID); err == nil {
		t.Fatal("removed owner could read")
	}
}
func seedPersonalStyleImage(t *testing.T, db *gorm.DB, a identity.Principal) uuid.UUID {
	t.Helper()
	id := uuid.New()
	key := "personal/" + a.OrgID.String() + "/" + a.ID.String() + "/image/2026/11/" + id.String() + ".png"
	// Metadata-only object identity; this test makes no claim of object I/O.
	profile := mediaapp.StorageProfile{ID: "style-fixture-cos", Provider: "cos", Endpoint: "http://127.0.0.1:9171", SigningEndpoint: "http://127.0.0.1:9171", Region: "test-region", Bucket: "style-schema-fixture"}
	if err := pgmedia.NewStorageRoutes(db).RegisterProfiles(t.Context(), []mediaapp.StorageProfile{profile}); err != nil {
		t.Fatal(err)
	}
	location := uuid.New()
	lifecycleFixtureSQL(t, db, `INSERT INTO media.object_location(id,profile_id,bucket,object_key,state) VALUES(?,?,?,?,'accepted')`, location, profile.ID, profile.Bucket, key)
	lifecycleFixtureSQL(t, db, `INSERT INTO media.media_asset(id,personal_org_id,personal_actor_id,kind,origin,status,object_key,file_name,mime_type,byte_size,sha256,width,height,codec,moderation_status,object_location_id) VALUES(?,?,?,'image','upload','ready',?,'style.png','image/png',12,repeat('a',64),2,2,'png','passed',?)`, id, a.OrgID, a.ID, key, location)
	return id
}
func TestStyleLibraryRevokedSessionCannotReplay(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	a := insertWorkspaceActor(ctx, t, db, insertWorkspaceOrganization(ctx, t, db))
	service := styleLibraryService(db)
	in := styleLibraryInput()
	if _, err := service.Change(ctx, a, in); err != nil {
		t.Fatal(err)
	}
	lifecycleFixtureSQL(t, db, `UPDATE identity.user_session SET revoked_at=now() WHERE id=?`, a.SessionID)
	if _, err := service.Change(ctx, a, in); !errors.Is(err, identity.ErrSessionInvalid) {
		t.Fatalf("revoked session replay: %v", err)
	}
}
func TestStyleLibraryActualPersonalReferenceMappingAndRecovery(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	a := insertWorkspaceActor(ctx, t, db, insertWorkspaceOrganization(ctx, t, db))
	s := app.NewStyleLibrary(workspacewire.ProvideStyleLibraryStore(db))
	asset := seedPersonalStyleImage(t, db, a)
	in := styleLibraryInput()
	in.ReferenceAssetIDs = []uuid.UUID{asset}
	out, err := s.Change(ctx, a, in)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Preset.ReferenceItems) != 1 || out.Preset.ReferenceItems[0].AssetID != asset || out.Preset.CoverItemID == nil || out.Preset.ReferenceItems[0].Revision != 1 {
		t.Fatalf("missing canonical virtual item: %+v", out.Preset)
	}
	var used bool
	if err := db.Transaction(func(tx *gorm.DB) error {
		var e error
		used, e = pgworkspace.NewStyleMediaGuard(tx).HasRetainedMediaReference(ctx, asset)
		return e
	}); err != nil || !used {
		t.Fatalf("adopted image not pinned: %v", err)
	}
	lifecycleFixtureSQL(t, db, `UPDATE media.media_asset SET status='failed',failure_reason='fixture unavailable' WHERE id=?`, asset)
	detail, err := s.Read(ctx, a, out.Preset.ID)
	if err != nil || detail.ReferencesAvailable || detail.PromptFragment != in.PromptFragment {
		t.Fatalf("stale image cannot be repaired: %+v %v", detail, err)
	}
	in.Action = "patch"
	in.ID = out.Preset.ID
	in.ExpectedRevision = 1
	in.IdempotencyKey = uuid.New()
	if _, err := s.Change(ctx, a, in); !errors.Is(err, app.ErrStyleLibraryReference) {
		t.Fatalf("stale image was saved: %v", err)
	}
	in.ReferenceAssetIDs = []uuid.UUID{}
	in.IdempotencyKey = uuid.New()
	if _, err := s.Change(ctx, a, in); err != nil {
		t.Fatalf("removing stale image failed: %v", err)
	}
	foreign := insertWorkspaceActor(ctx, t, db, a.OrgID.String())
	in = styleLibraryInput()
	in.ReferenceAssetIDs = []uuid.UUID{seedPersonalStyleImage(t, db, foreign)}
	if _, err := s.Change(ctx, a, in); !errors.Is(err, app.ErrStyleLibraryReference) {
		t.Fatalf("other actor image adopted: %v", err)
	}
}

func TestStyleLibraryRuntimeRoleAndCAS(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	a := insertWorkspaceActor(ctx, t, db, insertWorkspaceOrganization(ctx, t, db))
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE glowloom_app`).Error; err != nil {
			return err
		}
		service := styleLibraryService(tx)
		in := styleLibraryInput()
		out, err := service.Change(ctx, a, in)
		if err != nil {
			return err
		}
		in.Action = "patch"
		in.ID = out.Preset.ID
		in.ExpectedRevision = 1
		in.Name = "修改后"
		in.IdempotencyKey = uuid.New()
		updated, err := service.Change(ctx, a, in)
		if err != nil {
			return err
		}
		if updated.Preset.Revision != 2 {
			t.Fatal("runtime update revision not advanced")
		}
		in.Action = "delete"
		in.Name = ""
		in.PromptFragment = ""
		in.Category = ""
		in.ReferenceAssetIDs = nil
		in.ExpectedRevision = 2
		in.IdempotencyKey = uuid.New()
		_, err = service.Change(ctx, a, in)
		return err
	}); err != nil {
		t.Fatalf("least-privilege app role: %v", err)
	}
	service := styleLibraryService(db)
	out, err := service.Change(ctx, a, styleLibraryInput())
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range 2 {
		wg.Go(func() {
			in := styleLibraryInput()
			in.Action = "patch"
			in.ID = out.Preset.ID
			in.ExpectedRevision = 1
			_, errs[i] = service.Change(ctx, a, in)
		})
	}
	wg.Wait()
	successes, conflicts := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, app.ErrStyleLibraryRevision):
			conflicts++
		default:
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("lost CAS update: %+v", errs)
	}
}
