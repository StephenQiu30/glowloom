package script_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	app "github.com/StephenQiu30/lanverse/backend/internal/script/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
)

func TestScriptEpisodesPGWholeConfirmationHistoryRetainsIdentityAndPreface(t *testing.T) {
	db, owner := scriptTestDB(t)
	actor, pid := scriptActorProject(t, owner)
	objects := &sourceObjects{data: make(map[string][]byte)}
	store := scriptStore(db)
	sourceService := app.NewSourceService(store, objects, time.Now)
	_, input := sourceCommand()
	input.ProjectID = pid
	input.Action = "import"
	input.Sources = append(input.Sources, app.SourceInput{Kind: "episode", Title: "第二集", Status: "draft", Document: domain.RichDocument{Type: "doc", Content: []domain.RichDocument{{Type: "paragraph", Content: []domain.RichDocument{{Type: "text", Text: "中文"}}}}}})
	saved, err := sourceService.Write(t.Context(), actor, input)
	if err != nil {
		t.Fatal(err)
	}
	view, err := store.Episodes(t.Context(), actor, pid, saved.VersionID)
	if err != nil || len(view.Candidate.Boundaries) != 2 || len(view.Episodes) != 0 {
		t.Fatal("candidate/formal separation", view, err)
	}
	confirm := app.SplitCommand{ProjectID: pid, VersionID: saved.VersionID, Key: uuid.New(), RequestID: uuid.New(), ExpectedRevision: 1, ExpectedSplitRevision: 0, CandidateSetID: saved.SplitSetID, Boundaries: view.Candidate.Boundaries}
	first, err := store.ConfirmSplit(t.Context(), actor, confirm, time.Now().UTC())
	if err != nil || len(first.Episodes) != 2 || first.ScriptRevision != 2 || first.Episodes[0].SeqNo != 1 || first.Episodes[0].Start != 0 || first.Episodes[0].End != 5 {
		t.Fatal("formal complete boundary", first, err)
	}
	rename := confirm
	rename.Key = uuid.New()
	rename.ExpectedRevision = 2
	rename.ExpectedSplitRevision = 1
	rename.Boundaries = append([]domain.EpisodeBoundary(nil), confirm.Boundaries...)
	rename.Boundaries[0].Title = "新标题"
	second, err := store.ConfirmSplit(t.Context(), actor, rename, time.Now().UTC())
	if err != nil || second.SplitSetID != first.SplitSetID || second.Episodes[0].ID != first.Episodes[0].ID || len(second.RenamedIDs) != 1 {
		t.Fatal("title-only stable identity/fixed first formal set", second, err)
	}
	replay, err := store.ConfirmSplit(t.Context(), actor, confirm, time.Now().UTC())
	if err != nil || replay.ScriptRevision != 2 || replay.Episodes[0].Title != first.Episodes[0].Title {
		t.Fatal("immutable complete original confirmation", replay, err)
	}
	invalid := rename
	invalid.Key = uuid.New()
	invalid.ExpectedRevision = 3
	invalid.ExpectedSplitRevision = 2
	invalid.Boundaries = invalid.Boundaries[:1]
	if _, err := store.ConfirmSplit(t.Context(), actor, invalid, time.Now().UTC()); !errors.Is(err, domain.ErrInvalidSpan) {
		t.Fatal("incomplete boundaries accepted", err)
	}
	var count int64
	if err := owner.Raw(`SELECT count(*) FROM script.split_confirmation WHERE project_id=?`, pid).Scan(&count).Error; err != nil || count != 2 {
		t.Fatal("complete confirmation history", count, err)
	}
	if err := db.Exec(`UPDATE script.split_confirmation SET revision=99 WHERE project_id=?`, pid).Error; err == nil {
		t.Fatal("runtime can rewrite confirmations")
	}
}

