package bible_test

import (
	"errors"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	pg "github.com/StephenQiu30/lanverse/backend/internal/bible/adapter/postgres"
	app "github.com/StephenQiu30/lanverse/backend/internal/bible/application"
	"github.com/StephenQiu30/lanverse/backend/internal/bible/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	platformdb "github.com/StephenQiu30/lanverse/backend/internal/platform/db"
	workspacepg "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
)

func bibleTestDB(t *testing.T) (*gorm.DB, *gorm.DB) {
	t.Helper()
	dsn := os.Getenv("LV_TEST_BIBLE_DB_DSN")
	if dsn == "" {
		t.Skip("set LV_TEST_BIBLE_DB_DSN to isolated Bible PostgreSQL")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || parsed.Path != "/lanverse_bible" {
		t.Fatal("requires task-owned lanverse_bible", err)
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
		t.Fatal("missing nonowner", role, err)
	}
	return runtime, owner
}

func bibleActorProject(t *testing.T, owner *gorm.DB) (identityapp.Principal, uuid.UUID) {
	t.Helper()
	actor := identityapp.Principal{ID: uuid.New(), OrgID: uuid.New(), Role: identitydomain.RoleProducer}
	project := uuid.New()
	for _, r := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO workspace.organization(id,name) VALUES(?,?)`, []any{actor.OrgID, "bible-test-" + actor.OrgID.String()}},
		{`INSERT INTO identity."user"(id,org_id,login_name,display_name,password_hash,role,status,must_change_password) VALUES(?,?,?,?,?,'producer','active',false)`, []any{actor.ID, actor.OrgID, "bible-" + actor.ID.String(), "角色测试", "synthetic-test-not-a-credential"}},
		{`INSERT INTO workspace.project(id,org_id,name,description,aspect_ratio,style_type,resolution,status,revision,default_models) VALUES(?,?,'角色测试','','16:9','realistic','1080p','active',1,'{}')`, []any{project, actor.OrgID}},
	} {
		if err := owner.Exec(r.sql, r.args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	return actor, project
}

func bibleStore(db *gorm.DB) *pg.Store {
	return pg.NewStore(db, pg.Factories{Access: func(tx *gorm.DB) app.ProjectAccess { return workspacepg.NewProjectContentAccessStore(tx) }})
}

func createCharacter(project uuid.UUID) app.Command {
	return app.Command{ProjectID: project, Key: uuid.New(), RequestID: uuid.New(), Kind: domain.KindCharacter, Action: "create", Character: &app.CharacterInput{Name: "王总", Aliases: []string{"小王"}, Definition: domain.CharacterDefinition{Role: "主角"}}}
}

func TestBiblePGNoSeedImmutableVersionsPermanentReplayAndCAS(t *testing.T) {
	db, owner := bibleTestDB(t)
	actor, project := bibleActorProject(t, owner)
	service := app.NewService(bibleStore(db), time.Now)
	page, err := service.List(t.Context(), actor, app.ListInput{ProjectID: project, Kind: domain.KindCharacter})
	if err != nil || len(page.Entries) != 0 || page.CurrentActorID != actor.ID {
		t.Fatal("empty read", page, err)
	}
	input := createCharacter(project)
	first, err := service.Change(t.Context(), actor, input)
	if err != nil || first.Revision != 1 || first.VersionNumber != 1 || first.ProjectRevision != 2 {
		t.Fatal("create", first, err)
	}
	detail, err := service.Find(t.Context(), actor, project, domain.KindCharacter, first.EntryID)
	if err != nil || detail.Current.Character.Name != "王总" || len(detail.Current.Character.Looks) != 1 || !detail.Current.Character.Looks[0].Default {
		t.Fatal("detail", detail, err)
	}
	update := app.Command{ProjectID: project, Key: uuid.New(), RequestID: uuid.New(), Kind: domain.KindCharacter, Action: "update", EntryID: first.EntryID, ExpectedRevision: 1, Character: &app.CharacterInput{Name: "王总", Description: "  新设定\n正文  ", Aliases: []string{"小王"}}}
	second, err := service.Change(t.Context(), actor, update)
	if err != nil || second.VersionNumber != 2 || second.VersionID == first.VersionID || second.Revision != 2 {
		t.Fatal("update", second, err)
	}
	old, err := service.Version(t.Context(), actor, project, domain.KindCharacter, first.EntryID, first.VersionID)
	if err != nil || old.Character.Description != "" {
		t.Fatal("history rewritten", old, err)
	}
	replay, err := app.NewService(bibleStore(db), time.Now).Change(t.Context(), actor, input)
	if err != nil || replay != first {
		t.Fatal("permanent receipt", replay, err)
	}
	input.Character.Name = "different"
	if _, err := service.Change(t.Context(), actor, input); !errors.Is(err, app.ErrIdempotencyConflict) {
		t.Fatal("same key changed input", err)
	}
	update.Key = uuid.New()
	if _, err := service.Change(t.Context(), actor, update); !errors.Is(err, app.ErrConflict) {
		t.Fatal("stale", err)
	}
	confirmation := app.Command{ProjectID: project, Key: uuid.New(), RequestID: uuid.New(), Kind: domain.KindCharacter, Action: "confirm", EntryID: first.EntryID, ExpectedRevision: 2}
	confirmed, err := service.Change(t.Context(), actor, confirmation)
	if err != nil || confirmed.Revision != 3 || confirmed.ConfirmedVersionID == nil || *confirmed.ConfirmedVersionID != second.VersionID {
		t.Fatal("confirmation", confirmed, err)
	}
	if err := db.Exec(`UPDATE bible.character_version SET content_sha256=? WHERE id=?`, first.ContentSHA256, first.VersionID).Error; err == nil {
		t.Fatal("nonowner can rewrite immutable version")
	}
	if err := owner.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.Change(t.Context(), actor, confirmation); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatal("replay bypassed live actor", err)
	}
}
