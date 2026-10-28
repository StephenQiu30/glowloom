package operation_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	pgoperation "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/postgres"
	operationapp "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

// This fixture is opt-in and must point at a fresh, agent-owned disposable DB.
// It deliberately runs the production SQL against PostgreSQL, not a SQL driver.
func personalHistoryPostgres(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("GLOWLOOM_TEST_PERSONAL_HISTORY_DB_DSN")
	if dsn == "" {
		t.Skip("set GLOWLOOM_TEST_PERSONAL_HISTORY_DB_DSN to a fresh disposable PostgreSQL database")
	}
	database, err := gorm.Open(gormpostgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	// Abort on existing schemas: never reuse or erase application/test data.
	var existing int64
	if err := database.Raw(`SELECT count(*) FROM information_schema.schemata WHERE schema_name IN ('workspace','operation','catalog','media')`).Scan(&existing).Error; err != nil || existing != 0 {
		t.Fatalf("requires a fresh isolated DB: schemas=%d err=%v", existing, err)
	}
	const ddl = `CREATE SCHEMA workspace;CREATE SCHEMA operation;CREATE SCHEMA catalog;CREATE SCHEMA media;
 CREATE TABLE workspace.project(id uuid PRIMARY KEY,org_id uuid,owner_scope_id uuid,status text DEFAULT 'active',is_delete boolean DEFAULT false);
 CREATE TABLE catalog.model_profile(id uuid PRIMARY KEY,model_key text,display_name text);
 CREATE TABLE catalog.model_profile_version(id uuid PRIMARY KEY,model_profile_id uuid);
 CREATE TABLE operation.operation(id uuid PRIMARY KEY,project_id uuid,batch_id uuid,capability text,mode text DEFAULT 'test',params jsonb DEFAULT '{}'::jsonb,model_profile_version_id uuid,target_type text DEFAULT 'free',target_id uuid,origin text DEFAULT 'canvas',status text DEFAULT 'completed',quote_micros bigint,settled_micros bigint,failure_code text,retryable boolean,create_time timestamptz,update_time timestamptz,started_at timestamptz,finished_at timestamptz,source_context jsonb,is_delete boolean DEFAULT false);
 CREATE TABLE operation.operation_event(id uuid,operation_id uuid,reason text,is_delete boolean DEFAULT false);
 CREATE TABLE operation.operation_input(id uuid,operation_id uuid,seq_no int,role text,text_value text,is_delete boolean DEFAULT false);
 CREATE TABLE operation.operation_output(id uuid PRIMARY KEY,project_id uuid,operation_id uuid,seq_no int,kind text,media_asset_id uuid,json_payload jsonb,moderation_status text,create_time timestamptz,is_delete boolean DEFAULT false);
 CREATE TABLE operation.prompt_submission(operation_id uuid,actor_id uuid,project_id uuid);
 CREATE TABLE operation.quote_request(org_id uuid,actor_id uuid,result jsonb);
 CREATE TABLE media.media_asset(id uuid PRIMARY KEY,project_id uuid,status text,moderation_status text,kind text DEFAULT 'image',width int,height int,duration_ms bigint,is_delete boolean DEFAULT false);`
	if err := database.Exec(ddl).Error; err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../../db/personal_history_incremental.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(string(migration)).Error; err != nil {
		t.Fatal(err)
	}
	return database
}

type historyPostgresAccess struct {
	denied bool
	actor  identityapp.Principal
}

// This SQL fixture declares publication eligibility separately so read-list
// authorization can keep asserting it never acquires content-write locks.
func (a historyPostgresAccess) CanPublishHistory(_ context.Context, actor identityapp.Principal) (bool, error) {
	return !a.denied && actor == a.actor, nil
}

func (a historyPostgresAccess) AuthorizeOperation(_ context.Context, actor identityapp.Principal, _ uuid.UUID, write bool) error {
	if a.denied || write || actor != a.actor {
		return identityapp.ErrForbidden
	}
	return nil
}
func TestPersonalHistoryRealPostgresProvenanceFiltersPagingAndOutputs(t *testing.T) {
	db := personalHistoryPostgres(t)
	actor := identityapp.Principal{ID: uuid.New(), OrgID: uuid.New(), ScopeID: uuid.New(), SessionID: uuid.New(), CredentialEpoch: 1}
	other := uuid.New()
	project, foreignScope, foreignOrg := uuid.New(), uuid.New(), uuid.New()
	for _, p := range []struct{ id, org, scope uuid.UUID }{{project, actor.OrgID, actor.ScopeID}, {foreignScope, actor.OrgID, uuid.New()}, {foreignOrg, uuid.New(), actor.ScopeID}} {
		if err := db.Exec(`INSERT INTO workspace.project(id,org_id,owner_scope_id)VALUES(?,?,?)`, p.id, p.org, p.scope).Error; err != nil {
			t.Fatal(err)
		}
	}
	model, version := uuid.New(), uuid.New()
	if err := db.Exec(`INSERT INTO catalog.model_profile VALUES(?, 'model-key','晨光模型')`, model).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO catalog.model_profile_version VALUES(?,?)`, version, model).Error; err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	ids := []uuid.UUID{uuid.MustParse("00000000-0000-0000-0000-000000000001"), uuid.MustParse("00000000-0000-0000-0000-000000000002"), uuid.MustParse("00000000-0000-0000-0000-000000000003"), uuid.MustParse("00000000-0000-0000-0000-000000000004")}
	type task struct {
		id, project, initiator uuid.UUID
		capability, receipt    string
		at                     time.Time
	}
	tasks := []task{{ids[0], project, actor.ID, "image.generate", "submission", base}, {ids[1], project, actor.ID, "image.upscale", "single", base}, {ids[2], project, actor.ID, "image.edit", "batch", base}, {ids[3], project, actor.ID, "text.structured", "single", base.Add(time.Hour)},
		{uuid.New(), project, other, "image.generate", "single", base}, {uuid.New(), project, actor.ID, "image.generate", "unproven", base},
		{uuid.New(), foreignScope, actor.ID, "image.generate", "single", base}, {uuid.New(), foreignOrg, actor.ID, "image.generate", "single", base},
		{uuid.New(), project, actor.ID, "image.generate", "submission_wrong_project", base}, {uuid.New(), project, actor.ID, "document.generate", "single", base.Add(2 * time.Hour)},
		{uuid.New(), project, actor.ID, "image.generate", "single", base.Add(-time.Microsecond)}, {uuid.New(), project, actor.ID, "image.generate", "single", base.Add(24 * time.Hour)}}
	for _, task := range tasks {
		if err := db.Exec(`INSERT INTO operation.operation(id,project_id,capability,model_profile_version_id,create_time,update_time)VALUES(?,?,?,?,?,?)`, task.id, task.project, task.capability, version, task.at, task.at).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Exec(`INSERT INTO operation.operation_input(id,operation_id,seq_no,role,text_value)VALUES(?,?,0,'prompt','冻结提示词：猫')`, uuid.New(), task.id).Error; err != nil {
			t.Fatal(err)
		}
		switch task.receipt {
		case "submission", "submission_wrong_project":
			p := task.project
			if task.receipt == "submission_wrong_project" {
				p = foreignOrg
			}
			if err := db.Exec(`INSERT INTO operation.prompt_submission VALUES(?,?,?)`, task.id, task.initiator, p).Error; err != nil {
				t.Fatal(err)
			}
		case "single", "batch":
			var result any = map[string]any{"operation_id": task.id}
			if task.receipt == "batch" {
				result = map[string]any{"items": []any{map[string]any{"operation_id": task.id}, map[string]any{"error_code": "invalid"}}}
			}
			body, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			if err := db.Exec(`INSERT INTO operation.quote_request VALUES(?,?,?::jsonb)`, actor.OrgID, task.initiator, string(body)).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := db.Exec(`UPDATE operation.operation SET status=CASE WHEN id=? THEN 'ingesting' ELSE 'failed' END WHERE id IN (?,?)`, ids[1], ids[1], ids[2]).Error; err != nil {
		t.Fatal(err)
	}
	// Duplicate evidence must not duplicate a task; malformed old receipts stay harmless.
	if err := db.Exec(`INSERT INTO operation.quote_request VALUES(?,?,?::jsonb),(?,?,?::jsonb)`, actor.OrgID, actor.ID, `{"operation_id":"`+ids[0].String()+`"}`, actor.OrgID, actor.ID, `{"operation_id":"not-a-uuid","items":false}`).Error; err != nil {
		t.Fatal(err)
	}
	ready, rejected, pending := uuid.New(), uuid.New(), uuid.New()
	for _, a := range []struct {
		id             uuid.UUID
		status, review string
	}{{ready, "ready", "passed"}, {rejected, "ready", "rejected"}, {pending, "processing", "passed"}} {
		if err := db.Exec(`INSERT INTO media.media_asset(id,project_id,status,moderation_status)VALUES(?,?,?,?)`, a.id, project, a.status, a.review).Error; err != nil {
			t.Fatal(err)
		}
	}
	for seq, a := range []uuid.UUID{ready, rejected, pending} {
		if err := db.Exec(`INSERT INTO operation.operation_output(id,project_id,operation_id,seq_no,kind,media_asset_id,moderation_status,create_time)VALUES(?,?,?,?, 'image',?,'passed',?)`, uuid.New(), project, ids[0], seq, a, base).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Exec(`INSERT INTO operation.operation_output(id,project_id,operation_id,seq_no,kind,media_asset_id,moderation_status,create_time)VALUES(?,?,?,3,'image',?,'rejected',?)`, uuid.New(), project, ids[0], ready, base).Error; err != nil {
		t.Fatal(err)
	}
	store := pgoperation.NewScopedStore(db, func(*gorm.DB) operationapp.ProjectAccess { return historyPostgresAccess{actor: actor} }, nil)
	before := base.Add(24 * time.Hour)
	input := operationapp.PersonalHistoryInput{Kind: "image", CreatedFrom: &base, CreatedBefore: &before, Order: "asc", Limit: 1}
	var found []uuid.UUID
	for {
		page, err := store.ListPersonalHistory(t.Context(), actor, input)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Items) != 1 {
			t.Fatalf("page=%+v", page)
		}
		found = append(found, page.Items[0].ID)
		if len(found) > 3 {
			t.Fatal("unstable or leaking pagination")
		}
		if page.Items[0].ID == ids[0] {
			outputs := page.Items[0].Outputs
			if len(outputs) != 4 || outputs[0].MediaAssetID == nil || *outputs[0].MediaAssetID != ready || outputs[1].MediaAssetID != nil || outputs[2].MediaAssetID != nil || outputs[3].MediaAssetID != nil {
				t.Fatalf("review leaked media identity: %+v", outputs)
			}
		}
		if page.Next == nil {
			break
		}
		input.After = page.Next
	}
	if len(found) != 3 || found[0] != ids[0] || found[1] != ids[1] || found[2] != ids[2] {
		t.Fatalf("proven initiation/pagination mismatch: %v", found)
	}
	input.Order, input.After = "desc", nil
	for n := 2; n >= 0; n-- {
		page, err := store.ListPersonalHistory(t.Context(), actor, input)
		if err != nil || len(page.Items) != 1 || page.Items[0].ID != ids[n] {
			t.Fatalf("descending same-time page=%+v err=%v", page, err)
		}
		if n > 0 && page.Next == nil {
			t.Fatal("missing descending cursor")
		}
		if n == 0 && page.Next != nil {
			t.Fatal("unexpected final cursor")
		}
		input.After = page.Next
	}
	document, err := store.ListPersonalHistory(t.Context(), actor, operationapp.PersonalHistoryInput{Kind: "document"})
	if err != nil || len(document.Items) != 2 {
		t.Fatalf("document history=%+v err=%v", document, err)
	}
	for _, search := range []string{"晨光模型", "冻结提示词"} {
		page, err := store.ListPersonalHistory(t.Context(), actor, operationapp.PersonalHistoryInput{Kind: "document", Search: search})
		if err != nil || len(page.Items) != 2 {
			t.Fatalf("search=%s items=%v err=%v", search, page.Items, err)
		}
	}
	reverse, err := store.ListPersonalHistory(t.Context(), actor, operationapp.PersonalHistoryInput{Kind: "image", Capability: "image.edit", CreatedFrom: &base, CreatedBefore: &before, Order: "desc"})
	if err != nil || len(reverse.Items) != 1 || reverse.Items[0].ID != ids[2] {
		t.Fatalf("combined exact capability=%+v err=%v", reverse, err)
	}
	denied := pgoperation.NewScopedStore(db, func(*gorm.DB) operationapp.ProjectAccess { return historyPostgresAccess{actor: actor, denied: true} }, nil)
	if page, err := denied.ListPersonalHistory(t.Context(), actor, operationapp.PersonalHistoryInput{Kind: "image"}); !errors.Is(err, identityapp.ErrForbidden) || len(page.Items) != 0 {
		t.Fatalf("revoked project leaked %+v %v", page, err)
	}
	process, err := store.ListPersonalHistory(t.Context(), actor, operationapp.PersonalHistoryInput{Kind: "image", ProcessTypes: []string{"super_resolution"}, Limit: 1})
	if err != nil || len(process.Items) != 1 || process.Items[0].ID != ids[1] {
		t.Fatalf("explicit process filter %+v %v", process, err)
	}
	noProcess, err := store.ListPersonalHistory(t.Context(), actor, operationapp.PersonalHistoryInput{Kind: "image", ProcessTypes: []string{"frame_interpolation"}})
	if err != nil || len(noProcess.Items) != 0 {
		t.Fatalf("invented process classification %+v %v", noProcess, err)
	}
	verifyHistoryManagement(t, db, actor, project)
	verifyHistoryFinalEligibility(t, db, actor, project)
}

func verifyHistoryManagement(t *testing.T, database *gorm.DB, actor identityapp.Principal, project uuid.UUID) {
	t.Helper()
	store := pgoperation.NewScopedStore(database, func(*gorm.DB) operationapp.ProjectAccess { return historyPostgresAccess{actor: actor} }, nil)
	commander := operationapp.NewPersonalHistoryCommander(store)
	op, empty := uuid.New(), uuid.New()
	for _, id := range []uuid.UUID{op, empty} {
		if err := database.Exec(`INSERT INTO operation.operation(id,project_id,capability,mode,status,params,create_time,update_time)VALUES(?,?,'image.generate','omni_reference','completed','{"resolution":"2K","aspect_ratio":"16:9"}',now(),now())`, id, project).Error; err != nil {
			t.Fatal(err)
		}
		if err := database.Exec(`INSERT INTO operation.quote_request VALUES(?,?,?::jsonb)`, actor.OrgID, actor.ID, `{"operation_id":"`+id.String()+`"}`).Error; err != nil {
			t.Fatal(err)
		}
	}
	outputs := make([]uuid.UUID, 10)
	for n := range outputs {
		outputs[n] = uuid.New()
		asset := uuid.New()
		if err := database.Exec(`INSERT INTO media.media_asset(id,project_id,status,moderation_status,kind,width,height)VALUES(?,?,'ready','passed','image',2048,1152)`, asset, project).Error; err != nil {
			t.Fatal(err)
		}
		if err := database.Exec(`INSERT INTO operation.operation_output(id,project_id,operation_id,seq_no,kind,media_asset_id,moderation_status,create_time)VALUES(?,?,?,?,'media',?,'passed',now())`, outputs[n], project, op, n, asset).Error; err != nil {
			t.Fatal(err)
		}
	}
	page, err := store.ListPersonalHistory(t.Context(), actor, operationapp.PersonalHistoryInput{Kind: "image", GenerationModes: []string{"all_reference"}, Resolutions: []string{"2K"}, Ratios: []string{"16:9"}, Limit: 1})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != empty || page.Next == nil {
		t.Fatalf("pending facts before pagination: %+v %v", page, err)
	}
	// Later creation timestamps put empty first; follow cursor to the 10 candidates.
	page, err = store.ListPersonalHistory(t.Context(), actor, operationapp.PersonalHistoryInput{Kind: "image", GenerationModes: []string{"all_reference"}, Resolutions: []string{"2K"}, Ratios: []string{"16:9"}, Limit: 1, After: page.Next})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != op || len(page.Items[0].Outputs) != 10 || !page.Items[0].Outputs[0].CanPublish {
		t.Fatalf("complete candidates: %+v %v", page, err)
	}
	yes := true
	key := uuid.New()
	favorite := operationapp.PersonalHistoryCommand{Action: "favorite", Favorited: &yes, Targets: []operationapp.PersonalHistoryTarget{{OperationID: op, OutputID: &outputs[0]}, {OperationID: op, OutputID: &outputs[1]}, {OperationID: empty}, {OperationID: uuid.New()}}}
	receipt, err := commander.Apply(t.Context(), actor, key, favorite)
	if err != nil || len(receipt.Results) != 4 || !receipt.Results[0].OK || !receipt.Results[1].OK || receipt.Results[2].Code != "no_results" || receipt.Results[3].Code != "not_found" {
		t.Fatalf("partial results: %+v %v", receipt, err)
	}
	replay, err := commander.Apply(t.Context(), actor, key, favorite)
	before, _ := json.Marshal(receipt)
	after, _ := json.Marshal(replay)
	if err != nil || string(before) != string(after) {
		t.Fatalf("not identical replay %s %s %v", before, after, err)
	}
	changed := favorite
	changed.Action = "delete"
	changed.Favorited = nil
	if _, err := commander.Apply(t.Context(), actor, key, changed); !errors.Is(err, operationapp.ErrHistoryCommandConflict) {
		t.Fatalf("key reuse: %v", err)
	}
	page, err = store.ListPersonalHistory(t.Context(), actor, operationapp.PersonalHistoryInput{FavoriteOnly: true, Ratios: []string{"32:18"}, Limit: 1})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != op || len(page.Items[0].Outputs) != 2 || page.Items[0].HistoryRevision != 2 {
		t.Fatalf("favorites clipping: %+v %v", page, err)
	}
	for _, out := range page.Items[0].Outputs {
		if !out.Favorite || out.MetadataRevision != 1 || out.Width == nil || *out.Width != 2048 || out.Resolution != "2K" || out.Ratio != "16:9" || out.GenerationMode != "all_reference" {
			t.Fatalf("candidate facts %+v", out)
		}
	}
	stale, err := commander.Apply(t.Context(), actor, uuid.New(), operationapp.PersonalHistoryCommand{Action: "delete", Targets: []operationapp.PersonalHistoryTarget{{OperationID: op, OutputID: &outputs[0]}}})
	if err != nil || stale.Results[0].Code != "revision_conflict" {
		t.Fatalf("lost CAS %+v %v", stale, err)
	}
	deleted, err := commander.Apply(t.Context(), actor, uuid.New(), operationapp.PersonalHistoryCommand{Action: "delete", Targets: []operationapp.PersonalHistoryTarget{{OperationID: op, OutputID: &outputs[0], ExpectedRevision: 1}}})
	if err != nil || !deleted.Results[0].OK || deleted.Results[0].Revision != 2 {
		t.Fatalf("result delete %+v %v", deleted, err)
	}
	page, err = store.ListPersonalHistory(t.Context(), actor, operationapp.PersonalHistoryInput{FavoriteOnly: true})
	if err != nil || len(page.Items) != 1 || len(page.Items[0].Outputs) != 1 || page.Items[0].HistoryRevision != 3 {
		t.Fatalf("sibling preserved %+v %v", page, err)
	}
	record, err := commander.Apply(t.Context(), actor, uuid.New(), operationapp.PersonalHistoryCommand{Action: "delete", Targets: []operationapp.PersonalHistoryTarget{{OperationID: op, ExpectedRevision: 3}, {OperationID: empty}}})
	if err != nil || !record.Results[0].OK || !record.Results[1].OK {
		t.Fatalf("whole delete %+v %v", record, err)
	}
	page, err = store.ListPersonalHistory(t.Context(), actor, operationapp.PersonalHistoryInput{GenerationModes: []string{"all_reference"}})
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("removed record reappeared %+v %v", page, err)
	}
	var preserved int64
	if err := database.Raw(`SELECT count(*) FROM operation.operation_output WHERE operation_id=? AND NOT is_delete`, op).Scan(&preserved).Error; err != nil || preserved != 10 {
		t.Fatalf("ledger mutated count=%d err=%v", preserved, err)
	}
	// Exact-key replay retains its old receipt, without repeating metadata writes.
	if _, err := commander.Apply(t.Context(), actor, key, favorite); err != nil {
		t.Fatal(err)
	}
	// Actors, spaces and revoked current authority cannot use retained receipts.
	for _, other := range []identityapp.Principal{{ID: uuid.New(), OrgID: actor.OrgID, ScopeID: actor.ScopeID, SessionID: uuid.New(), CredentialEpoch: 1}, {ID: actor.ID, OrgID: actor.OrgID, ScopeID: uuid.New(), SessionID: actor.SessionID, CredentialEpoch: 1}} {
		otherStore := pgoperation.NewScopedStore(database, func(*gorm.DB) operationapp.ProjectAccess { return historyPostgresAccess{actor: other} }, nil)
		r, e := operationapp.NewPersonalHistoryCommander(otherStore).Apply(t.Context(), other, uuid.New(), operationapp.PersonalHistoryCommand{Action: "delete", Targets: []operationapp.PersonalHistoryTarget{{OperationID: op}}})
		if e != nil || r.Results[0].Code != "not_found" {
			t.Fatalf("cross identity command %+v %v", r, e)
		}
	}
	denied := pgoperation.NewScopedStore(database, func(*gorm.DB) operationapp.ProjectAccess { return historyPostgresAccess{actor: actor, denied: true} }, nil)
	if _, err := operationapp.NewPersonalHistoryCommander(denied).Apply(t.Context(), actor, key, favorite); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatalf("revoked replay %v", err)
	}
	// Concurrent distinct commands with the same expected revision have one winner.
	racing := uuid.New()
	asset := uuid.New()
	candidate := uuid.New()
	if err := database.Exec(`INSERT INTO operation.operation(id,project_id,capability,create_time,update_time)VALUES(?,?,'image.generate',now(),now());`, racing, project).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`INSERT INTO operation.quote_request VALUES(?,?,?::jsonb)`, actor.OrgID, actor.ID, `{"operation_id":"`+racing.String()+`"}`).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`INSERT INTO media.media_asset(id,project_id,status,moderation_status)VALUES(?,?,'ready','passed')`, asset, project).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`INSERT INTO operation.operation_output(id,project_id,operation_id,seq_no,kind,media_asset_id,moderation_status,create_time)VALUES(?,?,?,0,'media',?,'passed',now())`, candidate, project, racing, asset).Error; err != nil {
		t.Fatal(err)
	}
	results := make(chan operationapp.PersonalHistoryCommandReceipt, 2)
	errs := make(chan error, 2)
	for range 2 {
		go func() {
			r, e := commander.Apply(t.Context(), actor, uuid.New(), operationapp.PersonalHistoryCommand{Action: "favorite", Favorited: &yes, Targets: []operationapp.PersonalHistoryTarget{{OperationID: racing, OutputID: &candidate}}})
			results <- r
			errs <- e
		}()
	}
	successes, conflicts := 0, 0
	for range 2 {
		r, e := <-results, <-errs
		if e != nil {
			t.Fatal(e)
		}
		if r.Results[0].OK {
			successes++
		}
		if r.Results[0].Code == "revision_conflict" {
			conflicts++
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent CAS winners=%d conflicts=%d", successes, conflicts)
	}
	no := false
	unfavorite, err := commander.Apply(t.Context(), actor, uuid.New(), operationapp.PersonalHistoryCommand{Action: "favorite", Favorited: &no, Targets: []operationapp.PersonalHistoryTarget{{OperationID: racing, OutputID: &candidate, ExpectedRevision: 1}}})
	if err != nil || !unfavorite.Results[0].OK || unfavorite.Results[0].Revision != 2 {
		t.Fatalf("unfavorite %+v %v", unfavorite, err)
	}
	favorites, err := store.ListPersonalHistory(t.Context(), actor, operationapp.PersonalHistoryInput{FavoriteOnly: true})
	if err != nil || len(favorites.Items) != 0 {
		t.Fatalf("unfavorite remained in filter %+v %v", favorites, err)
	}
}

func verifyHistoryFinalEligibility(t *testing.T, database *gorm.DB, actor identityapp.Principal, project uuid.UUID) {
	t.Helper()
	store := pgoperation.NewScopedStore(database, func(*gorm.DB) operationapp.ProjectAccess { return historyPostgresAccess{actor: actor} }, nil)
	type eligibility struct {
		status, capability, mode, kind, review, assetStatus string
		publish, download                                   bool
	}
	cases := []eligibility{
		{"completed", "image.generate", "text2image", "image", "passed", "ready", true, true},
		{"succeeded", "image.generate", "text2image", "image", "passed", "ready", false, false},
		{"completed", "video.generate", "lip_sync", "video", "passed", "ready", true, true},
		{"completed", "video.dubbing", "lip_sync", "video", "passed", "ready", false, true},
		{"completed", "video.generate", "dubbing", "video", "passed", "ready", false, true},
		{"completed", "video.generate", "video_dubbing", "video", "passed", "ready", false, true},
		{"completed", "audio.generate", "song", "audio", "passed", "ready", false, true},
		{"completed", "image.generate", "text2image", "image", "rejected", "ready", false, false},
		{"completed", "image.generate", "text2image", "image", "passed", "processing", false, false},
	}
	for _, tc := range cases {
		op, out, asset := uuid.New(), uuid.New(), uuid.New()
		if err := database.Exec(`INSERT INTO operation.operation(id,project_id,capability,mode,status,create_time,update_time)VALUES(?,?,?,?,?,now(),now())`, op, project, tc.capability, tc.mode, tc.status).Error; err != nil {
			t.Fatal(err)
		}
		if err := database.Exec(`INSERT INTO operation.quote_request VALUES(?,?,?::jsonb)`, actor.OrgID, actor.ID, `{"operation_id":"`+op.String()+`"}`).Error; err != nil {
			t.Fatal(err)
		}
		if err := database.Exec(`INSERT INTO media.media_asset(id,project_id,status,moderation_status,kind)VALUES(?,?,?,?,?)`, asset, project, tc.assetStatus, tc.review, tc.kind).Error; err != nil {
			t.Fatal(err)
		}
		if err := database.Exec(`INSERT INTO operation.operation_output(id,project_id,operation_id,seq_no,kind,media_asset_id,moderation_status,create_time)VALUES(?,?,?,0,'media',?,'passed',now())`, out, project, op, asset).Error; err != nil {
			t.Fatal(err)
		}
		page, err := store.ListPersonalHistory(t.Context(), actor, operationapp.PersonalHistoryInput{Capability: tc.capability, Limit: 200})
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, item := range page.Items {
			if item.ID != op {
				continue
			}
			found = true
			if len(item.Outputs) != 1 || item.Outputs[0].CanPublish != tc.publish || item.Outputs[0].CanDownload != tc.download {
				t.Fatalf("eligibility %+v output %+v", tc, item.Outputs)
			}
		}
		if !found {
			t.Fatal("missing eligible history task")
		}
	}
	for _, status := range []string{"completed", "succeeded"} {
		op, out := uuid.New(), uuid.New()
		if err := database.Exec(`INSERT INTO operation.operation(id,project_id,capability,mode,status,create_time,update_time)VALUES(?,?,'text.structured','text_to_text',?,now(),now())`, op, project, status).Error; err != nil {
			t.Fatal(err)
		}
		if err := database.Exec(`INSERT INTO operation.quote_request VALUES(?,?,?::jsonb)`, actor.OrgID, actor.ID, `{"operation_id":"`+op.String()+`"}`).Error; err != nil {
			t.Fatal(err)
		}
		if err := database.Exec(`INSERT INTO operation.operation_output(id,project_id,operation_id,seq_no,kind,json_payload,moderation_status,create_time)VALUES(?,?,?,0,'json','{"text":"已审核文本"}','passed',now())`, out, project, op).Error; err != nil {
			t.Fatal(err)
		}
		page, err := store.ListPersonalHistory(t.Context(), actor, operationapp.PersonalHistoryInput{Kind: "document"})
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range page.Items {
			if item.ID == op {
				result := item.Outputs[0]
				if result.CanPublish || result.CanDownload != (status == "completed") || (result.Text != nil) != (status == "completed") {
					t.Fatalf("text gating status=%s result=%+v", status, result)
				}
			}
		}
	}
}
