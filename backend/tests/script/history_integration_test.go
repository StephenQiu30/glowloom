package script_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	app "github.com/StephenQiu30/lanverse/backend/internal/script/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
)

func TestScriptHistoryPGLazyCompleteSourceAndStructureVersions(t *testing.T) {
	db, owner := scriptTestDB(t)
	actor, pid := scriptActorProject(t, owner)
	objects := &sourceObjects{data: make(map[string][]byte)}
	store := scriptStore(db)
	sources := app.NewSourceService(store, objects, time.Now)
	history := app.NewHistoryService(store, sources)
	_, input := sourceCommand()
	input.ProjectID = pid
	input.Sources[0].Document = domain.RichDocument{Type: "doc", Content: []domain.RichDocument{{Type: "paragraph", Content: []domain.RichDocument{{Type: "text", Text: "走进客厅\n你好😀 "}}}}}
	first, err := sources.Write(t.Context(), actor, input)
	if err != nil {
		t.Fatal(err)
	}
	input.Key = uuid.New()
	input.Action = "update"
	input.ExpectedRevision = 1
	input.BaseVersionID = &first.VersionID
	input.LineageID = &first.Mappings[0].LineageID
	input.Sources[0].Document.Content[0].Content[0].Marks = []domain.RichMark{{Type: "bold"}}
	second, err := sources.Write(t.Context(), actor, input)
	if err != nil {
		t.Fatal(err)
	}
	page, err := history.Versions(t.Context(), actor, pid, 0, 1)
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != second.VersionID || page.NextVersionNo == nil {
		t.Fatal("lazy version history", page, err)
	}
	more, err := history.Versions(t.Context(), actor, pid, *page.NextVersionNo, 1)
	if err != nil || len(more.Items) != 1 || more.Items[0].ID != first.VersionID || more.Items[0].ContentHash != page.Items[0].ContentHash || more.Items[0].DocumentSHA256 == page.Items[0].DocumentSHA256 {
		t.Fatal("independent text/rich hashes", more, err)
	}
	old, err := history.SourceHistory(t.Context(), actor, pid, first.Mappings[0].LineageID, 0, 100)
	if err != nil || len(old.Items) != 2 || old.Items[0].Revision != 2 || old.Items[1].Revision != 1 {
		t.Fatal("stable lineage history", old, err)
	}
	detail, err := history.SourceSnapshot(t.Context(), actor, pid, old.Items[1].ID)
	if err != nil || detail.Document.Content[0].Content[0].Marks != nil {
		t.Fatal("immutable rich snapshot", detail, err)
	}
	episodes := app.NewEpisodeService(store, sources, time.Now)
	view, err := episodes.Episodes(t.Context(), actor, pid, second.VersionID)
	if err != nil {
		t.Fatal(err)
	}
	confirm := app.SplitCommand{ProjectID: pid, VersionID: second.VersionID, Key: uuid.New(), RequestID: uuid.New(), ExpectedRevision: 2, ExpectedSplitRevision: 0, CandidateSetID: view.Head.CandidateSetID, Boundaries: view.Candidate.Boundaries}
	formal, err := episodes.ConfirmSplit(t.Context(), actor, confirm)
	if err != nil {
		t.Fatal(err)
	}
	save := app.StructureCommand{ProjectID: pid, EpisodeID: formal.Episodes[0].ID, Key: uuid.New(), RequestID: uuid.New(), ExpectedRevision: 3, ExpectedEpisodeRevision: 1, Document: manualStructure()}
	structure, err := episodes.SaveStructure(t.Context(), actor, save)
	if err != nil {
		t.Fatal(err)
	}
	structures, err := history.Structures(t.Context(), actor, save.EpisodeID, 0, 100)
	if err != nil || len(structures.Items) != 1 || structures.Items[0].ID != structure.StructureID {
		t.Fatal("all manual history", structures, err)
	}
	selected, err := history.Structure(t.Context(), actor, save.EpisodeID, 1)
	if err != nil || len(selected.Document.Scenes[0].Items) != 2 || selected.Document.Scenes[0].Items[0].Type != "action" || selected.Document.Scenes[0].Items[1].Type != "line" {
		t.Fatal("ordered immutable structure", selected, err)
	}
	snippet, err := history.EpisodeText(t.Context(), actor, save.EpisodeID, 5, 8)
	if err != nil || snippet.Text != "你好😀" {
		t.Fatal("unicode scalar snippet", snippet, err)
	}
	if _, err := history.EpisodeText(t.Context(), actor, save.EpisodeID, 0, 999); !errors.Is(err, domain.ErrInvalidSpan) {
		t.Fatal("out of range snippet", err)
	}
	foreign, _ := scriptActorProject(t, owner)
	if _, err := history.Structure(t.Context(), foreign, save.EpisodeID, 1); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("foreign structure leakage", err)
	}
}
