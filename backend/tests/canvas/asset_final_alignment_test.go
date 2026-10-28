package canvas_test

import (
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	canvaswire "github.com/StephenQiu30/glowloom-server/backend/internal/app/canvas"
	operationwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/operation"
	pgcanvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/adapter/postgres"
	canvasapp "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	operationapp "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

func assetFinalTeam(t *testing.T, db *gorm.DB, actor identity.Principal, project uuid.UUID) (identity.Principal, uuid.UUID) {
	t.Helper()
	team, scope := uuid.New(), uuid.New()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO workspace.team(id,name,created_by)VALUES(?,'资产权限回归',?)`, []any{team, actor.ID}},
		{`INSERT INTO workspace.creation_scope(id,kind,team_id,physical_scope_id)VALUES(?,'team',?,?)`, []any{scope, team, actor.OrgID}},
		{`INSERT INTO workspace.team_member(team_id,user_id,role,status,joined_at,source)VALUES(?,?,'team_admin','active',now(),'created')`, []any{team, actor.ID}},
		{`UPDATE workspace.project SET owner_scope_id=? WHERE id=?`, []any{scope, project}},
	} {
		if err := db.Exec(q.sql, q.args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	actor.ScopeID = scope
	return actor, team
}

func assetFinalOperation(t *testing.T, db *gorm.DB, actor identity.Principal, project uuid.UUID, capability, kind string) (uuid.UUID, []uuid.UUID) {
	t.Helper()
	id := uuid.New()
	if err := db.Exec(`INSERT INTO operation.operation(id,project_id,target_type,capability,mode,input_hash,origin,status,region)VALUES(?,?,'agent_session',?,'test',repeat('a',64),'agent','completed','domestic')`, id, project, capability).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO operation.quote_request(org_id,actor_id,request_id,fingerprint,result)VALUES(?,?,?,decode(repeat('ab',32),'hex'),jsonb_build_object('operation_id',?::text))`, actor.OrgID, actor.ID, uuid.New(), id.String()).Error; err != nil {
		t.Fatal(err)
	}
	outputs := []uuid.UUID{uuid.New(), uuid.New()}
	for i, out := range outputs {
		asset := seedCanvasMedia(t, db, project, kind)
		width, height := 1920, 1080
		if i == 1 {
			width, height = 1280, 720
		}
		if err := db.Exec(`UPDATE media.media_asset SET width=?,height=? WHERE id=?`, width, height, asset).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Exec(`INSERT INTO operation.operation_output(id,project_id,operation_id,seq_no,kind,media_asset_id,moderation_status)VALUES(?,?,?,?,'media',?,'passed')`, out, project, id, i, asset).Error; err != nil {
			t.Fatal(err)
		}
	}
	return id, outputs
}

func TestAssetFinalArchivedPrimaryDeleteKeepsWorkspaceAndWriteGuards(t *testing.T) {
	db := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, db)
	service := canvasapp.NewService(canvasStore(db))
	main, err := service.Create(t.Context(), actor, project, uuid.NewString(), canvasapp.CreateInput{Name: "归档主画布"})
	if err != nil {
		t.Fatal(err)
	}
	secondary, err := service.Create(t.Context(), actor, project, uuid.NewString(), canvasapp.CreateInput{Name: "归档附加画布"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`UPDATE workspace.project SET initial_canvas_id=?,status='archived',archived_at=now(),revision=2 WHERE id=?`, main.ID, project).Error; err != nil {
		t.Fatal(err)
	}
	commands := canvasapp.NewAssetCommands(pgcanvas.NewAssetCommandStore(db, canvaswire.AssetMutationAuthority))
	for _, input := range []canvasapp.AssetCommandInput{
		{Action: "rename", Name: "不允许修改归档内容", Items: []canvasapp.AssetCommandItem{{ID: main.ID, ExpectedRevision: 1, ExpectedProjectRevision: 2}}},
		{Action: "delete", Items: []canvasapp.AssetCommandItem{{ID: secondary.ID, ExpectedRevision: 1}}},
	} {
		result, err := commands.Apply(t.Context(), actor, uuid.New(), input)
		if err != nil || result.Succeeded != 0 || result.Results[0].Code != "project_state_conflict" {
			t.Fatal("archived content write guard widened", result, err)
		}
	}
	input := canvasapp.AssetCommandInput{Action: "delete", Items: []canvasapp.AssetCommandItem{{ID: main.ID, ExpectedRevision: 1, ExpectedProjectRevision: 2}}}
	busy := seedCanvasMedia(t, db, project, "image")
	if err := db.Exec(`UPDATE media.media_asset SET status='processing' WHERE id=?`, busy).Error; err != nil {
		t.Fatal(err)
	}
	result, err := commands.Apply(t.Context(), actor, uuid.New(), input)
	if err != nil || result.Results[0].Code != "project_has_inflight_operations" {
		t.Fatal("archived primary did not reach workspace work guard", result, err)
	}
	if err := db.Exec(`UPDATE media.media_asset SET status='ready' WHERE id=?`, busy).Error; err != nil {
		t.Fatal(err)
	}
	result, err = commands.Apply(t.Context(), actor, uuid.New(), input)
	if err != nil || result.Succeeded != 1 {
		t.Fatal("archived primary cannot recycle project", result, err)
	}
	var deleted bool
	if err := db.Raw(`SELECT is_delete FROM workspace.project WHERE id=?`, project).Scan(&deleted).Error; err != nil || !deleted {
		t.Fatal(deleted, err)
	}
	actor2, project2 := seedCanvasActor(t, db)
	actor2, team := assetFinalTeam(t, db, actor2, project2)
	doc, err := service.Create(t.Context(), actor2, project2, uuid.NewString(), canvasapp.CreateInput{Name: "只读团队主画布"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`UPDATE workspace.project SET initial_canvas_id=? WHERE id=?`, doc.ID, project2).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`UPDATE workspace.team SET governance_blocked=true WHERE id=?`, team).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := commands.Apply(t.Context(), actor2, uuid.New(), canvasapp.AssetCommandInput{Action: "delete", Items: []canvasapp.AssetCommandItem{{ID: doc.ID, ExpectedRevision: 1, ExpectedProjectRevision: 1}}}); err == nil {
		t.Fatal("read-only scope gained project deletion")
	}
	if err := db.Raw(`SELECT is_delete FROM workspace.project WHERE id=?`, project2).Scan(&deleted).Error; err != nil || deleted {
		t.Fatal("read-only project deleted", deleted, err)
	}
}

