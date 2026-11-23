package agentbusiness_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	canvashttp "github.com/StephenQiu30/glowloom-server/backend/creation/canvas/adapter/http"
	pgcanvas "github.com/StephenQiu30/glowloom-server/backend/creation/canvas/adapter/postgres"
	canvas "github.com/StephenQiu30/glowloom-server/backend/creation/canvas/application"
	canvasdomain "github.com/StephenQiu30/glowloom-server/backend/creation/canvas/domain"
	"github.com/StephenQiu30/glowloom-server/backend/creation/evidence"
	evidencepg "github.com/StephenQiu30/glowloom-server/backend/creation/evidence/adapter/postgres"
	workspacehttp "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/http"
	workspace "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application"
	operationhttp "github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/http"
	operation "github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
	operationdomain "github.com/StephenQiu30/glowloom-server/backend/execution/operation/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	appwire "github.com/StephenQiu30/glowloom-server/backend/internal/app"
	agentwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/agent"
	canvaswire "github.com/StephenQiu30/glowloom-server/backend/internal/app/canvas"
	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	workspacewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/workspace"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/media/adapter/postgres"
	media "github.com/StephenQiu30/glowloom-server/backend/media/application"
)

func TestAgentStoryboardRealPostgresYjs(t *testing.T) {
	db, b, router, actor, grant := realCanvasBoundary(t)
	status, created, message := businessToolRequest(t, router, grant, "canvas_create", "storyboard-canvas", map[string]any{"name": "港口初稿"})
	require.Equal(t, 200, status, message)
	var doc canvasdomain.Document
	require.NoError(t, json.Unmarshal(created.Result, &doc))
	require.NotEqual(t, uuid.Nil, doc.ID)

	scope := grant.Scope
	scope.CanvasID = &doc.ID
	admission := domain.Admission{RootRunID: uuid.New(), SessionID: uuid.New(), SubmissionID: uuid.New(), RequestID: uuid.New(),
		ContentHash: domain.Hash([]byte("storyboard")), Scope: scope}
	require.NoError(t, b.Store.AdmitRun(t.Context(), admission))
	grant, err := b.Store.RegisterGrant(t.Context(), domain.GrantRequest{RootRunID: admission.RootRunID, SubmissionID: admission.SubmissionID, OwnerEpoch: 1,
		AuthorizationRevision: 1, LeaseUntil: time.Now().Add(time.Minute)})
	require.NoError(t, err)

	input := map[string]any{"canvas_id": doc.ID, "expected_revision": doc.Revision, "shots": []canvasdomain.StoryboardDraft{
		{Title: "抵岸", Action: "渡轮缓缓靠岸", DurationMS: 4000},
		{Title: "告别", Action: "乘客回头遥望港口", DurationMS: 5000},
	}}
	status, receipt, message := businessToolRequest(t, router, grant, "canvas_storyboard_write", "publish", input)
	require.Equal(t, 200, status, message)
	var first canvas.StoryboardView
	require.NoError(t, json.Unmarshal(receipt.Result, &first))
	require.NotNil(t, first.Board, message)
	require.Len(t, first.Board.Shots, 2)
	require.Equal(t, scope.ProjectID, first.ProjectID)
	require.Greater(t, first.Revision, doc.Revision)

	status, replayed, message := businessToolRequest(t, router, grant, "canvas_storyboard_write", "publish", input)
	require.Equal(t, 200, status, message)
	require.JSONEq(t, string(receipt.Result), string(replayed.Result))

	left, right := first.Board.Shots[0], first.Board.Shots[1]
	status, evidenceReceipt, message := businessToolRequest(t, router, grant, evidence.SearchTool, "locate-shot", evidence.Query{Text: right.ID.String(), Kinds: []string{"storyboard_shot"}})
	require.Equal(t, 200, status, message)
	var search evidence.Result
	require.NoError(t, json.Unmarshal(evidenceReceipt.Result, &search))
	require.Len(t, search.Items, 1)
	require.Equal(t, right.ID, search.Items[0].ID)
	require.Equal(t, &first.Board.ID, search.Items[0].ParentID)
	require.Contains(t, search.Items[0].Excerpt, right.Action)
	checkEvidence := func() error {
		return evidencepg.VerifyModelReceipts(db, scope.ActorID, scope.OrgID, scope.ScopeID, scope.ProjectID, admission.SessionID, []uuid.UUID{evidenceReceipt.ReceiptID})
	}
	require.NoError(t, checkEvidence())

	memory := gin.New()
	memory.Use(func(c *gin.Context) { c.Set("principal", actor) })
	workspacehttp.RegisterProjectMemory(memory.Group("/api"), workspace.NewProjectMemories(workspacewire.ProvideWorkspaceLifecycleStore(db), time.Now))
	memoryPath := "/api/projects/" + scope.ProjectID.String() + "/memory"
	memoryRequest := func(method, suffix string, key uuid.UUID, input any) *httptest.ResponseRecorder {
		t.Helper()
		body, err := json.Marshal(input)
		require.NoError(t, err)
		req := httptest.NewRequestWithContext(t.Context(), method, memoryPath+suffix, bytes.NewReader(body))
		req.Header.Set("Idempotency-Key", key.String())
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		memory.ServeHTTP(response, req)
		return response
	}
	projectMemory := map[string]any{"expected_revision": 0, "statement": "全片无旁白"}
	localMemory := map[string]any{"shot_id": right.ID, "expected_revision": 0, "statement": "只在本镜加入一句独白"}
	baseMemory := memoryRequest(http.MethodPut, "/audio", uuid.New(), projectMemory)
	require.Equal(t, http.StatusOK, baseMemory.Code, baseMemory.Body.String())
	localKey := uuid.New()
	local := memoryRequest(http.MethodPut, "/audio", localKey, localMemory)
	require.Equal(t, http.StatusOK, local.Code, local.Body.String())
	repeatLocal := memoryRequest(http.MethodPut, "/audio", localKey, localMemory)
	require.Equal(t, http.StatusOK, repeatLocal.Code, repeatLocal.Body.String())
	require.JSONEq(t, local.Body.String(), repeatLocal.Body.String())
	reuse := memoryRequest(http.MethodPut, "/audio", localKey, projectMemory)
	require.Equal(t, http.StatusConflict, reuse.Code, reuse.Body.String())
	invalidTarget := memoryRequest(http.MethodGet, "?shot_id="+uuid.NewString(), uuid.Nil, nil)
	require.Equal(t, http.StatusNotFound, invalidTarget.Code, invalidTarget.Body.String())
	globalList := memoryRequest(http.MethodGet, "", uuid.Nil, nil)
	require.Contains(t, globalList.Body.String(), "全片无旁白")
	require.NotContains(t, globalList.Body.String(), "只在本镜加入一句独白")
	localList := memoryRequest(http.MethodGet, "?shot_id="+right.ID.String(), uuid.Nil, nil)
	require.Contains(t, localList.Body.String(), "只在本镜加入一句独白")
	require.NotContains(t, localList.Body.String(), "全片无旁白")

	status, localEvidence, message := businessToolRequest(t, router, grant, evidence.SearchTool, "local-rule", evidence.Query{ShotID: &right.ID})
	require.Equal(t, http.StatusOK, status, message)
	var localSearch evidence.Result
	require.NoError(t, json.Unmarshal(localEvidence.Result, &localSearch))
	require.Len(t, localSearch.Items, 1)
	require.Equal(t, &right.ID, localSearch.Items[0].ParentID)
	require.Equal(t, "只在本镜加入一句独白", localSearch.Items[0].Excerpt)
	status, siblingEvidence, message := businessToolRequest(t, router, grant, evidence.SearchTool, "sibling-rule", evidence.Query{ShotID: &left.ID})
	require.Equal(t, http.StatusOK, status, message)
	require.JSONEq(t, `{"status":"no_match","retrieval_mode":"lexical","items":[]}`, string(siblingEvidence.Result))
	lexical, err := evidencepg.Search(db, scope.ProjectID, evidence.Query{Text: "audio"})
	require.NoError(t, err)
	require.NotEmpty(t, lexical.Items)
	for _, item := range lexical.Items {
		require.NotEqual(t, "shot_decision", item.Kind)
	}
	checkLocal := func() error {
		return evidencepg.VerifyModelReceipts(db, scope.ActorID, scope.OrgID, scope.ScopeID, scope.ProjectID, admission.SessionID, []uuid.UUID{localEvidence.ReceiptID})
	}
	require.NoError(t, checkLocal())

	move := map[string]any{"canvas_id": doc.ID, "board_id": first.Board.ID, "expected_revision": first.Revision, "move": canvasdomain.StoryboardMove{ShotID: right.ID, BeforeID: &left.ID}}
	status, moved, message := businessToolRequest(t, router, grant, "canvas_storyboard_write", "move", move)
	require.Equal(t, 200, status, message)
	var second canvas.StoryboardView
	require.NoError(t, json.Unmarshal(moved.Result, &second))
	require.Equal(t, []canvasdomain.StoryboardShot{right, left}, second.Board.Shots)
	require.NoError(t, checkEvidence(), "reordering preserves evidence")

	edit := map[string]any{"canvas_id": doc.ID, "board_id": first.Board.ID, "expected_revision": second.Revision, "edit": canvasdomain.StoryboardEdit{
		ShotID: right.ID, ExpectedRevision: right.Revision, Field: "action", Value: json.RawMessage(`"她注视渐远的港口"`),
	}}
	status, edited, message := businessToolRequest(t, router, grant, "canvas_storyboard_write", "edit", edit)
	require.Equal(t, 200, status, message)
	var saved canvas.StoryboardView
	require.NoError(t, json.Unmarshal(edited.Result, &saved))
	require.Equal(t, right.ID, saved.Board.Shots[0].ID)
	require.Equal(t, right.Revision+1, saved.Board.Shots[0].Revision)
	require.Equal(t, left, saved.Board.Shots[1])
	require.ErrorIs(t, checkEvidence(), evidencepg.ErrStale)
	status, oldEvidence, message := businessToolRequest(t, router, grant, evidence.ReadTool, "old-shot", search.Items[0].Reference)
	require.Equal(t, 200, status, message)
	var pinned evidence.Result
	require.NoError(t, json.Unmarshal(oldEvidence.Result, &pinned))
	require.Equal(t, "changed", pinned.Status)
	require.Empty(t, pinned.Items)

	require.NoError(t, checkLocal(), "an unrelated shot field update preserves local memory")
	withdraw := map[string]any{"shot_id": right.ID, "expected_revision": 1, "withdraw": true}
	withdrawn := memoryRequest(http.MethodPut, "/audio", uuid.New(), withdraw)
	require.Equal(t, http.StatusOK, withdrawn.Code, withdrawn.Body.String())
	require.ErrorIs(t, checkLocal(), evidencepg.ErrStale)
	localHistory := memoryRequest(http.MethodGet, "/audio?shot_id="+right.ID.String(), uuid.Nil, nil)
	require.Equal(t, http.StatusOK, localHistory.Code, localHistory.Body.String())
	require.Contains(t, localHistory.Body.String(), "只在本镜加入一句独白")
	require.Contains(t, localHistory.Body.String(), `"withdrawn":true`)

	status, _, message = businessToolRequest(t, router, grant, "canvas_storyboard_write", "old-edit", edit)
	require.Equal(t, 409, status, message)

	status, _, message = businessToolRequest(t, router, grant, "canvas_commands", "erase-board", agentwire.CanvasCommandsInput{
		CanvasID: doc.ID, ExpectedRevision: saved.Revision,
		Operations: []canvasdomain.Operation{{Type: "delete_node", NodeID: saved.Board.ID}},
	})
	require.NotEqual(t, 200, status, message)

	status, reopened, message := businessToolRequest(t, router, grant, "canvas_storyboard_read", "reopen", map[string]any{"canvas_id": doc.ID})
	require.Equal(t, 200, status, message)
	require.JSONEq(t, string(edited.Result), string(reopened.Result))
}

