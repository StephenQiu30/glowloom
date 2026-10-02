package bible_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	app "github.com/StephenQiu30/lanverse/backend/internal/bible/application"
	"github.com/StephenQiu30/lanverse/backend/internal/bible/domain"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
)

func TestBiblePGExactlyOneFaultsRollbackContentReceiptAndWorkspace(t *testing.T) {
	for _, table := range []string{"bible.character_version", "bible.look_version", "bible.character_confirmation", "bible.command", "infra.outbox"} {
		t.Run(table, func(t *testing.T) {
			db, owner := bibleTestDB(t)
			actor, project := bibleActorProject(t, owner)
			service := app.NewService(bibleStore(db), time.Now)
			input := createCharacter(project)
			var initial app.Receipt
			var err error
			if table == "bible.character_confirmation" {
				initial, err = service.Change(t.Context(), actor, input)
				if err != nil {
					t.Fatal(err)
				}
				input = app.Command{ProjectID: project, Key: uuid.New(), RequestID: uuid.New(), Kind: domain.KindCharacter, Action: "confirm", EntryID: initial.EntryID, ExpectedRevision: 1}
			}
			name := "bible_null_" + uuid.New().String()[:8]
			if err := owner.Exec("CREATE FUNCTION " + name + "() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NULL; END $$").Error; err != nil {
				t.Fatal(err)
			}
			if err := owner.Exec("CREATE TRIGGER " + name + " BEFORE INSERT ON " + table + " FOR EACH ROW EXECUTE FUNCTION " + name + "()").Error; err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := owner.Exec("DROP TRIGGER " + name + " ON " + table).Error; err != nil {
					t.Error(err)
				}
				if err := owner.Exec("DROP FUNCTION " + name + "()").Error; err != nil {
					t.Error(err)
				}
			})
			if _, err := service.Change(t.Context(), actor, input); !errors.Is(err, app.ErrConflict) && (table != "infra.outbox" || !errors.Is(err, workspaceapp.ErrProjectDependencyUnavailable)) {
				t.Fatal("zero-row accepted", err)
			}
			var revision int64
			if err := owner.Raw(`SELECT revision FROM workspace.project WHERE id=?`, project).Scan(&revision).Error; err != nil {
				t.Fatal(err)
			}
			expected := int64(1)
			if initial.EntryID != uuid.Nil {
				expected = 2
			}
			if revision != expected {
				t.Fatal("partial project revision", revision)
			}
			var commands int64
			if err := owner.Table("bible.command").Where("actor_id=? AND request_id=?", actor.ID, input.Key).Count(&commands).Error; err != nil || commands != 0 {
				t.Fatal("partial receipt", commands, err)
			}
			var heads int64
			if err := owner.Table("bible.character").Where("project_id=?", project).Count(&heads).Error; err != nil {
				t.Fatal(err)
			}
			if initial.EntryID == uuid.Nil && heads != 0 {
				t.Fatal("partial identity", heads)
			}
			if initial.EntryID != uuid.Nil {
				detail, err := service.Find(t.Context(), actor, project, domain.KindCharacter, initial.EntryID)
				if err != nil || detail.Head.Revision != 1 || detail.Head.ConfirmedVersionID != nil {
					t.Fatal("partial confirmation", detail, err)
				}
			}
		})
	}
}

func TestBiblePGConcurrentCASHasOneVersionAndNoLosingReceipts(t *testing.T) {
	db, owner := bibleTestDB(t)
	actor, project := bibleActorProject(t, owner)
	service := app.NewService(bibleStore(db), time.Now)
	first, err := service.Change(t.Context(), actor, createCharacter(project))
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 8)
	var group sync.WaitGroup
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := service.Change(t.Context(), actor, app.Command{ProjectID: project, Key: uuid.New(), RequestID: uuid.New(), Kind: domain.KindCharacter, Action: "update", EntryID: first.EntryID, ExpectedRevision: 1, Character: &app.CharacterInput{Name: "并发修改"}})
			results <- err
		}()
	}
	group.Wait()
	close(results)
	wins := 0
	for err := range results {
		if err == nil {
			wins++
		} else if !errors.Is(err, app.ErrConflict) {
			t.Fatal("unexpected failure", err)
		}
	}
	if wins != 1 {
		t.Fatal("CAS winners", wins)
	}
	var versions, commands int64
	if err := owner.Table("bible.character_version").Where("project_id=?", project).Count(&versions).Error; err != nil {
		t.Fatal(err)
	}
	if err := owner.Table("bible.command").Where("project_id=?", project).Count(&commands).Error; err != nil {
		t.Fatal(err)
	}
	if versions != 2 || commands != 2 {
		t.Fatal("losing write facts", versions, commands)
	}
}

func TestBiblePGLocationPropAndAppearanceStableIdentityHistory(t *testing.T) {
	db, owner := bibleTestDB(t)
	actor, project := bibleActorProject(t, owner)
	service := app.NewService(bibleStore(db), time.Now)
	for _, kind := range []domain.Kind{domain.KindLocation, domain.KindProp} {
		c := app.Command{ProjectID: project, Key: uuid.New(), RequestID: uuid.New(), Kind: kind, Action: "create"}
		if kind == domain.KindLocation {
			c.Location = &domain.LocationContent{Name: "庭院", Description: "古朴", Prompt: "晨光"}
		} else {
			c.Prop = &domain.PropContent{Name: "银色戒指", Description: "刻纹", Prompt: "金属"}
		}
		r, err := service.Change(t.Context(), actor, c)
		if err != nil {
			t.Fatal(kind, err)
		}
		if _, err := service.Version(t.Context(), actor, project, kind, r.EntryID, r.VersionID); err != nil {
			t.Fatal(err)
		}
	}
	c := createCharacter(project)
	first, err := service.Change(t.Context(), actor, c)
	if err != nil {
		t.Fatal(err)
	}
	detail, err := service.Find(t.Context(), actor, project, domain.KindCharacter, first.EntryID)
	if err != nil {
		t.Fatal(err)
	}
	originalLook := detail.Current.Character.Looks[0].ID
	create := app.Command{ProjectID: project, Key: uuid.New(), RequestID: uuid.New(), Kind: domain.KindCharacter, Action: "look_create", EntryID: first.EntryID, ExpectedRevision: 1, Look: &app.LookInput{Name: "正式服装", Default: true}}
	second, err := service.Change(t.Context(), actor, create)
	if err != nil {
		t.Fatal(err)
	}
	old, err := service.Version(t.Context(), actor, project, domain.KindCharacter, first.EntryID, first.VersionID)
	if err != nil || !old.Character.Looks[0].Default || old.Character.Looks[0].ID != originalLook {
		t.Fatal("old look changed", old, err)
	}
	current, err := service.Find(t.Context(), actor, project, domain.KindCharacter, first.EntryID)
	if err != nil || len(current.Current.Character.Looks) != 2 || current.Current.Character.Looks[0].ID != originalLook || current.Current.Character.Looks[0].Default {
		t.Fatal("stable looks", current, err)
	}
	missing := app.Command{ProjectID: project, Key: uuid.New(), RequestID: uuid.New(), Kind: domain.KindCharacter, Action: "look_delete", EntryID: first.EntryID, ExpectedRevision: second.Revision, LookID: &originalLook}
	if _, err := service.Change(t.Context(), actor, missing); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("fabricated downstream impact", err)
	}
}
