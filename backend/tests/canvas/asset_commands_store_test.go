package canvas_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"

	"github.com/google/uuid"

	canvaswire "github.com/StephenQiu30/glowloom-server/backend/internal/app/canvas"
	pgcanvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
)

func TestCanvasAssetPersonalFavoritesCASPartialReplayAndCollection(t *testing.T) {
	db := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, db)
	service := application.NewService(canvasStore(db))
	current, err := service.Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "当前"})
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := service.Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "有明确来源的旧画布"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`UPDATE canvas.document SET asset_collection='legacy',operation_version=1 WHERE id=?`, legacy.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`UPDATE canvas.document SET operation_version=0 WHERE id=?`, current.ID).Error; err != nil {
		t.Fatal(err)
	}
	commands := application.NewAssetCommands(pgcanvas.NewAssetCommandStore(db, canvaswire.AssetMutationAuthority))
	yes, no := true, false
	key := uuid.New()
	input := application.AssetCommandInput{Action: "favorite", Favorite: &yes, Items: []application.AssetCommandItem{{ID: current.ID}, {ID: uuid.New()}, {ID: legacy.ID}}}
	result, err := commands.Apply(t.Context(), actor, key, input)
	if err != nil || result.Succeeded != 2 || result.Failed != 1 || result.Results[1].Code != "not_found" {
		t.Fatal(result, err)
	}
	replayed, err := commands.Apply(t.Context(), actor, key, input)
	if err != nil || !reflect.DeepEqual(replayed, result) {
		t.Fatal("unstable batch replay", replayed, err)
	}
	input.Favorite = &no
	if _, err := commands.Apply(t.Context(), actor, key, input); !errors.Is(err, application.ErrIdempotencyConflict) {
		t.Fatal("key accepted different body", err)
	}
	stale, err := commands.Apply(t.Context(), actor, uuid.New(), application.AssetCommandInput{Action: "favorite", Favorite: &no, Items: []application.AssetCommandItem{{ID: current.ID}}})
	if err != nil || stale.Results[0].Code != "favorite_revision_conflict" {
		t.Fatal(stale, err)
	}
	query := application.NewAssetQuery(pgcanvas.NewAssetStore(db, canvaswire.AssetAuthority))
	for _, collection := range []string{"current", "legacy"} {
		page, err := query.List(t.Context(), actor, application.AssetListInput{Collection: collection, FavoriteOnly: true})
		if err != nil || len(page.Items) != 1 || !page.Items[0].Favorite || page.Items[0].FavoriteRevision != 1 || page.Items[0].Revision != 1 || page.Items[0].CanCopyProject != (collection == "current") {
			t.Fatal("source classification guessed from protocol or favorite changed graph", page, err)
		}
	}
	unfavored, err := commands.Apply(t.Context(), actor, uuid.New(), application.AssetCommandInput{Action: "favorite", Favorite: &no, Items: []application.AssetCommandItem{{ID: current.ID, ExpectedFavoriteRevision: 1}}})
	if err != nil || unfavored.Results[0].Favorite || unfavored.Results[0].FavoriteRevision != 2 {
		t.Fatal(unfavored, err)
	}
	other, _ := seedCanvasActor(t, db)
	denied, err := commands.Apply(t.Context(), other, uuid.New(), application.AssetCommandInput{Action: "favorite", Favorite: &yes, Items: []application.AssetCommandItem{{ID: legacy.ID}}})
	if err != nil || denied.Results[0].Code != "not_found" {
		t.Fatal("cross-scope preference mutated", denied, err)
	}
}