func TestAssetFinalHistoryPublicationReflectsReadOnlyScope(t *testing.T) {
	db := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, db)
	actor, team := assetFinalTeam(t, db, actor, project)
	op, outputs := assetFinalOperation(t, db, actor, project, "image.generate", "image")
	store := operationwire.ProvideOperationStore(db)
	page, err := store.ListPersonalHistory(t.Context(), actor, operationapp.PersonalHistoryInput{Kind: "image"})
	if err != nil || len(page.Items) != 1 || !page.Items[0].Outputs[0].CanPublish {
		t.Fatal("writable scope lost publication", page, err)
	}
	if err := db.Exec(`UPDATE workspace.team SET governance_blocked=true WHERE id=?`, team).Error; err != nil {
		t.Fatal(err)
	}
	page, err = store.ListPersonalHistory(t.Context(), actor, operationapp.PersonalHistoryInput{Kind: "image"})
	if err != nil || len(page.Items) != 1 || page.Items[0].Outputs[0].CanPublish || !page.Items[0].Outputs[0].CanDownload {
		t.Fatal("read-only scope advertised publication or lost readable history", page, err)
	}
	yes := true
	result, err := operationapp.NewPersonalHistoryCommander(store).Apply(t.Context(), actor, uuid.New(), operationapp.PersonalHistoryCommand{Action: "favorite", Favorited: &yes, Targets: []operationapp.PersonalHistoryTarget{{OperationID: op, OutputID: &outputs[0]}}})
	if err != nil || !result.Results[0].OK {
		t.Fatal("read-only personal favorites broken", result, err)
	}
	if _, err := operationapp.NewHistoryPublisher(store).Publish(t.Context(), actor, uuid.New(), operationapp.HistoryPublicationInput{Title: "不能公开", Targets: []operationapp.HistoryPublicationTarget{{OperationID: op, OutputID: outputs[0]}}}); err == nil {
		t.Fatal("read-only publisher guard relaxed")
	}
}

func TestAssetFinalVideoRecordRetainsSiblingsUnderResultFilters(t *testing.T) {
	db := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, db)
	op, outputs := assetFinalOperation(t, db, actor, project, "video.generate", "video")
	store := operationwire.ProvideOperationStore(db)
	yes := true
	commander := operationapp.NewPersonalHistoryCommander(store)
	favorite := operationapp.PersonalHistoryCommand{Action: "favorite", Favorited: &yes, Targets: []operationapp.PersonalHistoryTarget{{OperationID: op, OutputID: &outputs[0]}}}
	if result, err := commander.Apply(t.Context(), actor, uuid.New(), favorite); err != nil || !result.Results[0].OK {
		t.Fatal(result, err)
	}
	for _, input := range []operationapp.PersonalHistoryInput{{Kind: "video", FavoriteOnly: true}, {Kind: "video", Resolutions: []string{"1080P"}}, {Kind: "video", FavoriteOnly: true, Resolutions: []string{"1080P"}}} {
		page, err := store.ListPersonalHistory(t.Context(), actor, input)
		if err != nil || len(page.Items) != 1 || page.Items[0].ID != op || len(page.Items[0].Outputs) != 2 {
			t.Fatal("record-level video filter clipped sibling", page, err)
		}
	}
	if result, err := commander.Apply(t.Context(), actor, uuid.New(), operationapp.PersonalHistoryCommand{Action: "delete", Targets: []operationapp.PersonalHistoryTarget{{OperationID: op, OutputID: &outputs[1]}}}); err != nil || !result.Results[0].OK {
		t.Fatal(result, err)
	}
	page, err := store.ListPersonalHistory(t.Context(), actor, operationapp.PersonalHistoryInput{Kind: "video", FavoriteOnly: true})
	if err != nil || len(page.Items) != 1 || len(page.Items[0].Outputs) != 1 || page.Items[0].Outputs[0].ID != outputs[0] {
		t.Fatal("hidden video sibling resurrected", page, err)
	}
	image, imageOutputs := assetFinalOperation(t, db, actor, project, "image.generate", "image")
	if result, err := commander.Apply(t.Context(), actor, uuid.New(), operationapp.PersonalHistoryCommand{Action: "favorite", Favorited: &yes, Targets: []operationapp.PersonalHistoryTarget{{OperationID: image, OutputID: &imageOutputs[0]}}}); err != nil || !result.Results[0].OK {
		t.Fatal(result, err)
	}
	page, err = store.ListPersonalHistory(t.Context(), actor, operationapp.PersonalHistoryInput{Kind: "image", FavoriteOnly: true})
	if err != nil || len(page.Items) != 1 || len(page.Items[0].Outputs) != 1 || page.Items[0].Outputs[0].ID != imageOutputs[0] {
		t.Fatal("image result filter no longer clips", page, err)
	}
}
