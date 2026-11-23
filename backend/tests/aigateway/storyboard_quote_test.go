package aigateway_test

import (
	"encoding/json"
	"io"
	"net/http"
	"os/exec"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	canvas "github.com/StephenQiu30/glowloom-server/backend/creation/canvas/domain"
	pggateway "github.com/StephenQiu30/glowloom-server/backend/execution/gateway/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
	operationhttp "github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/http"
	operation "github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
	operationdomain "github.com/StephenQiu30/glowloom-server/backend/execution/operation/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/app/aigateway"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/media/application"
)

func TestStoryboardQuoteAndConfirmationRetainStableShot(t *testing.T) {
	db, store, _, actor := fixture(t, 300)
	model := videoBillingModel(t, store)
	project := entrypointProject(t, db, actor)
	entrypointSession(t, db, actor)
	canvasID, boardID := uuid.New(), uuid.New()
	first := canvas.StoryboardShot{ID: uuid.New(), Revision: 1, StoryboardDraft: canvas.StoryboardDraft{Title: "开场", Action: "推门入室", DurationMS: 4000}}
	second := canvas.StoryboardShot{ID: uuid.New(), Revision: 1, StoryboardDraft: canvas.StoryboardDraft{Title: "走廊", Action: "走向窗前", DurationMS: 4000}}
	encodeShots := func(shots ...canvas.StoryboardShot) string {
		t.Helper()
		config, err := json.Marshal(canvas.NodeConfig{Storyboard: &canvas.Storyboard{Shots: shots}})
		require.NoError(t, err)
		return string(config)
	}
	require.NoError(t, db.Exec(`INSERT INTO canvas.document(id,project_id,name) VALUES(?,?,'Storyboard')`, canvasID, project).Error)
	require.NoError(t, db.Exec(`INSERT INTO canvas.node(id,document_id,node_type,node_action,config,x,y)
		VALUES(?,?,?,'tool',?::jsonb,0,0)`, boardID, canvasID, canvas.NodeTypeStoryboard, encodeShots(first, second)).Error)
	setShots := func(revision int64, shots ...canvas.StoryboardShot) {
		t.Helper()
		require.NoError(t, db.Exec("UPDATE canvas.node SET config=?::jsonb WHERE id=?", encodeShots(shots...), boardID).Error)
		require.NoError(t, db.Exec("UPDATE canvas.document SET revision=? WHERE id=?", revision, canvasID).Error)
	}
	var submits, polls int
	client := mediaClient(func(req *http.Request) (*http.Response, error) {
		if req.Method == http.MethodPost {
			submits++
			body, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			require.Contains(t, string(body), "推门入室")
			require.NotContains(t, string(body), "推门后停步")
			return mediaResponse(200, `{"id":"storyboard-video-fixture"}`), nil
		}
		polls++
		require.Equal(t, "/api/v3/contents/generations/tasks/storyboard-video-fixture", req.URL.Path)
		return mediaResponse(200, `{"id":"storyboard-video-fixture","status":"succeeded","content":{"video_url":"https://fixture.tos-cn-beijing.volces.com/storyboard.mp4"},"usage":{"completion_tokens":86400}}`), nil
	})
	runtime := &aigateway.Runtime{
		DB: db, Objects: fixtureMediaStorage(t, db),
		Provider: client,
	}
	creation := aigateway.Creation{Runtime: runtime}
	inputFor := func(shot canvas.StoryboardShot, revision int64) operationhttp.QuoteItemRequest {
		return operationhttp.QuoteItemRequest{
			ModelKey: model.Name, Capability: model.Capability, Mode: "text2video", OutputCount: 1,
			Prompt: operation.StoryboardShotPrompt(shot.Title, shot.Action, shot.Dialogue, shot.Camera, shot.Sound),
			Params: json.RawMessage(`{"duration_ms":4000}`),
			Source: &operationdomain.CanvasSource{CanvasID: canvasID, NodeID: boardID, Revision: revision, ShotID: &shot.ID, ShotRevision: &shot.Revision},
		}
	}
	firstInput := inputFor(first, 1)
	bad := firstInput
	bad.Prompt = "another scene"
	_, err := creation.Quote(t.Context(), actor, project, uuid.NewString(), bad)
	require.ErrorIs(t, err, domain.ErrInvalid)
	bad = firstInput
	bad.Params = json.RawMessage(`{"duration_ms":5000}`)
	_, err = creation.Quote(t.Context(), actor, project, uuid.NewString(), bad)
	require.ErrorIs(t, err, domain.ErrInvalid)
	bad = inputFor(first, 1)
	*bad.Source.ShotRevision++
	_, err = creation.Quote(t.Context(), actor, project, uuid.NewString(), bad)
	require.ErrorIs(t, err, operation.ErrQuoteSourceStale)
	bad = firstInput
	bad.ModelKey = "unavailable-" + uuid.NewString()
	_, err = creation.Quote(t.Context(), actor, project, uuid.NewString(), bad)
	require.ErrorIs(t, err, domain.ErrUnavailable)
	unrelated := entrypointProject(t, db, actor)
	_, err = creation.Quote(t.Context(), actor, unrelated, uuid.NewString(), firstInput)
	require.ErrorIs(t, err, operation.ErrQuoteSourceStale)

	firstKey := uuid.NewString()
	firstQuote, err := creation.Quote(t.Context(), actor, project, firstKey, firstInput)
	require.NoError(t, err)
	require.NotNil(t, firstQuote.ConfirmKey)
	gotReplay, err := creation.Quote(t.Context(), actor, project, firstKey, firstInput)
	require.NoError(t, err)
	require.Equal(t, firstQuote.OperationID, gotReplay.OperationID)
	competingQuote, err := creation.Quote(t.Context(), actor, project, uuid.NewString(), firstInput)
	require.NoError(t, err)
	secondQuote, err := creation.Quote(t.Context(), actor, project, uuid.NewString(), inputFor(second, 1))
	require.NoError(t, err)
	var holds int64
	require.NoError(t, db.Raw("SELECT held FROM ai.account WHERE scope_id=?", actor.ScopeID).Scan(&holds).Error)
	require.Zero(t, holds)

	setShots(2, second, first)
	_, err = creation.Confirm(t.Context(), actor, project, firstQuote.OperationID, uuid.NewString())
	require.ErrorIs(t, err, domain.ErrConflict)
	var wg sync.WaitGroup
	const confirmations = 5
	results := make(chan error, confirmations)
	for range confirmations {
		wg.Go(func() {
			_, err := creation.Confirm(t.Context(), actor, project, firstQuote.OperationID, firstQuote.ConfirmKey.String())
			results <- err
		})
	}
	wg.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
	_, err = creation.Confirm(t.Context(), actor, project, competingQuote.OperationID, competingQuote.ConfirmKey.String())
	require.ErrorIs(t, err, domain.ErrUnknown)

	first.Action = "推门后停步"
	first.Revision++
	second.Action = "回身望向门外"
	second.Revision++
	setShots(3, second, first)
	_, err = creation.Confirm(t.Context(), actor, project, secondQuote.OperationID, secondQuote.ConfirmKey.String())
	require.ErrorIs(t, err, operation.ErrQuoteSourceStale)
	firstRecord, err := creation.Confirm(t.Context(), actor, project, firstQuote.OperationID, firstQuote.ConfirmKey.String())
	require.NoError(t, err)
	require.Equal(t, firstQuote.OperationID, firstRecord.ID)
	_, err = creation.Quote(t.Context(), actor, project, uuid.NewString(), inputFor(first, 3))
	require.ErrorIs(t, err, domain.ErrUnknown)

	revised, err := creation.Quote(t.Context(), actor, project, uuid.NewString(), inputFor(second, 3))
	require.NoError(t, err)
	require.NotNil(t, revised.ConfirmKey)
	require.NoError(t, db.Exec("UPDATE identity.user_session SET revoked_at=clock_timestamp() WHERE id=?", actor.SessionID).Error)
	_, err = creation.Confirm(t.Context(), actor, project, revised.OperationID, revised.ConfirmKey.String())
	require.ErrorIs(t, err, domain.ErrForbidden)
	require.NoError(t, db.Exec("UPDATE identity.user_session SET revoked_at=NULL WHERE id=?", actor.SessionID).Error)
	require.NoError(t, db.Exec("UPDATE ai.model SET enabled=false WHERE id=?", model.ID).Error)
	_, err = creation.Confirm(t.Context(), actor, project, revised.OperationID, revised.ConfirmKey.String())
	require.ErrorIs(t, err, operation.ErrQuoteStale)
	require.NoError(t, db.Exec("UPDATE ai.model SET enabled=true WHERE id=?", model.ID).Error)
	candidates, err := creation.StoryboardCandidates(t.Context(), actor, project, canvasID, boardID, second.ID)
	require.NoError(t, err)
	require.Len(t, candidates, 2)
	require.Equal(t, revised.OperationID, candidates[0].ID)
	require.EqualValues(t, 2, *candidates[0].Source.ShotRevision)
	require.Equal(t, secondQuote.OperationID, candidates[1].ID)
	require.EqualValues(t, 1, *candidates[1].Source.ShotRevision)

	publishMemory := func(shot *uuid.UUID, revision int64, statement string) {
		t.Helper()
		require.NoError(t, db.Exec(`INSERT INTO workspace.project_memory_revision
			(id,project_id,shot_id,key,revision,statement,withdrawn,confirmed_by,confirmed_at,request_id)
			VALUES (?,?,?,'audio',?,?,false,?,now(),?)`, uuid.New(), project, shot, revision, statement, actor.ID, uuid.New()).Error)
	}
	publishMemory(nil, 1, "全片无旁白")
	publishMemory(&second.ID, 1, "本镜一句独白")
	withMemory, err := creation.Quote(t.Context(), actor, project, uuid.NewString(), inputFor(second, 3))
	require.NoError(t, err)
	require.Contains(t, withMemory.FinalPrompt, "本镜一句独白")
	require.NotContains(t, withMemory.FinalPrompt, "全片无旁白")
	publishMemory(&second.ID, 2, "本镜一句独白")
	_, err = creation.Confirm(t.Context(), actor, project, withMemory.OperationID, withMemory.ConfirmKey.String())
	require.ErrorIs(t, err, operation.ErrQuoteSourceStale, "even an identical statement republishes a new immutable decision")

	task, err := creation.Task(t.Context(), actor, project, firstQuote.OperationID)
	require.NoError(t, err)
	require.Equal(t, firstInput.Source, task.Source)
	require.JSONEq(t, `{"duration":4,"ratio":"16:9","resolution":"720p"}`, string(task.Params))
	var work int64
	require.NoError(t, db.Raw("SELECT count(*) FROM ai.hold WHERE request_id IN ?", []uuid.UUID{firstQuote.OperationID, competingQuote.OperationID, secondQuote.OperationID, revised.OperationID}).Scan(&holds).Error)
	require.EqualValues(t, 1, holds)
	require.NoError(t, db.Raw("SELECT count(*) FROM infra.task_work WHERE kind='ai.execute' AND business_id=?", firstQuote.OperationID.String()).Scan(&work).Error)
	require.EqualValues(t, 1, work)
	require.Zero(t, submits)
	require.Zero(t, polls)

	video, err := exec.CommandContext(t.Context(), "ffmpeg", "-v", "error", "-f", "lavfi", "-i", "color=s=1280x720:r=24", "-t", "4", "-c:v", "libx264", "-movflags", "frag_keyframe+empty_moov", "-f", "mp4", "pipe:1").Output()
	require.NoError(t, err)
	objects := &durableBytes{t: t, db: db, routes: pgmedia.NewStorageRoutes(db), files: map[string][]byte{}}
	runtime.ResultDownloads = videoBillingDownload{video}
	runtime.MediaArchive = mediaapp.NewGeneratedOutputArchiver(objects.routes, objects, mediaflow.FFProber{}, mediaflow.FFRenderer{})
	_, err = creation.Execute(t.Context(), taskruntime.Job{Kind: pggateway.ExecutionKind, BusinessID: firstQuote.OperationID.String()})
	require.NoError(t, err)
	handlers := map[string]taskruntime.Handler{}
	runtime.RegisterWorker(handlers)
	for _, kind := range []string{"ai.poll", "ai.deliver", "ai.settle"} {
		_, err := handlers[kind](t.Context(), taskruntime.Job{Kind: kind, BusinessID: firstQuote.OperationID.String()})
		require.NoError(t, err)
	}
	task, err = creation.Task(t.Context(), actor, project, firstQuote.OperationID)
	require.NoError(t, err)
	require.Equal(t, operationdomain.StatusCompleted, task.Status)
	require.Len(t, task.Outputs, 1)
	require.Equal(t, "video", task.Outputs[0].Kind)
	require.Equal(t, "passed", task.Outputs[0].ModerationStatus)
	require.NotNil(t, task.Outputs[0].MediaAssetID)
	require.NotNil(t, task.SettledMicros)
	require.Positive(t, *task.SettledMicros)
	require.Equal(t, firstInput.Source, task.Source)
	var asset struct {
		SourceAIRequestID uuid.UUID
		ObjectKey         string
		DurationMS        int64
	}
	require.NoError(t, db.Raw("SELECT source_ai_request_id,object_key,duration_ms FROM media.media_asset WHERE id=? AND project_id=?",
		*task.Outputs[0].MediaAssetID, project).Scan(&asset).Error)
	require.Equal(t, firstQuote.OperationID, asset.SourceAIRequestID)
	require.EqualValues(t, 4000, asset.DurationMS)
	require.Equal(t, video, objects.files[asset.ObjectKey])
	require.Equal(t, 1, submits)
	require.Equal(t, 1, polls)
}