type storyboardCandidateFixture struct {
	canvaswire.StoryboardGeneration
	tasks map[uuid.UUID]operation.TaskDetail
}

func (f storyboardCandidateFixture) Task(_ context.Context, _ identity.Principal, _, id uuid.UUID) (operation.TaskDetail, error) {
	task, ok := f.tasks[id]
	if !ok {
		return task, operation.ErrPublicNotFound
	}
	return task, nil
}

// Metadata fixtures hold the provider boundary while the authorized public
// adoption, committed Yjs receipt and media retention use real storage.
func TestStoryboardAdoptionRealPostgresYjs(t *testing.T) {
	db, _, router, actor, grant := realCanvasBoundary(t)
	ctx, project := t.Context(), grant.Scope.ProjectID
	status, created, message := businessToolRequest(t, router, grant, "canvas_create", "adopt-canvas", map[string]any{"name": "采用样片"})
	require.Equal(t, 200, status, message)
	var doc canvasdomain.Document
	require.NoError(t, json.Unmarshal(created.Result, &doc))

	runtimeDB, err := gorm.Open(postgres.Open(os.Getenv("GLOWLOOM_TEST_AGENT_CANVAS_DB_DSN")+"&options=-c%20role%3Dglowloom_app"), &gorm.Config{})
	require.NoError(t, err)
	pool, err := runtimeDB.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = pool.Close() })
	mediaQuery := func(tx *gorm.DB) *mediawire.LegacyAssetAdapter {
		return mediawire.LegacyAssetReader(media.NewAssetQuery(pgmedia.NewScopedStore(tx, mediawire.NewMediaScopeAccess)))
	}
	store := pgcanvas.NewScopedStore(runtimeDB, func(tx *gorm.DB) canvas.MediaReader { return mediaQuery(tx) }, appwire.NewCanvasProjectAccess)
	service := canvas.NewYjsService(store, commandRuntime(t))
	input := canvasdomain.StoryboardCommand{ExpectedRevision: doc.Revision, Shots: []canvasdomain.StoryboardDraft{
		{Title: "追光", Action: "主体沿着光线缓慢移动", DurationMS: 5000},
	}}
	prepared, err := service.PrepareStoryboard(ctx, actor, doc.ID, uuid.New(), input)
	require.NoError(t, err)
	board, err := service.CommitStoryboard(ctx, actor, prepared)
	require.NoError(t, err)
	require.Len(t, board.Board.Shots, 1)
	shot := board.Board.Shots[0]

	require.NoError(t, pgmedia.NewStorageRoutes(db).RegisterProfiles(ctx, []media.StorageProfile{{
		ID: "storyboard-test", Provider: "cos", Endpoint: "http://127.0.0.1:19000",
		SigningEndpoint: "http://127.0.0.1:19000", Region: "us-east-1", Bucket: "storyboard-1250000000",
	}}))
	gateway := storyboardCandidateFixture{tasks: map[uuid.UUID]operation.TaskDetail{}}
	makeCandidate := func(pinned uuid.UUID) (uuid.UUID, uuid.UUID, uuid.UUID) {
		ledger, dispatch := gatewayCreation(t, db, actor, grant, operationhttp.QuoteItemRequest{
			Capability: "video.generate", Mode: "text2video", Prompt: "fixture storyboard video", Params: json.RawMessage(`{}`), OutputCount: 1,
		})
		ledger.settle(t, dispatch, 0)
		operationID, outputID, assetID, location := dispatch.Call.ID, uuid.New(), uuid.New(), uuid.New()
		key := "projects/" + project.String() + "/video/2026/11/" + assetID.String() + ".mp4"
		require.NoError(t, db.Exec(`INSERT INTO media.object_location(id,profile_id,bucket,object_key,state,byte_size,sha256,mime_type)
			VALUES(?,'storyboard-test','storyboard-1250000000',?,'accepted',16,repeat('a',64),'video/mp4')`, location, key).Error)
		require.NoError(t, db.Exec(`INSERT INTO media.media_asset(id,project_id,kind,origin,status,object_key,object_location_id,file_name,mime_type,
			byte_size,sha256,moderation_status,duration_ms,source_ai_request_id,provider_key,model_key,region)
			VALUES(?,?,'video','generated','ready',?,?,'shot.mp4','video/mp4',16,repeat('a',64),'passed',5000,?,'fixture','fixture','domestic')`,
			assetID, project, key, location, operationID).Error)
		gateway.tasks[operationID] = operation.TaskDetail{
			TaskSummary: operation.TaskSummary{ID: operationID, ProjectID: project, Capability: operationdomain.CapabilityVideoGenerate,
				Status: operationdomain.StatusCompleted, Source: &operationdomain.CanvasSource{
					CanvasID: doc.ID, NodeID: board.Board.ID, Revision: board.Revision, ShotID: &pinned, ShotRevision: &shot.Revision,
				}},
			Outputs: []operation.TaskOutput{{ID: outputID, Kind: "video", MediaAssetID: &assetID, ModerationStatus: "passed"}},
		}
		return operationID, outputID, assetID
	}
	firstOperation, firstOutput, firstAsset := makeCandidate(shot.ID)
	secondOperation, secondOutput, secondAsset := makeCandidate(shot.ID)
	wrongOperation, wrongOutput, _ := makeCandidate(uuid.New())

	public := gin.New()
	public.Use(func(c *gin.Context) { c.Set("principal", actor) })
	group := public.Group("/api")
	handler := canvashttp.NewStoryboardHandler(service, nil)
	handler.Register(group)
	canvaswire.RegisterStoryboardDelivery(group, service, nil, gateway, mediaQuery(runtimeDB), func(c *gin.Context, _ error) { c.Status(422) })
	request := func(method, suffix string, key uuid.UUID, input any) *httptest.ResponseRecorder {
		t.Helper()
		body, err := json.Marshal(input)
		require.NoError(t, err)
		r := httptest.NewRequestWithContext(ctx, method, "/api/canvases/"+doc.ID.String()+"/storyboard"+suffix, bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Idempotency-Key", key.String())
		result := httptest.NewRecorder()
		public.ServeHTTP(result, r)
		return result
	}

	target := canvasdomain.StoryboardShotTarget{BoardID: board.Board.ID, ShotID: shot.ID, ExpectedRevision: board.Revision, ShotRevision: shot.Revision}
	wrong := request(http.MethodPost, "/adopt", uuid.New(), canvasdomain.StoryboardAdoption{StoryboardShotTarget: target, OperationID: wrongOperation, OutputID: wrongOutput})
	require.Equal(t, http.StatusConflict, wrong.Code, wrong.Body.String())
	firstRequest := canvasdomain.StoryboardAdoption{StoryboardShotTarget: target, OperationID: firstOperation, OutputID: firstOutput}
	key := uuid.New()
	first := request(http.MethodPost, "/adopt", key, firstRequest)
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	var adopted canvas.StoryboardView
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &adopted))
	require.Equal(t, firstAsset, adopted.Board.Shots[0].Adopted.AssetID)
	require.Equal(t, shot.Revision, adopted.Board.Shots[0].Revision)

	target.ExpectedRevision = adopted.Revision
	reply := request(http.MethodPost, "/adopt", uuid.New(), canvasdomain.StoryboardAdoption{StoryboardShotTarget: target, OperationID: secondOperation, OutputID: secondOutput})
	require.Equal(t, http.StatusOK, reply.Code, reply.Body.String())
	replayed := request(http.MethodPost, "/adopt", key, firstRequest)
	require.Equal(t, http.StatusOK, replayed.Code, replayed.Body.String())
	require.JSONEq(t, first.Body.String(), replayed.Body.String())

	reopened := request(http.MethodGet, "", uuid.Nil, nil)
	require.Equal(t, http.StatusOK, reopened.Code, reopened.Body.String())
	require.JSONEq(t, reply.Body.String(), reopened.Body.String())
	require.NoError(t, json.Unmarshal(reopened.Body.Bytes(), &adopted))
	require.Equal(t, secondAsset, adopted.Board.Shots[0].Adopted.AssetID)
	var changes int64
	require.NoError(t, db.Raw("SELECT count(*) FROM canvas.node WHERE document_id=? AND node_type IN ('timeline','director')", doc.ID).Scan(&changes).Error)
	require.Zero(t, changes)
}
