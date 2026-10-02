package bible_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pg "github.com/StephenQiu30/lanverse/backend/internal/bible/adapter/postgres"
	app "github.com/StephenQiu30/lanverse/backend/internal/bible/application"
	"github.com/StephenQiu30/lanverse/backend/internal/bible/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstorage"
	scriptobjects "github.com/StephenQiu30/lanverse/backend/internal/script/adapter/objects"
	scriptpg "github.com/StephenQiu30/lanverse/backend/internal/script/adapter/postgres"
	scriptapp "github.com/StephenQiu30/lanverse/backend/internal/script/application"
	scriptdomain "github.com/StephenQiu30/lanverse/backend/internal/script/domain"
	workspacepg "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
)

func bibleObjects(t *testing.T) *objectstorage.Client {
	t.Helper()
	path := os.Getenv("LV_TEST_RECEIPT_STORAGE_CONFIG")
	if path == "" {
		t.Skip("set synthetic private storage config for real source bytes")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("read synthetic test object configuration")
	}
	var cfg struct {
		Endpoint, Bucket, Region string
		AccessKey                string `json:"access_key"`
		SecretKey                string `json:"secret_key"`
	}
	if json.Unmarshal(b, &cfg) != nil {
		t.Fatal("decode synthetic test object configuration")
	}
	storage, err := objectstorage.Open(cfg.Endpoint, cfg.Bucket, cfg.AccessKey, cfg.SecretKey, cfg.Region)
	if err != nil {
		t.Fatal("open synthetic test storage")
	}
	return storage
}

type characterBridge struct{ references *pg.References }

func (b characterBridge) Reference(ctx context.Context, a identityapp.Principal, p, id uuid.UUID, v *uuid.UUID) (scriptapp.CharacterReference, error) {
	fact, err := b.references.Reference(ctx, a, p, id, v)
	return scriptapp.CharacterReference{CharacterID: fact.CharacterID, VersionID: fact.VersionID, Revision: fact.Revision}, err
}

func characterScriptStore(db *gorm.DB) *scriptpg.SourceStore {
	return scriptpg.NewSourceStoreWithCharacters(db, func(tx *gorm.DB) scriptapp.ProjectAccess { return workspacepg.NewProjectContentAccessStore(tx) }, func(tx *gorm.DB) scriptapp.CharacterReferences {
		return characterBridge{pg.NewReferences(tx, workspacepg.NewProjectContentAccessStore(tx))}
	})
}

