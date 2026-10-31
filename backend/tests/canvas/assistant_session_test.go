package canvas_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	pgcanvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

func assistantDraft(t *testing.T, db *gorm.DB, a identity.Principal, p uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if err := db.Exec(`INSERT INTO workspace.prompt_draft(id,project_id,actor_id,revision,draft,draft_hash,input_hash) VALUES(?,?,?,1,'{}',repeat('a',64),repeat('b',64))`, id, p, a.ID).Error; err != nil {
		t.Fatal(err)
	}
	return id
}
func TestAssistantSessionPrivateBindingReplayAndFreeze(t *testing.T) {
	owner := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	runtime := canvasDB(t, "GLOWLOOM_TEST_CANVAS_M1_RUNTIME_DSN")
	a, p := seedCanvasActor(t, owner)
	canvas, err := application.NewService(canvasStore(runtime)).Create(t.Context(), a, p, uuid.NewString(), application.CreateInput{Name: "assistant test"})
	if err != nil {
		t.Fatal(err)
	}
	service := application.NewAssistant(pgcanvas.NewAssistantStore(runtime, mediawire.NewCanvasProjectAccess))
	input := application.AssistantCreate{ID: uuid.New(), Title: "Conversation"}
	session, err := service.Create(t.Context(), a, canvas.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := service.Create(t.Context(), a, canvas.ID, input)
	if err != nil || replay.ID != session.ID {
		t.Fatal(replay, err)
	}
	if _, err := service.Create(t.Context(), a, canvas.ID, application.AssistantCreate{ID: uuid.New(), Title: "Second empty"}); !errors.Is(err, application.ErrAssistantEmptySession) {
		t.Fatal("second empty session accepted", err)
	}
	input.Title = "changed"
	if _, err := service.Create(t.Context(), a, canvas.ID, input); !errors.Is(err, application.ErrIdempotencyConflict) {
		t.Fatal("changed create accepted", err)
	}
	draft := assistantDraft(t, owner, a, p)
	appendInput := application.AssistantAppend{ExpectedRevision: 1, DraftID: draft}
	session, err = service.Append(t.Context(), a, canvas.ID, session.ID, appendInput)
	if err != nil || session.Revision != 2 || len(session.Turns) != 1 {
		t.Fatal(session, err)
	}
	replay, err = service.Append(t.Context(), a, canvas.ID, session.ID, appendInput)
	if err != nil || replay.Revision != 2 {
		t.Fatal(replay, err)
	}
	other, _ := seedCanvasActor(t, owner)
	if _, err = service.Read(t.Context(), other, canvas.ID, session.ID); err == nil {
		t.Fatal("foreign scope exposed conversation")
	}
	next := assistantDraft(t, owner, a, p)
	appendInput = application.AssistantAppend{ExpectedRevision: 2, DraftID: next}
	if _, err = service.Append(t.Context(), a, canvas.ID, session.ID, appendInput); !errors.Is(err, application.ErrAssistantTurnActive) {
		t.Fatal("unresolved turn skipped", err)
	}
	op, sub := uuid.New(), uuid.New()
	if err = owner.Exec(`INSERT INTO operation.operation(id,project_id,target_type,capability,mode,input_hash,origin,status,region) VALUES(?,?,'agent_session','text.generate','test','test','agent','completed','domestic')`, op, p).Error; err != nil {
		t.Fatal(err)
	}
	if err = owner.Exec(`INSERT INTO operation.prompt_submission(id,actor_id,project_id,owner_key,epoch,operation_id,confirm_key,input_source,snapshot,quote,state) VALUES(?,?,?,?,1,?,?,'{}','{}','{}','accepted')`, sub, a.ID, p, "draft/"+draft.String(), op, uuid.New()).Error; err != nil {
		t.Fatal(err)
	}
	if err = owner.Exec(`INSERT INTO operation.prompt_submission_slot(actor_id,project_id,owner_key,epoch,submission_id) VALUES(?,?,?,1,?)`, a.ID, p, "draft/"+draft.String(), sub).Error; err != nil {
		t.Fatal(err)
	}
	if err := owner.Exec(`UPDATE operation.prompt_submission SET state='quoted' WHERE id=?`, sub).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(t.Context(), a, canvas.ID, application.AssistantCreate{ID: uuid.New(), Title: "Only quoted"}); !errors.Is(err, application.ErrAssistantEmptySession) {
		t.Fatal("quote alone allowed another empty session", err)
	}
	if err := owner.Exec(`UPDATE operation.prompt_submission SET state='accepted' WHERE id=?`, sub).Error; err != nil {
		t.Fatal(err)
	}
	listed, listErr := service.List(t.Context(), a, canvas.ID)
	if listErr != nil || len(listed) != 1 || listed[0].Turns[0].OperationID == nil || *listed[0].Turns[0].OperationID != op {
		t.Fatal("list failed to resolve current execution", listed, listErr)
	}
	if _, err := service.Create(t.Context(), a, canvas.ID, application.AssistantCreate{ID: uuid.New(), Title: "After sent"}); err != nil {
		t.Fatal("sent conversation blocked new empty session", err)
	}
	wrong := uuid.New()
	appendInput.PreviousOperationID = &wrong
	if _, err = service.Append(t.Context(), a, canvas.ID, session.ID, appendInput); err == nil {
		t.Fatal("continuation accepted a different history operation")
	}
	appendInput.PreviousOperationID = &op
	if err = owner.Exec(`UPDATE operation.prompt_submission_slot SET submission_id=NULL WHERE actor_id=? AND project_id=? AND owner_key=?`, a.ID, p, "draft/"+draft.String()).Error; err != nil {
		t.Fatal(err)
	}
	read, readErr := service.Read(t.Context(), a, canvas.ID, session.ID)
	if readErr != nil || read.Turns[0].OperationID == nil || *read.Turns[0].OperationID != op {
		t.Fatal("closed slot lost durable operation", read, readErr)
	}
	session, err = service.Append(t.Context(), a, canvas.ID, session.ID, appendInput)
	if err != nil || session.Revision != 3 || len(session.Turns) != 2 || session.Turns[0].OperationID == nil || *session.Turns[0].OperationID != op {
		t.Fatal(session, err)
	}
	if _, err = service.Append(t.Context(), a, canvas.ID, session.ID, application.AssistantAppend{ExpectedRevision: 1, DraftID: uuid.New()}); err == nil {
		t.Fatal("stale revision accepted")
	}
	if _, err = service.Append(t.Context(), a, canvas.ID, session.ID, application.AssistantAppend{ExpectedRevision: 3, DraftID: uuid.New(), Skill: "unknown"}); !errors.Is(err, domain.ErrInvalidCommand) {
		t.Fatal("unknown skill accepted", err)
	}
	if _, err = service.Append(t.Context(), a, canvas.ID, session.ID, application.AssistantAppend{ExpectedRevision: 3, DraftID: uuid.New(), Skill: "storyboard", Execution: "image"}); !errors.Is(err, domain.ErrInvalidCommand) {
		t.Fatal("unsupported professional executor admitted", err)
	}
	if err = owner.Exec(`UPDATE canvas.document SET is_delete=true WHERE id=?`, canvas.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = service.Read(t.Context(), a, canvas.ID, session.ID); !errors.Is(err, application.ErrNotFound) {
		t.Fatal("deleted canvas exposed private history", err)
	}
}
func TestAssistantDraftCannotRaceIntoTwoSessions(t *testing.T) {
	owner := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	runtime := canvasDB(t, "GLOWLOOM_TEST_CANVAS_M1_RUNTIME_DSN")
	a, p := seedCanvasActor(t, owner)
	canvas, err := application.NewService(canvasStore(runtime)).Create(t.Context(), a, p, uuid.NewString(), application.CreateInput{Name: "race"})
	if err != nil {
		t.Fatal(err)
	}
	service := application.NewAssistant(pgcanvas.NewAssistantStore(runtime, mediawire.NewCanvasProjectAccess))
	draft := assistantDraft(t, owner, a, p)
	sessions := make([]application.AssistantSession, 2)
	for i := range sessions {
		if i > 0 {
			canvas, err = application.NewService(canvasStore(runtime)).Create(t.Context(), a, p, uuid.NewString(), application.CreateInput{Name: "other canvas"})
			if err != nil {
				t.Fatal(err)
			}
		}
		sessions[i], err = service.Create(t.Context(), a, canvas.ID, application.AssistantCreate{ID: uuid.New(), Title: "race"})
		if err != nil {
			t.Fatal(err)
		}
	}
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, session := range sessions {
		wg.Go(func() {
			_, err := service.Append(t.Context(), a, session.CanvasID, session.ID, application.AssistantAppend{ExpectedRevision: 1, DraftID: draft})
			results <- err
		})
	}
	wg.Wait()
	close(results)
	success, conflicts := 0, 0
	for err := range results {
		switch {
		case err == nil:
			success++
		case errors.Is(err, application.ErrIdempotencyConflict):
			conflicts++
		default:
			t.Fatal(err)
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatal(success, conflicts)
	}
}

func TestAssistantEmptySessionCreationIsSerialized(t *testing.T) {
	owner := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	runtime := canvasDB(t, "GLOWLOOM_TEST_CANVAS_M1_RUNTIME_DSN")
	a, p := seedCanvasActor(t, owner)
	canvas, err := application.NewService(canvasStore(runtime)).Create(t.Context(), a, p, uuid.NewString(), application.CreateInput{Name: "empty race"})
	if err != nil {
		t.Fatal(err)
	}
	service := application.NewAssistant(pgcanvas.NewAssistantStore(runtime, mediawire.NewCanvasProjectAccess))
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			_, err := service.Create(t.Context(), a, canvas.ID, application.AssistantCreate{ID: uuid.New(), Title: "New"})
			results <- err
		})
	}
	wg.Wait()
	close(results)
	success, blocked := 0, 0
	for err := range results {
		switch {
		case err == nil:
			success++
		case errors.Is(err, application.ErrAssistantEmptySession):
			blocked++
		default:
			t.Fatal(err)
		}
	}
	if success != 1 || blocked != 1 {
		t.Fatal(success, blocked)
	}
}
