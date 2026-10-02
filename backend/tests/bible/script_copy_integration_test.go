package bible_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pg "github.com/StephenQiu30/lanverse/backend/internal/bible/adapter/postgres"
	app "github.com/StephenQiu30/lanverse/backend/internal/bible/application"
	"github.com/StephenQiu30/lanverse/backend/internal/bible/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	scriptobjects "github.com/StephenQiu30/lanverse/backend/internal/script/adapter/objects"
	scriptpg "github.com/StephenQiu30/lanverse/backend/internal/script/adapter/postgres"
	scriptapp "github.com/StephenQiu30/lanverse/backend/internal/script/application"
	scriptdomain "github.com/StephenQiu30/lanverse/backend/internal/script/domain"
	workspacepg "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
)

type bibleScriptCopyAccess struct {
	owner *workspacepg.ProjectCopyAccessStore
}

func (a bibleScriptCopyAccess) Authorize(ctx context.Context, actor identityapp.Principal, b scriptapp.ProjectCopyBinding, target bool) error {
	return a.owner.Authorize(ctx, actor, workspaceapp.ProjectCopyBinding{JobID: b.JobID, OrgID: b.OrgID, SourceProjectID: b.SourceProjectID, TargetProjectID: b.TargetProjectID}, target)
}
func scriptCopyOwner(db *gorm.DB, a workspaceapp.ProjectCopyAuthority) *scriptpg.ProjectCopyStore {
	return scriptpg.NewProjectCopyStore(db, func(tx *gorm.DB) scriptapp.ProjectCopyAccess {
		return bibleScriptCopyAccess{workspacepg.NewProjectCopyAccessStore(tx, a)}
	})
}

type bibleCopyScopes struct{ owner *scriptpg.ProjectCopyStore }

type currentBibleScopes struct{ owner *scriptpg.BibleScopes }

func (s currentBibleScopes) ValidateScopes(ctx context.Context, actor identityapp.Principal, project uuid.UUID, scopes []domain.LookScope) error {
	input := make([]scriptapp.BibleLookScope, 0, len(scopes))
	for _, scope := range scopes {
		input = append(input, scriptapp.BibleLookScope{EpisodeID: scope.EpisodeID, SceneKey: scope.SceneKey})
	}
	return s.owner.ValidateLookScopes(ctx, actor, project, input)
}

func (a bibleCopyScopes) RemapLookScopes(ctx context.Context, actor identityapp.Principal, b app.ProjectCopyBinding, scopes []domain.LookScope) ([]app.ScopeMapping, error) {
	input := make([]scriptapp.BibleLookScope, 0, len(scopes))
	for _, s := range scopes {
		input = append(input, scriptapp.BibleLookScope{EpisodeID: s.EpisodeID, SceneKey: s.SceneKey})
	}
	facts, err := a.owner.RemapLookScopes(ctx, actor, scriptapp.ProjectCopyBinding{JobID: b.JobID, OrgID: b.OrgID, SourceProjectID: b.SourceProjectID, TargetProjectID: b.TargetProjectID}, input)
	if err != nil {
		return nil, err
	}
	result := make([]app.ScopeMapping, 0, len(facts))
	for _, f := range facts {
		result = append(result, app.ScopeMapping{Source: domain.LookScope{EpisodeID: f.Source.EpisodeID, SceneKey: f.Source.SceneKey}, Target: domain.LookScope{EpisodeID: f.Target.EpisodeID, SceneKey: f.Target.SceneKey}})
	}
	return result, nil
}

type scriptCopyCharacters struct {
	owner    *pg.ProjectCopyStore
	snapshot app.ProjectCopySnapshot
}

