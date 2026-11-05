package agentbusiness_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	agentwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/agent"
	canvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	canvasdomain "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
	"github.com/StephenQiu30/glowloom-server/backend/tests/identityfixture"
)

func realCanvasBoundary(t *testing.T, runtimes ...canvas.YjsCommandRuntime) (*gorm.DB, *agentwire.Boundary, *gin.Engine, identity.Principal, domain.Grant) {
	t.Helper()
	dsn := os.Getenv("GLOWLOOM_TEST_AGENT_CANVAS_DB_DSN")
	if dsn == "" {
		t.Skip("full isolated migrated PostgreSQL required: GLOWLOOM_TEST_AGENT_CANVAS_DB_DSN")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || !strings.HasSuffix(parsed.Path, "_test") || parsed.Hostname() != "127.0.0.1" {
		t.Fatal("isolated loopback *_test database required")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	actor := identity.Principal{ID: uuid.New(), OrgID: uuid.New(), Role: identitydomain.RoleProducer}
	if err = db.Exec(`INSERT INTO workspace.organization(id,name) VALUES(?,'Agent canvas isolated fixture')`, actor.OrgID).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(`INSERT INTO identity."user"(id,org_id,login_name,display_name,password_hash,role,status,must_change_password)VALUES(?,?,?,'Agent canvas fixture','synthetic','producer','active',false)`, actor.ID, actor.OrgID, "agent-canvas-"+actor.ID.String()).Error; err != nil {
		t.Fatal(err)
	}
	actor = identityfixture.Session(t, db, actor)
	project := uuid.New()
	if err = db.Exec(`INSERT INTO workspace.project(id,org_id,owner_id,owner_scope_id,name,aspect_ratio,style_type)VALUES(?,?,?,?,'Agent canvas fixture','16:9','realistic')`, project, actor.OrgID, actor.ID, actor.ScopeID).Error; err != nil {
		t.Fatal(err)
	}
	var runtime canvas.YjsCommandRuntime = commandRuntime(t)
	if len(runtimes) > 0 {
		runtime = runtimes[0]
	}
	runtimeURL := *parsed
	query := runtimeURL.Query()
	query.Set("options", "-c role=glowloom_app")
	runtimeURL.RawQuery = query.Encode()
	runtimeDB, err := gorm.Open(postgres.Open(runtimeURL.String()), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	runtimeConnection, err := runtimeDB.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtimeConnection.Close() })
	var role string
	if err = runtimeDB.Raw(`SELECT current_user`).Scan(&role).Error; err != nil || role != "glowloom_app" {
		t.Fatal("restricted Go business role required", role, err)
	}
	b := agentwire.NewBoundary(runtimeDB, nil).WithCanvasRuntime(runtime)
	admission := domain.Admission{RootRunID: uuid.New(), SessionID: uuid.New(), SubmissionID: uuid.New(), RequestID: uuid.New(), ContentHash: domain.Hash([]byte(`{"goal":"actual canvas"}`)), Scope: domain.Scope{ActorID: actor.ID, OrgID: actor.OrgID, ScopeID: actor.ScopeID, ProjectID: project}, BudgetMicros: 100}
	if err = b.Store.AdmitRun(t.Context(), admission); err != nil {
		t.Fatal(err)
	}
	grant, err := b.Store.RegisterGrant(t.Context(), domain.GrantRequest{RootRunID: admission.RootRunID, SubmissionID: admission.SubmissionID, OwnerEpoch: 1, AuthorizationRevision: 1, LeaseUntil: time.Now().Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	b.BusinessHandler(businessToken).Register(router)
	return db, b, router, actor, grant
}
func businessToolRequest(t *testing.T, router http.Handler, g domain.Grant, name, callID string, input any) (int, domain.CallReceipt, string) {
	t.Helper()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	call := domain.CallInput{Grant: g, Call: domain.CallIdentity{TaskID: "real-pg-task", CallID: callID, CheckpointID: "committed-boundary", Attempt: 1, ContractVersion: "v1", InputHash: domain.Hash(raw)}, Input: raw, MaxMicros: 1_000_000}
	body, err := json.Marshal(call)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequestWithContext(t.Context(), "POST", "/internal/agent-business/tools/"+name+"/execute", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+businessToken)
	request.Header.Set("X-Agent-Audience", "agent-business")
	out := httptest.NewRecorder()
	router.ServeHTTP(out, request)
	var receipt domain.CallReceipt
	if out.Code == 200 && json.Unmarshal(out.Body.Bytes(), &receipt) != nil {
		t.Fatal(out.Body.String())
	}
	return out.Code, receipt, out.Body.String()
}
func realDirectorNode() canvasdomain.Node {
	camera, shot := uuid.New(), uuid.New()
	transform := canvasdomain.DirectorTransform{Position: []float64{0, 0, 0}, Rotation: []float64{0, 0, 0}, Scale: []float64{1, 1, 1}}
	return canvasdomain.Node{ID: uuid.New(), Title: "导演镜头", NodeType: "director", NodeAction: "tool", Config: canvasdomain.NodeConfig{Director: &canvasdomain.DirectorConfig{ID: uuid.New(), Version: 1, Title: "雨夜场景", Background: "#ffffff", EnvironmentIntensity: 1, ActiveShotID: shot, Objects: []canvasdomain.DirectorObject{{ID: uuid.New(), Name: "演员", Kind: "actor", BuiltinActor: "mannequin", Transform: transform, Color: "#aaaaaa", Pose: "stand", Visible: true, Keyframes: []canvasdomain.DirectorKeyframe{}}}, Cameras: []canvasdomain.DirectorCamera{{ID: camera, Name: "摄影机", Transform: transform, Target: []float64{0, 1, 0}, FocalLength: 35, FOV: 54, Aperture: 2.8, FocusDistance: 5, Near: .1, Far: 100, Keyframes: []canvasdomain.DirectorKeyframe{}}}, Lights: []canvasdomain.DirectorLight{}, Shots: []canvasdomain.DirectorShot{{ID: shot, Name: "镜头1", CameraID: camera, Duration: 5, FPS: 25, ShotSize: "medium", CameraMove: "static"}}}}}
}
func TestAgentCanvasCommandsRealPostgresYjsReplayConflictAndMediaAdoption(t *testing.T) {
	db, b, router, actor, g := realCanvasBoundary(t)
	status, created, message := businessToolRequest(t, router, g, "canvas_create", "create", map[string]any{"name": "Agent实际创作"})
	if status != 200 {
		t.Fatalf("create status %d %s", status, message)
	}
	var document canvasdomain.Document
	if json.Unmarshal(created.Result, &document) != nil || document.ID == uuid.Nil {
		t.Fatal(string(created.Result))
	}
	text := "雨夜，主角走入街道。"
	story := canvasdomain.Node{ID: uuid.New(), Title: "分镜1", NodeType: "text", NodeAction: "resource", Config: canvasdomain.NodeConfig{Text: &text}}
	director := realDirectorNode()
	track := uuid.New()
	timeline := canvasdomain.Node{ID: uuid.New(), Title: "时间线", NodeType: "timeline", NodeAction: "tool", Config: canvasdomain.NodeConfig{Timeline: &canvasdomain.TimelineConfig{Version: 1, AspectRatio: "16:9", FPS: 25, Tracks: []canvasdomain.TimelineTrack{{ID: track, Kind: "text", Label: "字幕", Visible: true}}, Clips: []canvasdomain.TimelineClip{{ID: uuid.New(), TrackID: track, Kind: "text", Title: "第一幕", DurationMS: 1000, Volume: 1, Text: "雨夜"}}, SubtitleStyle: canvasdomain.SubtitleStyle{FontSize: 32, Color: "#ffffff", Position: "bottom"}}}}
	input := agentwire.CanvasCommandsInput{CanvasID: document.ID, ExpectedRevision: document.Revision, Operations: []canvasdomain.Operation{{Type: "create_node", Node: &story}, {Type: "create_node", Node: &director}, {Type: "create_node", Node: &timeline}}}
	status, first, message := businessToolRequest(t, router, g, "canvas_commands", "structure", input)
	if status != 200 {
		t.Fatalf("structure status %d %s", status, message)
	}
	var result struct {
		Status   string          `json:"status"`
		Revision canvas.Revision `json:"revision"`
	}
	if err := json.Unmarshal(first.Result, &result); err != nil || result.Status != "accepted" {
		t.Fatal(string(first.Result), err)
	}
	status, replayed, message := businessToolRequest(t, router, g, "canvas_commands", "structure", input)
	if status != 200 || replayed.ReceiptID != first.ReceiptID {
		t.Fatal("replay", message)
	}
	var counts struct{ Nodes, Receipts, Commits int64 }
	if err := db.Raw(`SELECT (SELECT count(*) FROM canvas.node WHERE document_id=? AND NOT is_delete) nodes,(SELECT count(*) FROM canvas.collab_receipt WHERE document_id=?) receipts,(SELECT commit_seq FROM canvas.collab_document WHERE document_id=?) commits`, document.ID, document.ID, document.ID).Scan(&counts).Error; err != nil || counts.Nodes != 3 || counts.Receipts != 1 || counts.Commits != 1 {
		t.Fatal(counts, err)
	}
	status, _, _ = businessToolRequest(t, router, g, "canvas_commands", "stale", input)
	if status != 409 {
		t.Fatalf("stale command status %d", status)
	}
	// Primitive camera/shot and timeline fields use the same closed operation
	// schema as object-valued configuration; they must not be advertised as object-only.
	for _, tool := range b.Capabilities.Tools {
		if tool.Descriptor.Name != "canvas_commands" {
			continue
		}
		var schema map[string]any
		if json.Unmarshal(tool.Descriptor.Parameters, &schema) != nil {
			t.Fatal("tool schema")
		}
		value := schema["properties"].(map[string]any)["operations"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)["value"].(map[string]any)
		if value["type"] == "object" {
			t.Fatal("primitive canvas operation advertised as object only")
		}
	}
	edit := agentwire.CanvasCommandsInput{CanvasID: document.ID, ExpectedRevision: int64(result.Revision), Operations: []canvasdomain.Operation{{Type: "set", NodeID: director.ID, Path: []string{"config", "director", "shots", "@" + director.Config.Director.Shots[0].ID.String(), "name"}, Value: json.RawMessage(`"已采用镜头"`)}, {Type: "set", NodeID: timeline.ID, Path: []string{"config", "timeline", "snapping"}, Value: json.RawMessage(`true`)}}}
	status, edited, message := businessToolRequest(t, router, g, "canvas_commands", "edit-structure", edit)
	if status != 200 {
		t.Fatalf("primitive structure edit %d %s", status, message)
	}
	if err := json.Unmarshal(edited.Result, &result); err != nil || result.Status != "accepted" {
		t.Fatal(string(edited.Result), err)
	}
	var editedName string
	if err := db.Raw(`SELECT config->'director'->'shots'->0->>'name' FROM canvas.node WHERE id=?`, director.ID).Scan(&editedName).Error; err != nil || editedName != "已采用镜头" {
		t.Fatal("actual adopted shot edit", editedName, err)
	}
	// A real media record is a fixture; adoption still runs the production owner
	// authorization/ready/moderation check in the final Yjs commit transaction.
	asset, location := uuid.New(), uuid.New()
	profile := "agent-canvas-" + uuid.NewString()
	bucket := "agent-canvas-" + asset.String()[:8]
	key := "projects/" + g.Scope.ProjectID.String() + "/image/2026/11/" + asset.String() + ".png"
	if err := db.Exec(`INSERT INTO media.storage_profile(id,provider,endpoint,signing_endpoint,region,bucket,addressing_style,resource_identity)VALUES(?,'minio','http://127.0.0.1:1','http://127.0.0.1:1','us-east-1',?,'path',?)`, profile, bucket, domain.ContentHash(profile)).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO media.object_location(id,profile_id,bucket,object_key,state,byte_size,sha256,mime_type)VALUES(?,?,?,?,'accepted',1,repeat('a',64),'image/png')`, location, profile, bucket, key).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO media.media_asset(id,project_id,kind,origin,status,object_key,object_location_id,file_name,mime_type,byte_size,sha256,moderation_status)VALUES(?,?,'image','upload','ready',?,?,'fixture.png','image/png',1,repeat('a',64),'passed')`, asset, g.Scope.ProjectID, key, location).Error; err != nil {
		t.Fatal(err)
	}
	image := canvasdomain.Node{ID: uuid.New(), Title: "已归档真实素材", NodeType: "image", NodeAction: "resource", RefType: "media_asset", RefID: &asset}
	adopt := agentwire.CanvasCommandsInput{CanvasID: document.ID, ExpectedRevision: int64(result.Revision), Operations: []canvasdomain.Operation{{Type: "create_node", Node: &image}}}
	status, adopted, message := businessToolRequest(t, router, g, "canvas_commands", "adopt", adopt)
	if status != 200 {
		raw, _ := json.Marshal(adopt)
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Request = httptest.NewRequestWithContext(t.Context(), "POST", "/", nil)
		_, cause := b.AtomicTool(ctx, "canvas_commands", domain.CallInput{Grant: g, Call: domain.CallIdentity{TaskID: "real-pg-task", CallID: "adopt", CheckpointID: "committed-boundary", Attempt: 1, ContractVersion: "v1", InputHash: domain.Hash(raw)}, Input: raw, MaxMicros: 1_000_000})
		t.Fatalf("adoption %d %s cause %v", status, message, cause)
	}
	if err := json.Unmarshal(adopted.Result, &result); err != nil || result.Status != "accepted" {
		t.Fatal(string(adopted.Result), err)
	}
	var refs int64
	if err := db.Raw(`SELECT count(*) FROM canvas.node WHERE document_id=? AND ref_id=? AND NOT is_delete`, document.ID, asset).Scan(&refs).Error; err != nil || refs != 1 {
		t.Fatal("actual archived media binding", refs, err)
	}
	if _, err := b.Store.Decide(t.Context(), actor, g.Scope.ProjectID, g.RootRunID, domain.DecisionInput{RequestID: uuid.New(), Action: "stop", ExpectedAuthorizationRevision: 1}); err != nil {
		t.Fatal(err)
	}
	adopt.ExpectedRevision = int64(result.Revision)
	adopt.Operations[0].Node = &canvasdomain.Node{ID: uuid.New(), Title: "late", NodeType: "text", NodeAction: "resource", Config: canvasdomain.NodeConfig{Text: &text}}
	status, _, _ = businessToolRequest(t, router, g, "canvas_commands", "after-stop", adopt)
	if status != 409 {
		t.Fatalf("stop admitted late command %d", status)
	}
}

type pausedCommandRuntime struct {
	canvas.YjsCommandRuntime
	entered, release chan struct{}
}

func (r pausedCommandRuntime) Commands(ctx context.Context, s canvas.YjsState, id uuid.UUID, p canvas.DocumentDTO) (canvas.YjsCandidate, error) {
	candidate, err := r.YjsCommandRuntime.Commands(ctx, s, id, p)
	if err != nil {
		return candidate, err
	}
	close(r.entered)
	select {
	case <-r.release:
		return candidate, nil
	case <-ctx.Done():
		return canvas.YjsCandidate{}, ctx.Err()
	}
}
func TestStopDuringRealYjsPreparationPreventsAllCanvasAndReceiptEffects(t *testing.T) {
	paused := pausedCommandRuntime{YjsCommandRuntime: commandRuntime(t), entered: make(chan struct{}), release: make(chan struct{})}
	db, b, router, actor, g := realCanvasBoundary(t, paused)
	status, created, message := businessToolRequest(t, router, g, "canvas_create", "create", map[string]any{"name": "Stop preparation fixture"})
	if status != 200 {
		t.Fatal(message)
	}
	var document canvasdomain.Document
	if json.Unmarshal(created.Result, &document) != nil {
		t.Fatal(message)
	}
	text := "must not publish"
	input := agentwire.CanvasCommandsInput{CanvasID: document.ID, ExpectedRevision: document.Revision, Operations: []canvasdomain.Operation{{Type: "create_node", Node: &canvasdomain.Node{ID: uuid.New(), NodeType: "text", NodeAction: "resource", Title: "late storyboard", Config: canvasdomain.NodeConfig{Text: &text}}}}}
	returned := make(chan int, 1)
	go func() {
		status, _, _ := businessToolRequest(t, router, g, "canvas_commands", "preparing", input)
		returned <- status
	}()
	select {
	case <-paused.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("real command did not prepare")
	}
	if _, err := b.Store.Decide(t.Context(), actor, g.Scope.ProjectID, g.RootRunID, domain.DecisionInput{RequestID: uuid.New(), Action: "stop", ExpectedAuthorizationRevision: 1}); err != nil {
		t.Fatal(err)
	}
	close(paused.release)
	if status := <-returned; status != 409 {
		t.Fatalf("Stop admitted prepared write %d", status)
	}
	var count int64
	for _, query := range []string{`SELECT count(*) FROM canvas.node WHERE document_id=?`, `SELECT count(*) FROM canvas.collab_document WHERE document_id=?`, `SELECT count(*) FROM canvas.collab_receipt WHERE document_id=?`} {
		if err := db.Raw(query, document.ID).Scan(&count).Error; err != nil || count != 0 {
			t.Fatal("prepared side effect survived Stop", count, err)
		}
	}
	if err := db.Raw(`SELECT count(*) FROM agent_business.call_receipt WHERE root_run_id=? AND call_id='preparing'`, g.RootRunID).Scan(&count).Error; err != nil || count != 0 {
		t.Fatal("uncommitted command receipt survived", count, err)
	}
}
