package operation_test

import (
	"context"
	"errors"
	"testing"
	"time"

	billingdomain "github.com/StephenQiu30/glowloom-server/backend/internal/billing/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	pgoperation "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/postgres"
	operationapp "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	pgprompt "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/adapter/postgres"
	promptapp "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/application"
	promptdomain "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
)

type editorAccess struct{ tx *gorm.DB }

func (a editorAccess) AuthorizeDraft(ctx context.Context, actor identityapp.Principal, p uuid.UUID, write bool) error {
	_, err := operationProjectContentAccess(a.tx).Authorize(ctx, actor, p, write)
	return err
}
func editorDraftStore(db *gorm.DB) *pgprompt.DraftStore {
	return pgprompt.NewDraftStore(db, func(tx *gorm.DB) promptapp.DraftAccess { return editorAccess{tx} })
}
func editorOperationStore(db *gorm.DB) *pgoperation.Store {
	return pgoperation.NewScopedStoreWithEditor(db, nil, func(tx *gorm.DB) operationapp.EditorSnapshots { return editorDraftStore(tx) }, operationRequestAccess, operationWorkerAccess, operationPlatformAccess).WithExecutionAvailability(fixtureExecutionAvailability)
}
func TestEditorSubmissionRecoveryDiscardAndLateEpoch(t *testing.T) {
	db := operationStoreDB(t)
	actor, p := operationStoreProject(t, db)
	catalog, _, provider := seedGenerationQuoteCatalog(t, db, p, time.Now().UTC())
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
	if err != nil || quote.SubmissionID == nil || quote.ConfirmKey == nil || quote.FinalPrompt != "测试" || quote.CompilerVersion == "" {
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
	epoch = 4
	input.RequestID = uuid.NewString()
	staleQuote, err := store.CreateFreeQuote(t.Context(), actor, input)
	if err != nil {
		t.Fatal(err)
	}
	draft.Document.Content[0].Content[0].Text = "new intent"
	if _, err := editorDraftStore(db).SaveDraft(t.Context(), actor, promptapp.DraftWrite{ProjectID: p, DraftID: saved.DraftID, ExpectedRevision: 1, Key: uuid.New(), Draft: draft}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ConfirmSingleQuote(t.Context(), actor, operationapp.ConfirmSingleQuoteInput{ProjectID: p, OperationID: staleQuote.OperationID, RequestID: staleQuote.ConfirmKey.String()}); !errors.Is(err, promptdomain.ErrInputStale) {
		t.Fatal("edited source confirmed", err)
	}
	if _, err := store.ResolveSubmission(t.Context(), actor, operationapp.SubmissionResolution{ProjectID: p, ID: *staleQuote.SubmissionID, ExpectedEpoch: 5, Action: "discard_quote", Key: uuid.New()}); err != nil {
		t.Fatal(err)
	}
	source.Revision = 2
	epoch = 6
	input.RequestID = uuid.NewString()
	racing, err := store.CreateFreeQuote(t.Context(), actor, input)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	go func() {
		<-start
		_, err := store.ConfirmSingleQuote(t.Context(), actor, operationapp.ConfirmSingleQuoteInput{ProjectID: p, OperationID: racing.OperationID, RequestID: racing.ConfirmKey.String()})
		results <- err
	}()
	go func() {
		<-start
		_, err := store.ResolveSubmission(t.Context(), actor, operationapp.SubmissionResolution{ProjectID: p, ID: *racing.SubmissionID, ExpectedEpoch: 7, Action: "discard_quote", Key: uuid.New()})
		results <- err
	}()
	close(start)
	first, second := <-results, <-results
	if (first == nil) == (second == nil) {
		t.Fatal("confirm/discard must have exactly one winner", first, second)
	}
	var count int
	if err := db.Raw(`SELECT count(*) FROM billing.reservation WHERE operation_id=?`, racing.OperationID).Scan(&count).Error; err != nil || count > 1 {
		t.Fatal("duplicate reservation", count, err)
	}

	if err := db.Exec(`UPDATE workspace.project SET is_delete=true WHERE id=?`, p).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := store.ConfirmSingleQuote(t.Context(), actor, confirm); err == nil {
		t.Fatal("confirmation replay bypassed revoked owner")
	}
}

func TestCanvasEditorConcurrentNodeOperationsHaveIndependentHistory(t *testing.T) {
	db := operationStoreDB(t)
	actor, project := operationStoreProject(t, db)
	catalog, _, provider := seedGenerationQuoteCatalog(t, db, project, time.Now().UTC())
	require.NoError(t, db.Exec("UPDATE catalog.provider SET status='active' WHERE id=?", provider).Error)
	require.NoError(t, db.Exec("UPDATE catalog.model_profile_version SET limits='{\"max_outputs\":1}' WHERE id=?", catalog.ModelProfileVersionID).Error)
	canvasID := uuid.New()
	require.NoError(t, db.Exec("INSERT INTO canvas.document(id,project_id,name,revision) VALUES(?,?,'并行节点',7)", canvasID, project).Error)
	video, model, videoProvider := seedUnfundedQuoteCatalog(t, db, project, time.Now().UTC())
	require.NoError(t, db.Exec("UPDATE catalog.provider SET status='active' WHERE id=?", videoProvider).Error)
	require.NoError(t, db.Exec("UPDATE catalog.model_profile SET capability='video.generate' WHERE id=?", model).Error)
	require.NoError(t, db.Exec(`UPDATE catalog.model_profile_version SET modes=ARRAY['text2video'],limits='{"max_outputs":1}' WHERE id=?`, video.ModelProfileVersionID).Error)
	require.NoError(t, db.Exec(`UPDATE catalog.price_rule_version SET rule=jsonb_set(rule,'{base_micros}',to_jsonb(?::bigint)) WHERE id=?`, billingdomain.MicrosPerCredit, video.PriceRuleVersionID).Error)
	seedFixtureProcurement(t, db, *video.PriceRuleVersionID, *video.ModelProfileVersionID)
	nodes := []uuid.UUID{uuid.New(), uuid.New()}
	for idx, node := range nodes {
		kind, mode, version := "image", "text_to_image", catalog.ModelProfileVersionID
		if idx == 1 {
			kind, mode, version = "video", "text2video", video.ModelProfileVersionID
		}
		require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec(`INSERT INTO canvas.node(id,document_id,title,node_type,node_action,input_revision,content_version,config,x,y,width,height)
		VALUES(?,?,?,?,'resource',3,5,jsonb_build_object('editor','{"schemaVersion":1}'::jsonb,'draft',jsonb_build_object('composer',jsonb_build_object(
		'schemaVersion',1,'document','{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"并行测试"}]}]}'::jsonb,
		'references','[]'::jsonb,'modelVersion',?::text,'mode',?::text,'params','{}'::jsonb,'labelState','{"highWater":{"image":0,"video":0,"audio":0,"subject":0},"allocations":{}}'::jsonb))),0,0,300,300)`, node, canvasID, kind, kind, version.String(), mode).Error; err != nil {
				return err
			}
			return tx.Exec(`INSERT INTO canvas.node_input(node_id,input_revision,draft,created_by) SELECT id,input_revision,config->'draft',? FROM canvas.node WHERE id=?`, actor.ID, node).Error
		}))
	}
	store := editorOperationStore(db)
	epoch := int64(0)
	var group errgroup.Group
	for _, node := range nodes {
		group.Go(func() error {
			source := promptdomain.InputSource{Kind: "canvas_input", CanvasID: canvasID, NodeID: node, CanvasRevision: 7, InputRevision: 3}
			quote, err := store.CreateFreeQuote(t.Context(), actor, operationapp.CreateFreeQuoteInput{ProjectID: project, RequestID: uuid.NewString(), FreeQuoteItemInput: operationapp.FreeQuoteItemInput{InputSource: &source, ExpectedSubmissionEpoch: &epoch}})
			if err != nil {
				return err
			}
			_, err = store.ConfirmSingleQuote(t.Context(), actor, operationapp.ConfirmSingleQuoteInput{ProjectID: project, OperationID: quote.OperationID, RequestID: quote.ConfirmKey.String()})
			return err
		})
	}
	require.NoError(t, group.Wait())
	page, err := store.ListPublicTasks(t.Context(), actor, operationapp.ListTasksInput{ProjectID: project, CanvasID: &canvasID, Limit: 20})
	require.NoError(t, err)
	require.Len(t, page.Items, 2)
	var got []uuid.UUID
	for _, item := range page.Items {
		require.EqualValues(t, "confirmed", item.Status)
		require.Equal(t, "并行测试", item.CanvasPromptPreview)
		require.Equal(t, canvasID, item.Source.CanvasID)
		require.EqualValues(t, 7, item.Source.Revision)
		detail, err := store.ReadPublicTask(t.Context(), actor, project, item.ID)
		require.NoError(t, err)
		require.Equal(t, "5", detail.TargetContentVersion)
		got = append(got, item.Source.NodeID)
	}
	require.ElementsMatch(t, nodes, got)
}