func (a scriptCopyCharacters) FreezeCharacters(ctx context.Context, actor identityapp.Principal, b scriptapp.ProjectCopyBinding, refs []scriptapp.ProjectCopyCharacterReference) ([]scriptapp.ProjectCopyCharacterMapping, error) {
	requested := make([]app.CharacterCopyReference, 0, len(refs))
	for _, ref := range refs {
		requested = append(requested, app.CharacterCopyReference{CharacterID: ref.CharacterID, VersionID: ref.VersionID})
	}
	facts, err := a.owner.ResolveCharacterMappings(ctx, actor, app.ProjectCopyBinding{JobID: b.JobID, OrgID: b.OrgID, SourceProjectID: b.SourceProjectID, TargetProjectID: b.TargetProjectID}, a.snapshot, requested)
	if err != nil {
		return nil, err
	}
	result := make([]scriptapp.ProjectCopyCharacterMapping, 0, len(facts))
	for _, f := range facts {
		result = append(result, scriptapp.ProjectCopyCharacterMapping{Source: scriptapp.ProjectCopyCharacterReference{CharacterID: f.Source.CharacterID, VersionID: f.Source.VersionID}, Target: scriptapp.ProjectCopyCharacterReference{CharacterID: f.Target.CharacterID, VersionID: f.Target.VersionID}})
	}
	return result, nil
}

