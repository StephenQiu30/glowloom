package canvas_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	canvaswire "github.com/StephenQiu30/glowloom-server/backend/internal/app/canvas"
	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	pgcanvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

func TestAssetCanvasStoreReadsActualDocumentsAndExplicitShortDramas(t *testing.T) {
	database := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, database)
	service := application.NewService(canvasStore(database))
	main, err := service.Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "旧主画布名"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "第二张画布"})
	if err != nil {
		t.Fatal(err)
	}
	dramaKey := uuid.NewString()
	drama, err := service.Create(t.Context(), actor, project, dramaKey, application.CreateInput{Name: "独立短剧", Kind: "short_drama"})
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := service.Create(t.Context(), actor, project, dramaKey, application.CreateInput{Name: "独立短剧", Kind: "short_drama"})
	if err != nil || replayed.ID != drama.ID || replayed.Kind != "short_drama" {
		t.Fatal("short drama replay changed identity", replayed, err)
	}
	if _, err := service.Create(t.Context(), actor, project, dramaKey, application.CreateInput{Name: "独立短剧"}); !errors.Is(err, application.ErrIdempotencyConflict) {
		t.Fatal("changed kind reused durable creation key", err)
	}
	if err := database.Exec(`UPDATE workspace.project SET initial_canvas_id=?,name='当前主画布名' WHERE id=?`, main.ID, project).Error; err != nil {
		t.Fatal(err)
	}
	query := application.NewAssetQuery(pgcanvas.NewAssetStore(database, canvaswire.AssetAuthority))
	page, err := query.List(t.Context(), actor, application.AssetListInput{Sort: "created", Order: "asc"})
	if err != nil || len(page.Items) != 2 {
		t.Fatalf("ordinary inventory must include both actual documents: %+v %v", page, err)
	}
	byID := make(map[uuid.UUID]application.AssetCanvas)
	for _, item := range page.Items {
		byID[item.ID] = item
	}
	if byID[main.ID].Name != "当前主画布名" || !byID[main.ID].IsMain || byID[second.ID].Name != "第二张画布" || byID[second.ID].IsMain {
		t.Fatal("main projection or secondary document lost", page.Items)
	}
	page, err = query.List(t.Context(), actor, application.AssetListInput{Kind: "short_drama"})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != drama.ID || page.Items[0].Kind != "short_drama" {
		t.Fatal(page, err)
	}
	read, err := service.Get(t.Context(), actor, drama.ID)
	if err != nil || application.Snapshot(read).Kind != "short_drama" {
		t.Fatal(read, err)
	}
	other, _ := seedCanvasActor(t, database)
	page, err = query.List(t.Context(), other, application.AssetListInput{})
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("cross-scope documents leaked: %+v %v", page, err)
	}
	if err := database.Exec(`UPDATE identity.user_session SET revoked_at=now() WHERE id=?`, other.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := query.List(t.Context(), other, application.AssetListInput{}); err == nil {
		t.Fatal("empty inventory accepted a revoked session")
	}
}

func TestAssetCanvasStoreUsesHalfOpenDatesAndStableBidirectionalPages(t *testing.T) {
	database := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, database)
	service := application.NewService(canvasStore(database))
	when := time.Date(2026, 10, 28, 8, 0, 0, 0, time.UTC)
	for range 3 {
		doc, err := service.Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "同一时刻"})
		if err != nil {
			t.Fatal(err)
		}
		if err := database.Exec(`UPDATE canvas.document SET create_time=?,update_time=? WHERE id=?`, when, when, doc.ID).Error; err != nil {
			t.Fatal(err)
		}
	}
	query := application.NewAssetQuery(pgcanvas.NewAssetStore(database, canvaswire.AssetAuthority))
	before := when.Add(time.Hour)
	for _, order := range []string{"asc", "desc"} {
		input := application.AssetListInput{Search: "同一", CreatedFrom: &when, CreatedBefore: &before, Sort: "created", Order: order, Limit: 1}
		seen := make(map[uuid.UUID]bool)
		for {
			page, err := query.List(t.Context(), actor, input)
			if err != nil || len(page.Items) != 1 || seen[page.Items[0].ID] {
				t.Fatalf("unstable %s pagination: %+v %v", order, page, err)
			}
			seen[page.Items[0].ID] = true
			if page.Next == nil {
				break
			}
			input.After = page.Next
		}
		if len(seen) != 3 {
			t.Fatalf("missing documents across %s pages: %d", order, len(seen))
		}
	}
	page, err := query.List(t.Context(), actor, application.AssetListInput{CreatedBefore: &when})
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("exclusive upper bound included boundary: %+v %v", page, err)
	}
}

func TestAssetCanvasStoreCopyRetainsShortDramaClassification(t *testing.T) {
	database := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, database)
	_, err := application.NewService(canvasStore(database)).Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "明确短剧", Kind: "short_drama"})
	if err != nil {
		t.Fatal(err)
	}
	binding := application.ProjectCopyBinding{JobID: uuid.New(), OrgID: actor.OrgID, SourceProjectID: project, TargetProjectID: uuid.New()}
	if err := database.Exec(`INSERT INTO workspace.project(id,org_id,owner_id,owner_scope_id,name,aspect_ratio,style_type,status) VALUES(?,?,?,?,'短剧副本','16:9','realistic','copying')`, binding.TargetProjectID, actor.OrgID, actor.ID, actor.ScopeID).Error; err != nil {
		t.Fatal(err)
	}
	var snapshot application.ProjectCopySnapshot
	if err := database.Transaction(func(tx *gorm.DB) error {
		var err error
		snapshot, err = pgcanvas.NewScopedProjectCopyStore(tx, nil, nil, mediawire.NewCanvasProjectAccess).Freeze(t.Context(), actor, binding, map[uuid.UUID]uuid.UUID{})
		if err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO workspace.project_copy_job(id,org_id,actor_id,owner_scope_id,source_project_id,source_revision,target_project_id,target_name,status,stage,manifest,workspace_snapshot,idem_key,request_sha256,admission_response) VALUES(?,?,?,?,?,1,?,'短剧副本','queued','media','{}','{}',gen_random_uuid(),repeat('a',64),'{}')`, binding.JobID, actor.OrgID, actor.ID, actor.ScopeID, project, binding.TargetProjectID).Error
	}); err != nil {
		t.Fatal(err)
	}
	factory := func(tx *gorm.DB, _ application.ProjectCopyBinding) application.MediaReader {
		return mediaapp.NewAssetQuery(pgmedia.NewScopedStore(tx, mediawire.NewMediaScopeAccess), nil)
	}
	for range 2 {
		if err := database.Transaction(func(tx *gorm.DB) error {
			_, err := pgcanvas.NewScopedProjectCopyStore(tx, nil, factory, mediawire.NewCanvasProjectAccess).Copy(t.Context(), actor, binding, snapshot)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	var kind string
	if err := database.Raw(`SELECT kind FROM canvas.document WHERE project_id=?`, binding.TargetProjectID).Scan(&kind).Error; err != nil || kind != "short_drama" {
		t.Fatal("copy or durable replay lost explicit classification", kind, err)
	}
}
