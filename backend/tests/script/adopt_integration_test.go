package script_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	agentwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/agent"
	"github.com/StephenQiu30/glowloom-server/backend/internal/creation/evidence"
	evidencepg "github.com/StephenQiu30/glowloom-server/backend/internal/creation/evidence/adapter/postgres"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/creation/script/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/creation/script/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

func TestScriptAdoptPGFirstFormalPublicationReplayAndMissingOwnerPreservesOldFacts(t *testing.T) {
	db, owner := scriptTestDB(t)
	actor, pid := scriptActorProject(t, owner)
	store := scriptStore(db)
	objects := &sourceObjects{data: make(map[string][]byte)}
	sources := app.NewSourceService(store, objects, time.Now)
	_, input := sourceCommand()
	input.ProjectID = pid
	input.Sources[0].Status = domain.SourceStatusReady
	input.Sources[0].Document.Content[0].Content[0].Text = "清晨渡轮穿过浓雾，码头渐显。"
	saved, err := sources.Write(t.Context(), scriptActor(actor), input)
	if err != nil {
		t.Fatal(err)
	}
	view, err := store.Episodes(t.Context(), scriptActor(actor), pid, saved.VersionID)
	if err != nil {
		t.Fatal(err)
	}
	formal, err := store.ConfirmSplit(t.Context(), scriptActor(actor), app.SplitCommand{ProjectID: pid, VersionID: saved.VersionID, Key: uuid.New(), RequestID: uuid.New(), ExpectedRevision: 1, CandidateSetID: saved.SplitSetID, Boundaries: view.Candidate.Boundaries}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	adopt := app.NewAdoptService(store, time.Now)
	command := app.AdoptCommand{ProjectID: pid, VersionID: saved.VersionID, Key: uuid.New(), RequestID: uuid.New(), ExpectedRevision: 2, ExpectedSplitRevision: 1}
	first, err := adopt.Adopt(t.Context(), scriptActor(actor), command)
	if err != nil || !first.Changed || first.ScriptRevision != 3 || first.ProjectRevision != 4 || first.SplitSetID != formal.SplitSetID || first.PreviousVersionID != nil || len(first.Mappings) != 1 || first.Mappings[0].InheritStatus != "not_inherited" {
		t.Fatal("first real adopted content", first, err)
	}
	ref := evidencepg.ScriptIndexRequest{ActorID: actor.ID, OrgID: actor.OrgID, ProjectID: pid, VersionID: saved.VersionID}
	var job taskruntime.Job
	if err := db.Raw("SELECT kind,business_id,reference FROM infra.task_work WHERE kind=? AND business_id=?", evidencepg.ScriptIndexTask, ref.BusinessID()).Scan(&job).Error; err != nil || len(job.Reference) == 0 {
		t.Fatal("adopt did not enqueue script indexing", job, err)
	}
	index := agentwire.EvidenceIndexHandler(db, objects)
	for range 2 {
		if _, err := index(t.Context(), job); err != nil {
			t.Fatal("indexing adopted version", err)
		}
	}
	result, err := evidencepg.Search(db, pid, evidence.Query{Text: "渡轮穿过浓雾", Kinds: []string{"script_source"}})
	if err != nil || len(result.Items) != 1 || result.Items[0].Excerpt != "清晨渡轮穿过浓雾，码头渐显。" || result.Items[0].Start == nil || *result.Items[0].Start != 0 {
		t.Fatal("adopted script text not searchable", result, err)
	}
	repeat, err := app.NewAdoptService(scriptStore(db), time.Now).Adopt(t.Context(), scriptActor(actor), command)
	if err != nil || repeat.VersionID != first.VersionID || repeat.ScriptRevision != first.ScriptRevision {
		t.Fatal("permanent original adopt", repeat, err)
	}
	command.Key = uuid.New()
	command.ExpectedRevision = 3
	same, err := adopt.Adopt(t.Context(), scriptActor(actor), command)
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
	second, err := sources.Write(t.Context(), scriptActor(actor), input)
	if err != nil {
		t.Fatal(err)
	}
	newView, err := store.Episodes(t.Context(), scriptActor(actor), pid, second.VersionID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ConfirmSplit(t.Context(), scriptActor(actor), app.SplitCommand{ProjectID: pid, VersionID: second.VersionID, Key: uuid.New(), RequestID: uuid.New(), ExpectedRevision: 4, CandidateSetID: second.SplitSetID, Boundaries: newView.Candidate.Boundaries}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	command.Key = uuid.New()
	command.VersionID = second.VersionID
	command.ExpectedRevision = 5
	command.AckInvalidate = true
	if _, err := adopt.Adopt(t.Context(), scriptActor(actor), command); !errors.Is(err, app.ErrContextUnavailable) {
		t.Fatal("unproved downstream guessed empty", err)
	}
	current, err := sources.Workspace(t.Context(), scriptActor(actor), pid)
	if err != nil || current.State.Revision != 5 || current.State.AdoptedVersionID == nil || *current.State.AdoptedVersionID != saved.VersionID {
		t.Fatal("old adopted facts changed", current, err)
	}
	var count int64
	if err := owner.Raw(`SELECT count(*) FROM script.review_command WHERE project_id=? AND action='adopt'`, pid).Scan(&count).Error; err != nil || count != 2 {
		t.Fatal("missing owner wrote receipt", count, err)
	}
	if err := owner.Exec("UPDATE script.project_state SET adopted_version_id=NULL WHERE project_id=?", pid).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := index(t.Context(), job); err != nil {
		t.Fatal("late job should finish without reactivating the old version", err)
	}
	removed, err := evidencepg.Read(db, pid, result.Items[0].Reference)
	if err != nil || removed.Status != "unavailable" || len(removed.Items) != 0 {
		t.Fatal("old indexed text was still visible", removed, err)
	}
}
