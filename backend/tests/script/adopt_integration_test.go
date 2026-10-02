package script_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	app "github.com/StephenQiu30/lanverse/backend/internal/script/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
)

func TestScriptAdoptPGFirstFormalPublicationReplayAndMissingOwnerPreservesOldFacts(t *testing.T) {
	db, owner := scriptTestDB(t)
	actor, pid := scriptActorProject(t, owner)
	store := scriptStore(db)
	objects := &sourceObjects{data: make(map[string][]byte)}
	sources := app.NewSourceService(store, objects, time.Now)
	_, input := sourceCommand()
	input.ProjectID = pid
	saved, err := sources.Write(t.Context(), actor, input)
	if err != nil {
		t.Fatal(err)
	}
	view, err := store.Episodes(t.Context(), actor, pid, saved.VersionID)
	if err != nil {
		t.Fatal(err)
	}
	formal, err := store.ConfirmSplit(t.Context(), actor, app.SplitCommand{ProjectID: pid, VersionID: saved.VersionID, Key: uuid.New(), RequestID: uuid.New(), ExpectedRevision: 1, CandidateSetID: saved.SplitSetID, Boundaries: view.Candidate.Boundaries}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	adopt := app.NewAdoptService(store, time.Now)
	command := app.AdoptCommand{ProjectID: pid, VersionID: saved.VersionID, Key: uuid.New(), RequestID: uuid.New(), ExpectedRevision: 2, ExpectedSplitRevision: 1}
	first, err := adopt.Adopt(t.Context(), actor, command)
	if err != nil || !first.Changed || first.ScriptRevision != 3 || first.ProjectRevision != 4 || first.SplitSetID != formal.SplitSetID || first.PreviousVersionID != nil || len(first.Mappings) != 1 || first.Mappings[0].InheritStatus != "not_inherited" {
		t.Fatal("first real adopted content", first, err)
	}
	repeat, err := app.NewAdoptService(scriptStore(db), time.Now).Adopt(t.Context(), actor, command)
	if err != nil || repeat.VersionID != first.VersionID || repeat.ScriptRevision != first.ScriptRevision {
		t.Fatal("permanent original adopt", repeat, err)
	}
	command.Key = uuid.New()
	command.ExpectedRevision = 3
	same, err := adopt.Adopt(t.Context(), actor, command)
	if err != nil || same.Changed || !same.Duplicate || same.ScriptRevision != 3 {
		t.Fatal("same adopted version touch", same, err)
	}
	lineage := saved.Mappings[0].LineageID
	input.Key = uuid.New()
	input.RequestID = uuid.New()
	input.Action = "update"
	input.ExpectedRevision = 3
	input.LineageID = &lineage
	input.BaseVersionID = &saved.VersionID
	input.Sources[0].Document = domain.RichDocument{Type: "doc", Content: []domain.RichDocument{{Type: "paragraph", Content: []domain.RichDocument{{Type: "text", Text: "新的正文"}}}}}
	second, err := sources.Write(t.Context(), actor, input)
	if err != nil {
		t.Fatal(err)
	}
	newView, err := store.Episodes(t.Context(), actor, pid, second.VersionID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ConfirmSplit(t.Context(), actor, app.SplitCommand{ProjectID: pid, VersionID: second.VersionID, Key: uuid.New(), RequestID: uuid.New(), ExpectedRevision: 4, CandidateSetID: second.SplitSetID, Boundaries: newView.Candidate.Boundaries}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	command.Key = uuid.New()
	command.VersionID = second.VersionID
	command.ExpectedRevision = 5
	command.AckInvalidate = true
	if _, err := adopt.Adopt(t.Context(), actor, command); !errors.Is(err, app.ErrContextUnavailable) {
		t.Fatal("unproved downstream guessed empty", err)
	}
	current, err := sources.Workspace(t.Context(), actor, pid)
	if err != nil || current.State.Revision != 5 || current.State.AdoptedVersionID == nil || *current.State.AdoptedVersionID != saved.VersionID {
		t.Fatal("old adopted facts changed", current, err)
	}
	var count int64
	if err := owner.Raw(`SELECT count(*) FROM script.review_command WHERE project_id=? AND action='adopt'`, pid).Scan(&count).Error; err != nil || count != 2 {
		t.Fatal("missing owner wrote receipt", count, err)
	}
}