func TestCanvasAssetMutationsKeepMainProjectProtectionAndPartialDelete(t *testing.T) {
	db := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, db)
	service := application.NewService(canvasStore(db))
	main, err := service.Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "主画布"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "普通画布"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`UPDATE workspace.project SET initial_canvas_id=? WHERE id=?`, main.ID, project).Error; err != nil {
		t.Fatal(err)
	}
	commands := application.NewAssetCommands(pgcanvas.NewAssetCommandStore(db, canvaswire.AssetMutationAuthority))
	rename := application.AssetCommandInput{Action: "rename", Name: "同步项目名称", Items: []application.AssetCommandItem{{ID: main.ID, ExpectedRevision: 1}}}
	refused, err := commands.Apply(t.Context(), actor, uuid.New(), rename)
	if err != nil || refused.Results[0].Code != "project_revision_required" {
		t.Fatal("main CAS bypassed", refused, err)
	}
	rename.Items[0].ExpectedProjectRevision = 1
	renamed, err := commands.Apply(t.Context(), actor, uuid.New(), rename)
	if err != nil || renamed.Succeeded != 1 || renamed.Results[0].ProjectRevision != 2 {
		t.Fatal(renamed, err)
	}
	var projectName string
	if err := db.Raw(`SELECT name FROM workspace.project WHERE id=?`, project).Scan(&projectName).Error; err != nil || projectName != rename.Name {
		t.Fatal(projectName, err)
	}
	if _, err := service.Delete(t.Context(), actor, main.ID, uuid.NewString(), application.DeleteInput{ExpectedRevision: 1}); !errors.Is(err, application.ErrMainCanvasRequired) {
		t.Fatal("ordinary delete lost main protection", err)
	}
	busy := seedCanvasMedia(t, db, project, "image")
	if err := db.Exec(`UPDATE media.media_asset SET status='processing' WHERE id=?`, busy).Error; err != nil {
		t.Fatal(err)
	}
	blocked, err := commands.Apply(t.Context(), actor, uuid.New(), application.AssetCommandInput{Action: "delete", Items: []application.AssetCommandItem{{ID: main.ID, ExpectedRevision: 1, ExpectedProjectRevision: 2}}})
	if err != nil || blocked.Results[0].Code != "project_has_inflight_operations" {
		t.Fatal("main deletion bypassed owning work guard", blocked, err)
	}
	if err := db.Exec(`UPDATE media.media_asset SET status='ready' WHERE id=?`, busy).Error; err != nil {
		t.Fatal(err)
	}
	bad := uuid.New()
	key := uuid.New()
	input := application.AssetCommandInput{Action: "delete", Items: []application.AssetCommandItem{{ID: main.ID, ExpectedRevision: 1, ExpectedProjectRevision: 2}, {ID: bad, ExpectedRevision: 1}, {ID: second.ID, ExpectedRevision: 1}}}
	result, err := commands.Apply(t.Context(), actor, key, input)
	if err != nil || result.Succeeded != 2 || result.Failed != 1 || result.Results[1].Code != "not_found" {
		t.Fatal(result, err)
	}
	var facts struct{ ProjectDeleted, MainDeleted, SecondDeleted bool }
	if err := db.Raw(`SELECT p.is_delete AS project_deleted,d.is_delete AS main_deleted,s.is_delete AS second_deleted FROM workspace.project p JOIN canvas.document d ON d.id=p.initial_canvas_id JOIN canvas.document s ON s.id=? WHERE p.id=?`, second.ID, project).Scan(&facts).Error; err != nil || !facts.ProjectDeleted || facts.MainDeleted || !facts.SecondDeleted {
		t.Fatal("main project recycle or selected secondary outcome changed", facts, err)
	}
	replayed, err := commands.Apply(t.Context(), actor, key, input)
	if err != nil || !reflect.DeepEqual(replayed, result) {
		t.Fatal("deleted project receipt unavailable", replayed, err)
	}
}

func TestCanvasAssetCoverUsesOnlyCurrentlyReferenceableImage(t *testing.T) {
	db := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, db)
	doc, err := application.NewService(canvasStore(db)).Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "封面"})
	if err != nil {
		t.Fatal(err)
	}
	image := seedCanvasMedia(t, db, project, "image")
	if err := db.Exec(`UPDATE workspace.project SET initial_canvas_id=?,cover_asset_id=? WHERE id=?`, doc.ID, image, project).Error; err != nil {
		t.Fatal(err)
	}
	query := application.NewAssetQuery(pgcanvas.NewAssetStore(db, canvaswire.AssetAuthority))
	page, err := query.List(t.Context(), actor, application.AssetListInput{})
	if err != nil || len(page.Items) != 1 || page.Items[0].CoverAssetID == nil || *page.Items[0].CoverAssetID != image || page.Items[0].CoverUnavailable {
		t.Fatal("real image identity missing", page, err)
	}
	if err := db.Exec(`UPDATE media.media_asset SET moderation_status='rejected',status='rejected' WHERE id=?`, image).Error; err != nil {
		t.Fatal(err)
	}
	page, err = query.List(t.Context(), actor, application.AssetListInput{})
	if err != nil || len(page.Items) != 1 || page.Items[0].CoverAssetID != nil || !page.Items[0].CoverUnavailable {
		t.Fatal("unavailable cover exposed", page, err)
	}
}