func TestBibleScriptCopyPGFullPinnedHistoryScopesAndActualPrivateObjects(t *testing.T) {
	db, owner := bibleTestDB(t)
	objects := bibleObjects(t)
	actor, project := bibleActorProject(t, owner)
	sources := scriptpg.NewSourceStoreWithCharacters(db, func(tx *gorm.DB) scriptapp.ProjectAccess { return workspacepg.NewProjectContentAccessStore(tx) }, func(tx *gorm.DB) scriptapp.CharacterReferences {
		return characterBridge{pg.NewReferences(tx, workspacepg.NewProjectContentAccessStore(tx))}
	})
	saved, err := scriptapp.NewSourceService(sources, scriptobjects.NewStorage(objects), time.Now).Write(t.Context(), actor, scriptapp.SourceCommand{ProjectID: project, Key: uuid.New(), RequestID: uuid.New(), Action: "create", RightsConfirmed: true, Sources: []scriptapp.SourceInput{{Kind: "chapter", Title: "历史原文", Status: "draft", Document: scriptdomain.RichDocument{Type: "doc", Content: []scriptdomain.RichDocument{{Type: "paragraph", Content: []scriptdomain.RichDocument{{Type: "text", Text: "王总：你好😀"}}}}}}}})
	if err != nil {
		t.Fatal(err)
	}
	view, err := sources.Episodes(t.Context(), actor, project, saved.VersionID)
	if err != nil {
		t.Fatal(err)
	}
	formal, err := sources.ConfirmSplit(t.Context(), actor, scriptapp.SplitCommand{ProjectID: project, VersionID: saved.VersionID, Key: uuid.New(), RequestID: uuid.New(), ExpectedRevision: 1, ExpectedSplitRevision: 0, CandidateSetID: saved.SplitSetID, Boundaries: view.Candidate.Boundaries}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	bible := app.NewService(bibleStore(db), time.Now)
	character, err := bible.Change(t.Context(), actor, createCharacter(project))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bible.Change(t.Context(), actor, app.Command{ProjectID: project, Kind: domain.KindCharacter, Action: "confirm", EntryID: character.EntryID, ExpectedRevision: 1, Key: uuid.New(), RequestID: uuid.New()}); err != nil {
		t.Fatal(err)
	}
	scene, line := uuid.New(), uuid.New()
	structure, err := sources.SaveStructure(t.Context(), actor, scriptapp.StructureCommand{ProjectID: project, EpisodeID: formal.Episodes[0].ID, Key: uuid.New(), RequestID: uuid.New(), ExpectedRevision: 2, ExpectedEpisodeRevision: 1, Document: scriptdomain.StructureDocument{Scenes: []scriptdomain.StructureScene{{Key: scene, SeqNo: 1, Heading: "客厅", Start: 0, End: 6, Items: []scriptdomain.StructureItem{{Type: "line", Key: line, Kind: "dialogue", Speaker: "王总", CharacterID: &character.EntryID, Content: "你好😀", Start: 0, End: 6}}}}}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sources.ConfirmStructure(t.Context(), actor, scriptapp.StructureCommand{ProjectID: project, EpisodeID: formal.Episodes[0].ID, Key: uuid.New(), RequestID: uuid.New(), ExpectedRevision: 3, ExpectedEpisodeRevision: 2, BaseStructureVersionNo: 1}, time.Now()); err != nil {
		t.Fatal(err)
	}
	withScopes := app.NewService(pg.NewStore(db, pg.Factories{Access: func(tx *gorm.DB) app.ProjectAccess { return workspacepg.NewProjectContentAccessStore(tx) }, Scopes: func(tx *gorm.DB) app.ScriptScopes {
		return currentBibleScopes{scriptpg.NewBibleScopes(tx, workspacepg.NewProjectContentAccessStore(tx))}
	}}), time.Now)
	detail, err := bible.Find(t.Context(), actor, project, domain.KindCharacter, character.EntryID)
	if err != nil {
		t.Fatal(err)
	}
	look := detail.Current.Character.Looks[0].ID
	if _, err := withScopes.Change(t.Context(), actor, app.Command{ProjectID: project, Kind: domain.KindCharacter, Action: "look_update", EntryID: character.EntryID, ExpectedRevision: 2, LookID: &look, Look: &app.LookInput{Name: "正式剧集造型", Default: true, AppliesTo: []domain.LookScope{{EpisodeID: formal.Episodes[0].ID, SceneKey: &scene}}}, Key: uuid.New(), RequestID: uuid.New()}); err != nil {
		t.Fatal("actual owning scene binding", err)
	}
	b, authority := bibleCopyFixture(t, owner, actor, project)
	admission := authority
	admission.Phase = "freeze"
	admission.WorkerID = uuid.Nil
	var bibleSnapshot app.ProjectCopySnapshot
	var scriptSnapshot scriptapp.ProjectCopySnapshot
	sb := scriptapp.ProjectCopyBinding{JobID: b.JobID, OrgID: b.OrgID, SourceProjectID: b.SourceProjectID, TargetProjectID: b.TargetProjectID}
	err = db.WithContext(t.Context()).Transaction(func(tx *gorm.DB) error {
		access := func(tx *gorm.DB) app.ProjectCopyAccess {
			return bibleCopyAccess{workspacepg.NewProjectCopyAccessStore(tx, admission)}
		}
		bowner := pg.NewProjectCopyStore(tx, access, nil, func(tx *gorm.DB) app.ProjectCopyScopes { return bibleCopyScopes{scriptCopyOwner(tx, admission)} })
		var err error
		bibleSnapshot, err = bowner.Freeze(t.Context(), actor, b, nil, time.Now())
		if err != nil {
			return err
		}
		unknown := uuid.New()
		if _, err := bowner.ResolveCharacterMappings(t.Context(), actor, b, bibleSnapshot, []app.CharacterCopyReference{{CharacterID: character.EntryID, VersionID: &unknown}}); !errors.Is(err, app.ErrUnavailable) {
			t.Fatal("unknown pin guessed current", err)
		}
		if _, err := bowner.ResolveCharacterMappings(t.Context(), actor, b, bibleSnapshot, []app.CharacterCopyReference{{CharacterID: uuid.New(), VersionID: &character.VersionID}}); !errors.Is(err, app.ErrUnavailable) {
			t.Fatal("foreign character allowed", err)
		}
		legacy, err := bowner.ResolveCharacterMappings(t.Context(), actor, b, bibleSnapshot, []app.CharacterCopyReference{{CharacterID: character.EntryID}})
		if err != nil || legacy[0].Target.VersionID != nil {
			t.Fatal("legacy nil pin invented", legacy, err)
		}
		old := scriptCopyOwner(tx, admission)
		if _, err := old.Freeze(t.Context(), actor, sb, nil, time.Now()); !errors.Is(err, scriptapp.ErrContextUnavailable) {
			t.Fatal("missing owning mapping guessed UUID", err)
		}
		current := scriptpg.NewProjectCopyStoreWithCharacters(tx, func(tx *gorm.DB) scriptapp.ProjectCopyAccess {
			return bibleScriptCopyAccess{workspacepg.NewProjectCopyAccessStore(tx, admission)}
		}, func(_ *gorm.DB) scriptapp.ProjectCopyCharacters {
			return scriptCopyCharacters{owner: bowner, snapshot: bibleSnapshot}
		})
		scriptSnapshot, err = current.Freeze(t.Context(), actor, sb, nil, time.Now())
		return err
	})
	if err != nil {
		t.Fatal("two owners admission", err)
	}
	if err := app.NewProjectCopy(bibleCopyStore(db, authority), nil).Transfer(t.Context(), actor, b, bibleSnapshot); err != nil {
		t.Fatal(err)
	}
	scriptTransfer := scriptapp.NewProjectCopy(scriptCopyOwner(db, authority), scriptobjects.NewStorage(objects))
	if _, err := scriptTransfer.Transfer(t.Context(), actor, sb, scriptSnapshot); err != nil {
		t.Fatal("actual source-object transfer", err)
	}
	register := authority
	register.Phase = "register"
	if err := db.WithContext(t.Context()).Transaction(func(tx *gorm.DB) error {
		if _, err := bibleCopyStore(tx, register).Register(t.Context(), actor, b, bibleSnapshot); err != nil {
			return err
		}
		_, err := scriptCopyOwner(tx, register).Register(t.Context(), actor, sb, scriptSnapshot)
		return err
	}); err != nil {
		t.Fatal("complete own registration", err)
	}
	var sourceLine, targetLine struct {
		Content, ContentHash            string
		SpanStart, SpanEnd              int
		CharacterID, CharacterVersionID uuid.UUID
	}
	query := `SELECT d.content,d.content_hash,d.span_start,d.span_end,d.character_id,d.character_version_id FROM script.dialogue_line d JOIN script.scene s ON s.id=d.scene_id WHERE s.project_id=?`
	if err := owner.Raw(query, project).Scan(&sourceLine).Error; err != nil {
		t.Fatal(err)
	}
	if err := owner.Raw(query, b.TargetProjectID).Scan(&targetLine).Error; err != nil {
		t.Fatal(err)
	}
	if sourceLine.Content != targetLine.Content || sourceLine.ContentHash != targetLine.ContentHash || sourceLine.SpanStart != targetLine.SpanStart || sourceLine.SpanEnd != targetLine.SpanEnd || sourceLine.CharacterID == targetLine.CharacterID || sourceLine.CharacterVersionID == targetLine.CharacterVersionID || sourceLine.CharacterVersionID != character.VersionID {
		t.Fatal("line history/hash/span or pinned identities lost", sourceLine, targetLine)
	}
	var scope struct{ EpisodeID, SceneKey uuid.UUID }
	if err := owner.Raw(`SELECT (v.applies_to->0->>'episode_id')::uuid AS episode_id,(v.applies_to->0->>'scene_key')::uuid AS scene_key FROM bible.look_version v WHERE v.project_id=? AND (v.applies_to->0->>'episode_id') IS NOT NULL`, b.TargetProjectID).Scan(&scope).Error; err != nil || scope.EpisodeID == formal.Episodes[0].ID || scope.SceneKey == scene || scope.EpisodeID == uuid.Nil {
		t.Fatal("formal historical applicability missing", scope, err)
	}
	var linked int64
	if err := owner.Raw(`SELECT count(*) FROM script.scene s JOIN script.episode_structure st ON st.id=s.episode_structure_id WHERE s.project_id=? AND s.scene_key=? AND st.episode_id=?`, b.TargetProjectID, scope.SceneKey, scope.EpisodeID).Scan(&linked).Error; err != nil || linked != 1 {
		t.Fatal("Bible scope points outside real copied history", linked, err)
	}
	if structure.StructureID == uuid.Nil || scriptSnapshot.Counts.DialogueLines != 1 || scriptSnapshot.Counts.Objects < 3 {
		t.Fatal("incomplete copied fact counts", scriptSnapshot.Counts)
	}
}