func TestBiblePGScriptImmutableBindingDoesNotMutateRequestAndPinnedHistory(t *testing.T) {
	db, owner := bibleTestDB(t)
	actor, project := bibleActorProject(t, owner)
	objects := scriptobjects.NewStorage(bibleObjects(t))
	store := characterScriptStore(db)
	sources := scriptapp.NewSourceService(store, objects, time.Now)
	saved, err := sources.Write(t.Context(), actor, scriptapp.SourceCommand{ProjectID: project, Key: uuid.New(), RequestID: uuid.New(), Action: "create", RightsConfirmed: true, Sources: []scriptapp.SourceInput{{Kind: "chapter", Title: "第一章", Status: "draft", Document: scriptdomain.RichDocument{Type: "doc", Content: []scriptdomain.RichDocument{{Type: "paragraph", Content: []scriptdomain.RichDocument{{Type: "text", Text: "王总：你好"}}}}}}}})
	if err != nil {
		t.Fatal(err)
	}
	view, err := store.Episodes(t.Context(), actor, project, saved.VersionID)
	if err != nil {
		t.Fatal(err)
	}
	formal, err := store.ConfirmSplit(t.Context(), actor, scriptapp.SplitCommand{ProjectID: project, VersionID: saved.VersionID, Key: uuid.New(), RequestID: uuid.New(), ExpectedRevision: 1, ExpectedSplitRevision: 0, CandidateSetID: saved.SplitSetID, Boundaries: view.Candidate.Boundaries}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	bible := app.NewService(bibleStore(db), time.Now)
	character, err := bible.Change(t.Context(), actor, createCharacter(project))
	if err != nil {
		t.Fatal(err)
	}
	confirmation := app.Command{ProjectID: project, Key: uuid.New(), RequestID: uuid.New(), Kind: domain.KindCharacter, Action: "confirm", EntryID: character.EntryID, ExpectedRevision: 1}
	if _, err := bible.Change(t.Context(), actor, confirmation); err != nil {
		t.Fatal(err)
	}
	sceneKey, lineKey := uuid.New(), uuid.New()
	input := scriptapp.StructureCommand{ProjectID: project, EpisodeID: formal.Episodes[0].ID, Key: uuid.New(), RequestID: uuid.New(), ExpectedRevision: 2, ExpectedEpisodeRevision: 1, Document: scriptdomain.StructureDocument{Scenes: []scriptdomain.StructureScene{{Key: sceneKey, SeqNo: 1, Heading: "客厅", Start: 0, End: 5, Items: []scriptdomain.StructureItem{{Type: "line", Key: lineKey, Kind: "dialogue", Speaker: "王总", CharacterID: &character.EntryID, Content: "你好", Start: 0, End: 5}}}}}}
	legacy := scriptpg.NewSourceStore(db, func(tx *gorm.DB) scriptapp.ProjectAccess { return workspacepg.NewProjectContentAccessStore(tx) })
	if _, err := legacy.SaveStructure(t.Context(), actor, input, time.Now()); !errors.Is(err, scriptapp.ErrContextUnavailable) {
		t.Fatal("missing Bible owner guessed success", err)
	}
	first, err := store.SaveStructure(t.Context(), actor, input, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if input.Document.Scenes[0].Items[0].CharacterVersionID != nil {
		t.Fatal("owner silently mutated original request/idempotency body")
	}
	replay, err := store.SaveStructure(t.Context(), actor, input, time.Now())
	if err != nil || replay.StructureID != first.StructureID {
		t.Fatal("original request cannot replay", replay, err)
	}
	var original struct {
		CharacterID, CharacterVersionID uuid.UUID
		Content, ContentHash            string
		SpanStart, SpanEnd              int
	}
	if err := owner.Raw(`SELECT d.character_id,d.character_version_id,d.content,d.content_hash,d.span_start,d.span_end FROM script.dialogue_line d JOIN script.scene s ON s.id=d.scene_id WHERE s.episode_structure_id=?`, first.StructureID).Scan(&original).Error; err != nil || original.CharacterID != character.EntryID || original.CharacterVersionID != character.VersionID || original.Content != "你好" || original.SpanStart != 0 || original.SpanEnd != 5 {
		t.Fatal("actual immutable binding", original, err)
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		return scriptpg.NewBibleScopes(tx, workspacepg.NewProjectContentAccessStore(tx)).ValidateLookScopes(t.Context(), actor, project, []scriptapp.BibleLookScope{{EpisodeID: formal.Episodes[0].ID, SceneKey: &sceneKey}})
	})
	if !errors.Is(err, scriptapp.ErrNotFound) {
		t.Fatal("candidate scene counted confirmed", err)
	}
	confirm := input
	confirm.Key = uuid.New()
	confirm.ExpectedRevision = 3
	confirm.ExpectedEpisodeRevision = 2
	confirm.BaseStructureVersionNo = 1
	confirm.Document = scriptdomain.StructureDocument{}
	if _, err := store.ConfirmStructure(t.Context(), actor, confirm, time.Now()); err != nil {
		t.Fatal(err)
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		return scriptpg.NewBibleScopes(tx, workspacepg.NewProjectContentAccessStore(tx)).ValidateLookScopes(t.Context(), actor, project, []scriptapp.BibleLookScope{{EpisodeID: formal.Episodes[0].ID, SceneKey: &sceneKey}})
	})
	if err != nil {
		t.Fatal("confirmed real scope", err)
	}
	changed, err := bible.Change(t.Context(), actor, app.Command{ProjectID: project, Key: uuid.New(), RequestID: uuid.New(), Kind: domain.KindCharacter, Action: "update", EntryID: character.EntryID, ExpectedRevision: 2, Character: &app.CharacterInput{Name: "新王总"}})
	if err != nil {
		t.Fatal(err)
	}
	confirmation.Key = uuid.New()
	confirmation.ExpectedRevision = 3
	if _, err := bible.Change(t.Context(), actor, confirmation); err != nil {
		t.Fatal(err)
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		fact, err := pg.NewReferences(tx, workspacepg.NewProjectContentAccessStore(tx)).Reference(t.Context(), actor, project, character.EntryID, &character.VersionID)
		if err != nil {
			return err
		}
		if fact.VersionID != character.VersionID || fact.VersionID == changed.VersionID {
			t.Fatal("pinned silently refreshed", fact)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var retained struct{ CharacterVersionID uuid.UUID }
	if err := owner.Raw(`SELECT d.character_version_id FROM script.dialogue_line d JOIN script.scene s ON s.id=d.scene_id WHERE s.episode_structure_id=?`, first.StructureID).Scan(&retained).Error; err != nil || retained.CharacterVersionID != character.VersionID {
		t.Fatal("old dialogue rewritten", retained, err)
	}
	if err := db.Exec(`UPDATE script.dialogue_line SET character_version_id=? WHERE character_id=?`, changed.VersionID, character.EntryID).Error; err == nil {
		t.Fatal("runtime can rewrite old assignments")
	}
}
