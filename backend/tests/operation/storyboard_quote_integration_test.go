package operation_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	canvas "github.com/StephenQiu30/glowloom-server/backend/creation/canvas/domain"
	pg "github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/execution/operation/domain"
)

func TestStoryboardQuoteLegacyStoreRechecksShotBeforeReserve(t *testing.T) {
	db := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, db)
	catalogQuote, modelID, _ := seedGenerationQuoteCatalog(t, db, projectID, time.Now())
	modelKey := "quote-model-" + modelID.String()
	require.NoError(t, db.Exec("UPDATE catalog.model_profile SET capability='video.generate' WHERE id=?", modelID).Error)
	require.NoError(t, db.Exec(`UPDATE catalog.model_profile_version
		SET modes=ARRAY['text2video'], limits='{"max_outputs":1,"durations_ms":[4000]}'::jsonb,
			param_schema='[{"field":"duration_ms","type":"integer","required":true}]'::jsonb
		WHERE id=?`, catalogQuote.ModelProfileVersionID).Error)
	canvasID, boardID, shotID := uuid.New(), uuid.New(), uuid.New()
	shot := canvas.StoryboardShot{ID: shotID, Revision: 1, StoryboardDraft: canvas.StoryboardDraft{Title: "一镜", Action: "推门", DurationMS: 4000}}
	config := func() string {
		raw, err := json.Marshal(canvas.NodeConfig{Storyboard: &canvas.Storyboard{Shots: []canvas.StoryboardShot{shot}}})
		require.NoError(t, err)
		return string(raw)
	}
	require.NoError(t, db.Exec(`INSERT INTO canvas.document(id,project_id,name) VALUES(?,?,'Storyboard')`, canvasID, projectID).Error)
	require.NoError(t, db.Exec(`INSERT INTO canvas.node(id,document_id,node_type,node_action,config,x,y)
		VALUES(?,?,?,'tool',?::jsonb,0,0)`, boardID, canvasID, canvas.NodeTypeStoryboard, config()).Error)
	store := operationStore(db)
	input := application.CreateFreeQuoteInput{ProjectID: projectID, RequestID: uuid.NewString(), FreeQuoteItemInput: application.FreeQuoteItemInput{
		ModelKey: modelKey, Capability: "video.generate", Mode: "text2video", OutputCount: 1,
		Prompt: application.StoryboardShotPrompt("一镜", "推门", "", "", ""),
		Params: json.RawMessage(`{"duration_ms":4000}`),
		Source: &domain.CanvasSource{CanvasID: canvasID, NodeID: boardID, Revision: 1, ShotID: &shotID, ShotRevision: int64Pointer(1)},
	}}
	quote, err := store.CreateFreeQuote(t.Context(), actor, input)
	require.NoError(t, err)
	frozen, err := store.FindOperation(t.Context(), actor, projectID, quote.OperationID)
	require.NoError(t, err)
	require.Equal(t, input.Source, frozen.Source)
	shot.Revision++
	shot.Action = "停步"
	require.NoError(t, db.Exec("UPDATE canvas.node SET config=?::jsonb WHERE id=?", config(), boardID).Error)
	require.NoError(t, db.Exec("UPDATE canvas.document SET revision=2 WHERE id=?", canvasID).Error)
	_, err = store.ConfirmSingleQuote(t.Context(), actor, application.ConfirmSingleQuoteInput{
		ProjectID: projectID, OperationID: quote.OperationID, RequestID: uuid.NewString(),
	})
	var stale *pg.QuoteStaleError
	require.ErrorAs(t, err, &stale)
	require.Equal(t, []string{"canvas_source_changed"}, stale.Reasons)
	frozen, err = store.FindOperation(t.Context(), actor, projectID, quote.OperationID)
	require.NoError(t, err)
	require.Equal(t, domain.StatusExpired, frozen.Status)
	var reservations int
	require.NoError(t, db.Raw("SELECT count(*) FROM billing.reservation WHERE operation_id=?", quote.OperationID).Scan(&reservations).Error)
	require.Zero(t, reservations)
}