func TestCanvasAssetFavoritesIsolateTeamMembersAndSerializeCAS(t *testing.T) {
	db := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, _ := seedCanvasActor(t, db)
	other, _ := seedCanvasActor(t, db)
	team, scope, project := uuid.New(), uuid.New(), uuid.New()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO workspace.team(id,name,created_by) VALUES(?,'收藏隔离',?)`, []any{team, actor.ID}},
		{`INSERT INTO workspace.creation_scope(id,kind,team_id,physical_scope_id) VALUES(?,'team',?,?)`, []any{scope, team, actor.OrgID}},
		{`INSERT INTO workspace.team_member(team_id,user_id,role,status,joined_at,source) VALUES(?,?,'team_admin','active',now(),'created'),(?,?,'member','active',now(),'invited')`, []any{team, actor.ID, team, other.ID}},
		{`INSERT INTO workspace.project(id,org_id,owner_id,owner_scope_id,name,aspect_ratio,style_type) VALUES(?,?,?,?,'收藏隔离','16:9','realistic')`, []any{project, actor.OrgID, actor.ID, scope}},
	} {
		if err := db.Exec(q.sql, q.args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	actor.ScopeID = scope
	other.ScopeID = scope
	other.OrgID = actor.OrgID
	doc, err := application.NewService(canvasStore(db)).Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "团队画布"})
	if err != nil {
		t.Fatal(err)
	}
	commands := application.NewAssetCommands(pgcanvas.NewAssetCommandStore(db, canvaswire.AssetMutationAuthority))
	yes, no := true, false
	input := application.AssetCommandInput{Action: "favorite", Favorite: &yes, Items: []application.AssetCommandItem{{ID: doc.ID}}}
	if result, err := commands.Apply(t.Context(), actor, uuid.New(), input); err != nil || result.Succeeded != 1 {
		t.Fatal(result, err)
	}
	query := application.NewAssetQuery(pgcanvas.NewAssetStore(db, canvaswire.AssetAuthority))
	if page, err := query.List(t.Context(), other, application.AssetListInput{FavoriteOnly: true}); err != nil || len(page.Items) != 0 {
		t.Fatal("another team member inherited favorite", page, err)
	}
	if result, err := commands.Apply(t.Context(), other, uuid.New(), input); err != nil || result.Succeeded != 1 || result.Results[0].FavoriteRevision != 1 {
		t.Fatal("independent preference CAS lost", result, err)
	}
	input.Favorite = &no
	input.Items[0].ExpectedFavoriteRevision = 1
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	type outcome struct {
		receipt application.AssetCommandReceipt
		err     error
	}
	done := make(chan outcome, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Go(func() { result, err := commands.Apply(ctx, actor, uuid.New(), input); done <- outcome{result, err} })
	}
	workers.Wait()
	close(done)
	succeeded, conflicted := 0, 0
	for result := range done {
		if result.err != nil {
			t.Fatal(result.err)
		}
		succeeded += result.receipt.Succeeded
		if result.receipt.Results[0].Code == "favorite_revision_conflict" {
			conflicted++
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatal("concurrent stale favorites both committed", succeeded, conflicted)
	}
	if page, err := query.List(t.Context(), other, application.AssetListInput{FavoriteOnly: true}); err != nil || len(page.Items) != 1 {
		t.Fatal("other member favorite was canceled", page, err)
	}
}

type failAssetAuthority struct {
	application.AssetMutationAccess
	fail uuid.UUID
	err  error
}

func (a failAssetAuthority) AuthorizeAssetProject(ctx context.Context, actor identityapp.Principal, id uuid.UUID, write bool) error {
	if id == a.fail {
		return a.err
	}
	return a.AssetMutationAccess.AuthorizeAssetProject(ctx, actor, id, write)
}
func TestCanvasAssetUnknownDependencyRollsBackAllSuccessfulItems(t *testing.T) {
	db := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, db)
	service := application.NewService(canvasStore(db))
	first, err := service.Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "事务前项"})
	if err != nil {
		t.Fatal(err)
	}
	otherProject := uuid.New()
	if err := db.Exec(`INSERT INTO workspace.project(id,org_id,owner_id,owner_scope_id,name,aspect_ratio,style_type) VALUES(?,?,?,?,'事务后项','16:9','realistic')`, otherProject, actor.OrgID, actor.ID, actor.ScopeID).Error; err != nil {
		t.Fatal(err)
	}
	second, err := service.Create(t.Context(), actor, otherProject, uuid.NewString(), application.CreateInput{Name: "事务后项"})
	if err != nil {
		t.Fatal(err)
	}
	failProject := otherProject
	if first.ID.String() > second.ID.String() {
		failProject = project
	}
	unavailable := errors.New("owned fixture dependency failure")
	commands := application.NewAssetCommands(pgcanvas.NewAssetCommandStore(db, func(tx *gorm.DB) application.AssetMutationAccess {
		return failAssetAuthority{AssetMutationAccess: canvaswire.AssetMutationAuthority(tx), fail: failProject, err: unavailable}
	}))
	yes := true
	key := uuid.New()
	_, err = commands.Apply(t.Context(), actor, key, application.AssetCommandInput{Action: "favorite", Favorite: &yes, Items: []application.AssetCommandItem{{ID: first.ID}, {ID: second.ID}}})
	if !errors.Is(err, unavailable) {
		t.Fatal(err)
	}
	var count int64
	if err := db.Raw(`SELECT count(*) FROM canvas.asset_preference WHERE actor_id=?`, actor.ID).Scan(&count).Error; err != nil || count != 0 {
		t.Fatal("partial writes survived unknown failure", count, err)
	}
	if err := db.Raw(`SELECT count(*) FROM infra.idempotency_record WHERE actor_id=? AND idem_key=?`, actor.ID, key.String()).Scan(&count).Error; err != nil || count != 0 {
		t.Fatal("failed transaction froze a receipt", count, err)
	}
}

func TestCanvasAssetRenamePreservesInitializedCollaborationMetadata(t *testing.T) {
	db := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, db)
	service := application.NewService(canvasStore(db))
	doc, err := service.Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "协同画布"})
	if err != nil {
		t.Fatal(err)
	}
	runtime := yjsRuntime(t)
	initial, err := application.NewYjsService(canvasStore(db), runtime).Initialize(t.Context(), actor, doc.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	commands := application.NewAssetCommands(pgcanvas.NewAssetCommandStore(db, canvaswire.NewAssetMutationAuthority(runtime)))
	result, err := commands.Apply(t.Context(), actor, uuid.New(), application.AssetCommandInput{Action: "rename", Name: "协同新名", Items: []application.AssetCommandItem{{ID: doc.ID, ExpectedRevision: 1}}})
	if err != nil || result.Succeeded != 1 || result.Results[0].Revision != 2 {
		t.Fatal(result, err)
	}
	state, err := canvasStore(db).LoadYjs(t.Context(), actor, doc.ID)
	if err != nil || state.Document.Revision != 2 || state.Document.Name != "协同新名" || state.Epoch != initial.Epoch || state.CommitSeq != initial.CommitSeq+1 || bytes.Equal(state.State, initial.State) {
		t.Fatal("title rename broke collaboration state", state, err)
	}
	noop := []byte{0, 0}
	hash := sha256.Sum256(noop)
	candidate, err := runtime.Prepare(t.Context(), state, application.YjsUpdate{Epoch: state.Epoch, SessionID: actor.SessionID, UpdateID: uuid.New(), Update: noop, PayloadHash: hex.EncodeToString(hash[:])})
	if err != nil || candidate.Projection.Name != "协同新名" || candidate.Effect != "NO_CHANGE" {
		t.Fatal("next edit would restore an old title", candidate, err)
	}
}
