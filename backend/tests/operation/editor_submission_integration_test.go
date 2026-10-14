package operation_test

import (
	"context"
	"errors"
	"testing"
	"time"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	pgoperation "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/postgres"
	operationapp "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	pgprompt "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/adapter/postgres"
	promptapp "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/application"
	promptdomain "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/postgres"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type editorAccess struct{ tx *gorm.DB }

func (a editorAccess) AuthorizeDraft(ctx context.Context, actor identityapp.Principal, p uuid.UUID, write bool) error {
	_, err := pgworkspace.NewProjectContentAccessStore(a.tx).Authorize(ctx, actor, p, write)
	return err
}
func editorDraftStore(db *gorm.DB) *pgprompt.DraftStore {
	return pgprompt.NewDraftStore(db, func(tx *gorm.DB) promptapp.DraftAccess { return editorAccess{tx} })
}
func editorOperationStore(db *gorm.DB) *pgoperation.Store {
	return pgoperation.NewStoreWithEditor(db, nil, func(tx *gorm.DB) operationapp.EditorSnapshots { return editorDraftStore(tx) })
}
func TestEditorSubmissionRecoveryDiscardAndLateEpoch(t *testing.T) {
	db := operationStoreDB(t)
	actor, p := operationStoreProject(t, db)
	catalog, _, provider := seedQuoteCatalog(t, db, p, time.Now().UTC())
	if err := db.Exec(`UPDATE catalog.provider SET status='active' WHERE id=?`, provider).Error; err != nil {
		t.Fatal(err)
	}
	draft, err := promptdomain.DecodeDraft([]byte(`{"schemaVersion":1,"document":{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"测试"}]}]},"references":[],"modelVersion":null,"mode":null,"params":{},"labelState":{"highWater":{"image":0,"video":0,"audio":0,"subject":0},"allocations":{}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`UPDATE catalog.model_profile_version SET limits='{"max_outputs":1}' WHERE id=?`, *catalog.ModelProfileVersionID).Error; err != nil {
		t.Fatal(err)
	}
	version, mode := catalog.ModelProfileVersionID.String(), "text_to_image"
	draft.ModelVersion = &version
	draft.Mode = &mode
	saved, err := editorDraftStore(db).SaveDraft(t.Context(), actor, promptapp.DraftWrite{ProjectID: p, Key: uuid.New(), Draft: draft})
	if err != nil {
		t.Fatal(err)
	}
	source := promptdomain.InputSource{Kind: "prompt_draft", DraftID: saved.DraftID, Revision: 1}
	store := editorOperationStore(db)
	empty, err := store.CurrentSubmission(t.Context(), actor, p, source)
	if err != nil || empty.Epoch != 0 || empty.Submission != nil {
		t.Fatal(empty, err)
	}
	epoch := int64(0)
	input := operationapp.CreateFreeQuoteInput{ProjectID: p, RequestID: uuid.NewString(), FreeQuoteItemInput: operationapp.FreeQuoteItemInput{InputSource: &source, ExpectedSubmissionEpoch: &epoch}}
	quote, err := operationapp.NewCreateFreeQuoteCommand(store).Execute(t.Context(), actor, input)
	if err != nil || quote.SubmissionID == nil || quote.ConfirmKey == nil {
		t.Fatal(quote, err)
	}
	recovered, err := store.CurrentSubmission(t.Context(), actor, p, source)
	if err != nil || recovered.Submission == nil || recovered.Submission.OperationID != quote.OperationID {
		t.Fatal("cache-independent discovery", err, recovered)
	}
	replay, err := store.CreateFreeQuote(t.Context(), actor, input)
	if err != nil || replay.OperationID != quote.OperationID {
		t.Fatal("quote replay", err)
	}
	resolve := operationapp.SubmissionResolution{ProjectID: p, ID: *quote.SubmissionID, ExpectedEpoch: 1, Action: "discard_quote", Key: uuid.New()}
	closed, err := store.ResolveSubmission(t.Context(), actor, resolve)
	if err != nil || closed.Epoch != 2 {
		t.Fatal("discard", closed, err)
	}
	if _, err := store.ConfirmSingleQuote(t.Context(), actor, operationapp.ConfirmSingleQuoteInput{ProjectID: p, OperationID: quote.OperationID, RequestID: quote.ConfirmKey.String()}); err == nil {
		t.Fatal("discarded quote confirmed")
	}
	late := input
	late.RequestID = uuid.NewString()
	if _, err := store.CreateFreeQuote(t.Context(), actor, late); !errors.Is(err, operationapp.ErrSubmissionEpoch) {
		t.Fatal("late epoch created new operation", err)
	}
	epoch = 2
	input.RequestID = uuid.NewString()
	next, err := store.CreateFreeQuote(t.Context(), actor, input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ResolveSubmission(t.Context(), actor, resolve); err != nil {
		t.Fatal("resolve replay", err)
	}
	current, err := store.CurrentSubmission(t.Context(), actor, p, source)
	if err != nil || current.Submission == nil || current.Submission.OperationID != next.OperationID {
		t.Fatal("old resolve cleared new slot", err)
	}
	confirm := operationapp.ConfirmSingleQuoteInput{ProjectID: p, OperationID: next.OperationID, RequestID: next.ConfirmKey.String()}
	accepted, err := store.ConfirmSingleQuote(t.Context(), actor, confirm)
	if err != nil {
		t.Fatal("confirm", err)
	}
	repeated, err := store.ConfirmSingleQuote(t.Context(), actor, confirm)
	if err != nil || repeated.ReservationID != accepted.ReservationID {
		t.Fatal("confirm replay", err)
	}
	if _, err := store.ResolveSubmission(t.Context(), actor, operationapp.SubmissionResolution{ProjectID: p, ID: *next.SubmissionID, ExpectedEpoch: 3, Action: "discard_quote", Key: uuid.New()}); !errors.Is(err, operationapp.ErrSubmissionAccepted) {
		t.Fatal("accepted discarded", err)
	}
	if _, err := store.ResolveSubmission(t.Context(), actor, operationapp.SubmissionResolution{ProjectID: p, ID: *next.SubmissionID, ExpectedEpoch: 3, Action: "acknowledge_result", Key: uuid.New()}); err != nil {
		t.Fatal("acknowledge", err)
	}
}
