package script_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	platformdb "github.com/StephenQiu30/lanverse/backend/internal/platform/db"
	pg "github.com/StephenQiu30/lanverse/backend/internal/script/adapter/postgres"
	app "github.com/StephenQiu30/lanverse/backend/internal/script/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
	workspacepg "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
)

func scriptTestDB(t *testing.T) (*gorm.DB, *gorm.DB) {
	t.Helper()
	dsn := os.Getenv("LV_TEST_SCRIPT_DB_DSN")
	if dsn == "" {
		t.Skip("set LV_TEST_SCRIPT_DB_DSN to isolated migrated PostgreSQL")
	}
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := parsed.Query()
	q.Set("options", "-c role=lanverse_app")
	parsed.RawQuery = q.Encode()
	open := func(d string) *gorm.DB {
		client, err := platformdb.Open(t.Context(), d, noop.NewTracerProvider())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := client.Close(); err != nil {
				t.Error(err)
			}
		})
		return client.DB
	}
	runtime, owner := open(parsed.String()), open(dsn)
	var role string
	if err := runtime.Raw(`SELECT current_user`).Scan(&role).Error; err != nil || role != "lanverse_app" {
		t.Fatal("test lacks actual nonowner", role, err)
	}
	return runtime, owner
}
func scriptActorProject(t *testing.T, db *gorm.DB) (identityapp.Principal, uuid.UUID) {
	t.Helper()
	actor := identityapp.Principal{ID: uuid.New(), OrgID: uuid.New(), Role: identitydomain.RoleProducer}
	pid := uuid.New()
	for _, r := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO workspace.organization(id,name) VALUES(?,?)`, []any{actor.OrgID, "script-test-" + actor.OrgID.String()}},
		{`INSERT INTO identity."user"(id,org_id,login_name,display_name,password_hash,role,status,must_change_password) VALUES(?,?,?,?,?,'producer','active',false)`, []any{actor.ID, actor.OrgID, "script-" + actor.ID.String(), "脚本测试", "test-only-not-a-credential"}},
		{`INSERT INTO workspace.project(id,org_id,name,description,aspect_ratio,style_type,resolution,status,revision,default_models) VALUES(?,?,'剧本真实测试','','16:9','realistic','1080p','active',1,'{}')`, []any{pid, actor.OrgID}},
	} {
		if err := db.Exec(r.query, r.args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	return actor, pid
}
func scriptStore(db *gorm.DB) *pg.SourceStore {
	return pg.NewSourceStore(db, func(tx *gorm.DB) app.ProjectAccess { return workspacepg.NewProjectContentAccessStore(tx) })
}
func scriptCounts(t *testing.T, db *gorm.DB, pid uuid.UUID) (int64, int64, int64) {
	t.Helper()
	var sources, versions, commands int64
	for _, r := range []struct {
		table string
		n     *int64
	}{{"script.script_source", &sources}, {"script.script_version", &versions}, {"script.command", &commands}} {
		if err := db.Table(r.table).Where("project_id=?", pid).Count(r.n).Error; err != nil {
			t.Fatal(err)
		}
	}
	return sources, versions, commands
}

func TestScriptSourcePGImmutableFormatHistoryReplayAndActualCAS(t *testing.T) {
	db, owner := scriptTestDB(t)
	actor, pid := scriptActorProject(t, owner)
	objects := &sourceObjects{data: make(map[string][]byte)}
	service := app.NewSourceService(scriptStore(db), objects, time.Now)
	base, err := scriptStore(db).LoadBase(t.Context(), actor, pid, nil)
	if err != nil || base.State.Revision != 0 || base.Version != nil {
		t.Fatal("empty GET", base, err)
	}
	if s, v, c := scriptCounts(t, owner, pid); s != 0 || v != 0 || c != 0 {
		t.Fatal("empty GET wrote", s, v, c)
	}
	_, input := sourceCommand()
	input.ProjectID = pid
	first, err := service.Write(t.Context(), actor, input)
	if err != nil || first.ScriptRevision != 1 || first.ProjectRevision != 2 {
		t.Fatal("first save", first, err)
	}
	firstBase, err := scriptStore(db).LoadBase(t.Context(), actor, pid, &first.VersionID)
	if err != nil || len(firstBase.Sources) != 1 {
		t.Fatal(err)
	}
	lineage := first.Mappings[0].LineageID
	var update app.SourceCommand
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, &update); err != nil {
		t.Fatal(err)
	}
	update.Key = uuid.New()
	update.RequestID = uuid.New()
	update.Action = "update"
	update.ExpectedRevision = 1
	update.BaseVersionID = &first.VersionID
	update.LineageID = &lineage
	update.Sources[0].Document.Content[0].Content[0].Marks = []domain.RichMark{{Type: "bold"}}
	second, err := service.Write(t.Context(), actor, update)
	if err != nil || second.ScriptRevision != 2 || second.VersionID == first.VersionID {
		t.Fatal("format save", second, err)
	}
	secondBase, err := scriptStore(db).LoadBase(t.Context(), actor, pid, nil)
	if err != nil || secondBase.Version.ContentHash != firstBase.Version.ContentHash || secondBase.Version.DocumentSHA256 == firstBase.Version.DocumentSHA256 || secondBase.Sources[0].PreviousID == nil || *secondBase.Sources[0].PreviousID != firstBase.Sources[0].ID || secondBase.Sources[0].LineageID != lineage {
		t.Fatal("format/lineage facts", secondBase, err)
	}
	replay, err := app.NewSourceService(scriptStore(db), objects, time.Now).Write(t.Context(), actor, input)
	if err != nil || replay.VersionID != first.VersionID || replay.ScriptRevision != 1 {
		t.Fatal("durable original receipt", replay, err)
	}
	stale := update
	stale.Key = uuid.New()
	if _, err := service.Write(t.Context(), actor, stale); !errors.Is(err, app.ErrConflict) {
		t.Fatal("stale CAS", err)
	}
	if s, v, c := scriptCounts(t, owner, pid); s != 2 || v != 2 || c != 2 {
		t.Fatal("rejected CAS wrote", s, v, c)
	}
	var events int64
	if err := owner.Raw(`SELECT count(*) FROM infra.outbox WHERE partition_key=?`, pid.String()).Scan(&events).Error; err != nil || events != 4 {
		t.Fatal("project plus script audit atomic events", events, err)
	}
	if err := db.Exec(`UPDATE script.script_source SET title='illegal' WHERE project_id=?`, pid).Error; err == nil {
		t.Fatal("runtime immutable source UPDATE allowed")
	}
	if err := db.Exec(`DELETE FROM script.script_version WHERE project_id=?`, pid).Error; err == nil {
		t.Fatal("runtime version DELETE allowed")
	}
}

func TestScriptSourcePGWholeReorderRightsAndRevokedReplay(t *testing.T) {
	db, owner := scriptTestDB(t)
	actor, pid := scriptActorProject(t, owner)
	objects := &sourceObjects{data: make(map[string][]byte)}
	service := app.NewSourceService(scriptStore(db), objects, time.Now)
	_, input := sourceCommand()
	input.ProjectID = pid
	input.Action = "import"
	input.Sources = append(input.Sources, app.SourceInput{Kind: "chapter", Title: "第二章", Status: "ready", Document: domain.RichDocument{Type: "doc", Content: []domain.RichDocument{{Type: "paragraph", Content: []domain.RichDocument{{Type: "text", Text: "中文"}}}}}})
	saved, err := service.Write(t.Context(), actor, input)
	if err != nil {
		t.Fatal(err)
	}
	order := []uuid.UUID{saved.Mappings[1].LineageID, saved.Mappings[0].LineageID}
	reorder := app.SourceCommand{ProjectID: pid, Key: uuid.New(), RequestID: uuid.New(), Action: "reorder", ExpectedRevision: 1, BaseVersionID: &saved.VersionID, Order: order}
	out, err := service.Write(t.Context(), actor, reorder)
	if err != nil || out.ScriptRevision != 2 {
		t.Fatal("complete reorder", out, err)
	}
	reorder.Key = uuid.New()
	reorder.ExpectedRevision = 2
	reorder.BaseVersionID = &out.VersionID
	reorder.Order = order[:1]
	if _, err := service.Write(t.Context(), actor, reorder); err == nil {
		t.Fatal("partial reorder accepted")
	}
	foreign, otherPid := scriptActorProject(t, owner)
	if _, err := scriptStore(db).LoadBase(t.Context(), foreign, pid, nil); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("cross org history", otherPid, err)
	}
	if err := owner.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.Write(context.Background(), actor, input); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatal("revoked original replay", err)
	}
}

func TestScriptSourcePGPendingWriteRejectsParallelIntentBeforeObject(t *testing.T) {
	db, owner := scriptTestDB(t)
	actor, pid := scriptActorProject(t, owner)
	objects := &sourceObjects{data: make(map[string][]byte), uncertain: true}
	_, input := sourceCommand()
	input.ProjectID = pid
	service := app.NewSourceService(scriptStore(db), objects, time.Now)
	if _, err := service.Write(t.Context(), actor, input); !errors.Is(err, app.ErrNeedsReconciliation) {
		t.Fatal(err)
	}
	puts := objects.puts
	another := input
	another.Key = uuid.New()
	another.RequestID = uuid.New()
	objects.uncertain = false
	if _, err := service.Write(t.Context(), actor, another); !errors.Is(err, app.ErrNeedsReconciliation) {
		t.Fatal("parallel pending intent accepted", err)
	}
	if s, v, c := scriptCounts(t, owner, pid); s != 0 || v != 0 || c != 1 || objects.puts != puts {
		t.Fatal("second unknown command or object created", s, v, c, objects.puts, puts)
	}
	if _, err := service.Write(t.Context(), actor, input); err != nil {
		t.Fatal("same original key was blocked", err)
	}
}

func TestScriptSourcePGReorderReusesExactManifestWithoutVersionOrObjectDuplication(t *testing.T) {
	db, owner := scriptTestDB(t)
	actor, pid := scriptActorProject(t, owner)
	objects := &sourceObjects{data: make(map[string][]byte)}
	service := app.NewSourceService(scriptStore(db), objects, time.Now)
	_, input := sourceCommand()
	input.ProjectID = pid
	input.Action = "import"
	input.Sources = append(input.Sources, app.SourceInput{Kind: "chapter", Title: "第二章", Status: "draft", Document: domain.RichDocument{Type: "doc", Content: []domain.RichDocument{{Type: "paragraph", Content: []domain.RichDocument{{Type: "text", Text: "第二章"}}}}}})
	first, err := service.Write(t.Context(), actor, input)
	if err != nil {
		t.Fatal(err)
	}
	original := []uuid.UUID{first.Mappings[0].LineageID, first.Mappings[1].LineageID}
	puts := objects.puts
	order := app.SourceCommand{ProjectID: pid, Key: uuid.New(), RequestID: uuid.New(), Action: "reorder", ExpectedRevision: 1, BaseVersionID: &first.VersionID, Order: original}
	same, err := service.Write(t.Context(), actor, order)
	if err != nil || same.Changed || !same.Duplicate || same.VersionID != first.VersionID || same.ScriptRevision != 1 || same.ProjectRevision != 2 || objects.puts != puts {
		t.Fatal("unchanged complete order duplicated", same, err, objects.puts, puts)
	}
	order.Key = uuid.New()
	order.Order = []uuid.UUID{original[1], original[0]}
	second, err := service.Write(t.Context(), actor, order)
	if err != nil || second.ScriptRevision != 2 || second.Duplicate {
		t.Fatal("new order", second, err)
	}
	puts = objects.puts
	order.Key = uuid.New()
	order.ExpectedRevision = 2
	order.BaseVersionID = &second.VersionID
	order.Order = original
	reused, err := service.Write(t.Context(), actor, order)
	if err != nil || !reused.Changed || !reused.Duplicate || reused.VersionID != first.VersionID || reused.ScriptRevision != 3 || reused.ProjectRevision != 4 || objects.puts != puts {
		t.Fatal("original exact manifest not reused", reused, err, objects.puts, puts)
	}
	input.Key = uuid.New()
	input.ExpectedRevision = 3
	input.BaseVersionID = &first.VersionID
	input.Action = "create"
	input.Sources = input.Sources[:1]
	third, err := service.Write(t.Context(), actor, input)
	if err != nil {
		t.Fatal("chronological version after old reuse", err)
	}
	base, err := scriptStore(db).LoadBase(t.Context(), actor, pid, &third.VersionID)
	if err != nil || base.Version.VersionNo != 3 {
		t.Fatal("chronological version sequence", base, err)
	}
	if s, v, c := scriptCounts(t, owner, pid); s != 3 || v != 3 || c != 5 {
		t.Fatal("manifest uniqueness and permanent all receipts", s, v, c)
	}
}