func TestScriptStructuresPGImmutableCandidatesConfirmAndMissingImpactFence(t *testing.T) {
	db, owner := scriptTestDB(t)
	actor, pid := scriptActorProject(t, owner)
	objects := &sourceObjects{data: make(map[string][]byte)}
	store := scriptStore(db)
	sources := app.NewSourceService(store, objects, time.Now)
	_, input := sourceCommand()
	input.ProjectID = pid
	input.Sources[0].Document = domain.RichDocument{Type: "doc", Content: []domain.RichDocument{{Type: "paragraph", Content: []domain.RichDocument{{Type: "text", Text: "走进客厅\n你好😀 "}}}}}
	saved, err := sources.Write(t.Context(), actor, input)
	if err != nil {
		t.Fatal(err)
	}
	view, err := store.Episodes(t.Context(), actor, pid, saved.VersionID)
	if err != nil {
		t.Fatal(err)
	}
	split := app.SplitCommand{ProjectID: pid, VersionID: saved.VersionID, Key: uuid.New(), RequestID: uuid.New(), ExpectedRevision: 1, ExpectedSplitRevision: 0, CandidateSetID: saved.SplitSetID, Boundaries: view.Candidate.Boundaries}
	formal, err := store.ConfirmSplit(t.Context(), actor, split, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	episodes := app.NewEpisodeService(store, sources, time.Now)
	save := app.StructureCommand{ProjectID: pid, EpisodeID: formal.Episodes[0].ID, Key: uuid.New(), RequestID: uuid.New(), ExpectedRevision: 2, ExpectedEpisodeRevision: 1, Document: manualStructure()}
	first, err := episodes.SaveStructure(t.Context(), actor, save)
	if err != nil || first.VersionNo != 1 || first.ReviewStatus != "candidate" {
		t.Fatal("actual structure saved", first, err)
	}
	confirm := save
	confirm.Key = uuid.New()
	confirm.ExpectedRevision = 3
	confirm.ExpectedEpisodeRevision = 2
	confirm.BaseStructureVersionNo = 1
	confirm.Document = domain.StructureDocument{}
	confirmed, err := episodes.ConfirmStructure(t.Context(), actor, confirm)
	if err != nil || confirmed.ReviewStatus != "confirmed" || confirmed.ScriptRevision != 4 {
		t.Fatal("first manual confirmation", confirmed, err)
	}
	save.Key = uuid.New()
	save.ExpectedRevision = 4
	save.ExpectedEpisodeRevision = 3
	save.BaseStructureVersionNo = 1
	save.Document.Scenes[0].Items[1].Content = "欢迎回来😀"
	second, err := episodes.SaveStructure(t.Context(), actor, save)
	if err != nil || second.VersionNo != 2 || second.StructureID == first.StructureID {
		t.Fatal("new immutable candidate", second, err)
	}
	current, err := store.Episode(t.Context(), actor, save.EpisodeID)
	if err != nil || current.ConfirmedStructureID == nil || *current.ConfirmedStructureID != first.StructureID || *current.CurrentStructureID != second.StructureID {
		t.Fatal("old confirmed facts overwritten", current, err)
	}
	confirm.Key = uuid.New()
	confirm.ExpectedRevision = 5
	confirm.ExpectedEpisodeRevision = 4
	confirm.BaseStructureVersionNo = 2
	if _, err := episodes.ConfirmStructure(t.Context(), actor, confirm); !errors.Is(err, app.ErrConfirmationRequired) {
		t.Fatal("changed confirmed structure lacks ack", err)
	}
	confirm.AckInvalidate = true
	if _, err := episodes.ConfirmStructure(t.Context(), actor, confirm); !errors.Is(err, app.ErrContextUnavailable) {
		t.Fatal("missing downstream impact guessed zero", err)
	}
	var count int64
	if err := owner.Raw(`SELECT count(*) FROM script.episode_structure WHERE project_id=?`, pid).Scan(&count).Error; err != nil || count != 2 {
		t.Fatal("all structure history", count, err)
	}
	if err := owner.Raw(`SELECT count(*) FROM script.review_command WHERE project_id=?`, pid).Scan(&count).Error; err != nil || count != 4 {
		t.Fatal("rejected confirmations wrote permanent result", count, err)
	}
	for _, table := range []string{"script.episode_structure", "script.scene", "script.dialogue_line", "script.action_line"} {
		if err := db.Exec(`DELETE FROM `+table+` WHERE project_id=?`, pid).Error; err == nil {
			t.Fatal("runtime can delete structure history", table)
		}
	}
}

func TestScriptReviewPGAuditZeroRowAndCrossFamilyKeysRollback(t *testing.T) {
	db, owner := scriptTestDB(t)
	actor, pid := scriptActorProject(t, owner)
	objects := &sourceObjects{data: make(map[string][]byte)}
	store := scriptStore(db)
	sources := app.NewSourceService(store, objects, time.Now)
	_, input := sourceCommand()
	input.ProjectID = pid
	saved, err := sources.Write(t.Context(), actor, input)
	if err != nil {
		t.Fatal(err)
	}
	episodes := app.NewEpisodeService(store, sources, time.Now)
	view, err := episodes.Episodes(t.Context(), actor, pid, saved.VersionID)
	if err != nil {
		t.Fatal(err)
	}
	confirm := app.SplitCommand{ProjectID: pid, VersionID: saved.VersionID, Key: input.Key, RequestID: uuid.New(), ExpectedRevision: 1, ExpectedSplitRevision: 0, CandidateSetID: saved.SplitSetID, Boundaries: view.Candidate.Boundaries}
	if _, err := episodes.ConfirmSplit(t.Context(), actor, confirm); !errors.Is(err, app.ErrIdempotencyConflict) {
		t.Fatal("cross-family key reused", err)
	}
	confirm.Key = uuid.New()
	scriptNullTrigger(t, owner, "infra.outbox", "NEW.partition_key='"+pid.String()+"' AND NEW.payload#>>'{data,action}'='script.split_confirmed'")
	if _, err := episodes.ConfirmSplit(t.Context(), actor, confirm); !errors.Is(err, app.ErrConflict) {
		t.Fatal("zero-row audit accepted confirmation", err)
	}
	state, err := sources.Workspace(t.Context(), actor, pid)
	if err != nil || state.State.Revision != 1 {
		t.Fatal("zero-row changed script head", state, err)
	}
	current, err := episodes.Episodes(t.Context(), actor, pid, saved.VersionID)
	if err != nil || len(current.Episodes) != 0 || current.Head.SplitRevision != 0 || current.Head.ConfirmedSetID != nil {
		t.Fatal("partial formal confirmation", current, err)
	}
	var count int64
	for _, table := range []string{"script.split_confirmation", "script.review_command"} {
		if err := owner.Table(table).Where("project_id=?", pid).Count(&count).Error; err != nil || count != 0 {
			t.Fatal("zero-row permanent facts", table, count, err)
		}
	}
	if err := owner.Raw(`SELECT revision FROM workspace.project WHERE id=?`, pid).Scan(&count).Error; err != nil || count != 2 {
		t.Fatal("zero-row project touched", count, err)
	}
}
