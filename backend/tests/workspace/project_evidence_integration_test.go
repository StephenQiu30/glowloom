package workspace_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	agent "github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	agentwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/agent"
	"github.com/StephenQiu30/glowloom-server/backend/internal/creation/evidence"
	evidencepg "github.com/StephenQiu30/glowloom-server/backend/internal/creation/evidence/adapter/postgres"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

func TestProjectEvidenceHTTPResolvesOnlyCurrentProjectSources(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	actor, project := lifecycleActorProject(ctx, t, db)
	_, other := lifecycleActorProject(ctx, t, db)
	first := uuid.New()
	var nodes []uuid.UUID
	for _, projectID := range []uuid.UUID{project, other} {
		canvas := uuid.New()
		lifecycleFixtureSQL(t, db, `INSERT INTO canvas.document(id,project_id,name) VALUES(?,?,'画布')`, canvas, projectID)
		for range 2 {
			node := uuid.New()
			lifecycleFixtureSQL(t, db, `INSERT INTO canvas.node(id,document_id,node_type,title,x,y,config)
			VALUES(?,?,'text','开场镜头',0,0,'{"text":"乘客握住潮湿栏杆"}')`, node, canvas)
			if projectID == project {
				nodes = append(nodes, node)
			}
			first = node
		}
	}
	b := agentwire.NewBoundary(db, nil)
	scope := agent.Scope{ActorID: actor.ID, OrgID: actor.OrgID, ScopeID: actor.ScopeID, ProjectID: project}
	run := agent.Admission{RootRunID: uuid.New(), SessionID: uuid.New(), SubmissionID: uuid.New(), RequestID: uuid.New(), ContentHash: agent.Hash([]byte("evidence")), Scope: scope}
	if err := b.Store.AdmitRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	g, err := b.Store.RegisterGrant(ctx, agent.GrantRequest{RootRunID: run.RootRunID, SubmissionID: run.SubmissionID, OwnerEpoch: 1, AuthorizationRevision: 1, LeaseUntil: time.Now().Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	token := "isolated-evidence-business-service-token"
	b.BusinessHandler(token).Register(router)
	invoke := func(name string, input any) (evidence.Result, uuid.UUID) {
		t.Helper()
		raw, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		c := agent.CallInput{Grant: g, Call: agent.CallIdentity{TaskID: "evidence", CallID: uuid.NewString(), CheckpointID: "proof", Attempt: 1, ContractVersion: "v1", InputHash: agent.Hash(raw)}, Input: raw}
		body, err := json.Marshal(c)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/internal/agent-business/tools/"+name+"/execute", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Agent-Audience", "agent-business")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		var receipt agent.CallReceipt
		if res.Code != 200 || json.Unmarshal(res.Body.Bytes(), &receipt) != nil {
			t.Fatalf("tool %s: %d %s", name, res.Code, res.Body)
		}
		var result evidence.Result
		if receipt.Status != "completed" || json.Unmarshal(receipt.Result, &result) != nil {
			t.Fatalf("invalid tool result %s", receipt.Result)
		}
		return result, receipt.ReceiptID
	}
	lifecycleFixtureSQL(t, db, `UPDATE canvas.node SET config=jsonb_build_object('text',repeat('长镜头描述',600)) WHERE id=?`, nodes[1])
	result, receipt := invoke(evidence.SearchTool, evidence.Query{Text: "开场镜头", Kinds: []string{"canvas_node"}, Limit: 2})
	if result.Status != "ambiguous" || len(result.Items) != 2 {
		t.Fatal(result)
	}
	var ref evidence.Reference
	for _, item := range result.Items {
		if item.ID != nodes[0] && item.ID != nodes[1] {
			t.Fatalf("foreign node leaked: %+v", item)
		}
		if item.ID == nodes[0] && item.Excerpt == "乘客握住潮湿栏杆" {
			ref = item.Reference
		}
	}
	if ref.ID == uuid.Nil {
		t.Fatal("the short named node was not retrieved")
	}
	narrow, _ := invoke(evidence.SearchTool, evidence.Query{Text: "开场镜头", Kinds: []string{"canvas_node"}, Limit: 1})
	if narrow.Status != "ambiguous" || len(narrow.Items) != 1 {
		t.Fatal(narrow)
	}
	foreign, _ := invoke(evidence.ReadTool, evidence.Reference{Kind: "canvas_node", ID: first})
	if foreign.Status != "unavailable" || len(foreign.Items) > 0 {
		t.Fatal(foreign)
	}
	found, _ := invoke(evidence.ReadTool, ref)
	if found.Status != "ok" || len(found.Items) != 1 || found.Items[0].Reference != ref {
		t.Fatal(found)
	}
	verify := func(session uuid.UUID) error {
		return evidencepg.VerifyModelReceipts(db, actor.ID, actor.OrgID, actor.ScopeID, project, session, []uuid.UUID{receipt})
	}
	if err := verify(run.SessionID); err != nil {
		t.Fatal(err)
	}
	if err := verify(uuid.New()); !errors.Is(err, evidencepg.ErrStale) {
		t.Fatalf("foreign session accepted: %v", err)
	}
	lifecycleFixtureSQL(t, db, "UPDATE canvas.node SET title='已更新', content_version=2 WHERE id=?", ref.ID)
	changed, _ := invoke(evidence.ReadTool, ref)
	if changed.Status != "changed" || len(changed.Items) != 0 {
		t.Fatal(changed)
	}
	if err := verify(run.SessionID); !errors.Is(err, evidencepg.ErrStale) {
		t.Fatalf("old receipt passed admission: %v", err)
	}
	lifecycleFixtureSQL(t, db, "UPDATE canvas.node SET is_delete=true WHERE id=?", ref.ID)
	removed, _ := invoke(evidence.ReadTool, ref)
	if removed.Status != "unavailable" || len(removed.Items) != 0 {
		t.Fatal(removed)
	}

	docID, nodeID, assetID, asrID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	assetKey := "evidence/" + assetID.String() + ".wav"
	const endpoint = "https://evidence-fixture-1250000000.cos.ap-shanghai.myqcloud.com"
	profile := mediaapp.StorageProfile{ID: "evidence-fixture", Provider: "cos", Endpoint: endpoint,
		SigningEndpoint: endpoint, Bucket: "evidence-fixture-1250000000", Region: "ap-shanghai"}
	if err := pgmedia.NewStorageRoutes(db).RegisterProfiles(ctx, []mediaapp.StorageProfile{profile}); err != nil {
		t.Fatal(err)
	}
	location := uuid.New()
	lifecycleFixtureSQL(t, db, "INSERT INTO media.object_location(id,profile_id,bucket,object_key,state) VALUES(?,?,?,?,'accepted')",
		location, profile.ID, profile.Bucket, assetKey)
	lifecycleFixtureSQL(t, db, `INSERT INTO media.media_asset (id,project_id,kind,origin,status,object_key,file_name,mime_type,byte_size,sha256,duration_ms,moderation_status,object_location_id)
		VALUES (?,?,'audio','upload','ready',?,'渡轮汽笛.wav','audio/wav',120,repeat('a',64),3000,'passed',?)`,
		assetID, project, assetKey, location)
	lifecycleFixtureSQL(t, db, "INSERT INTO canvas.document(id,project_id,name) VALUES(?,?,'声音')", docID, project)
	lifecycleFixtureSQL(t, db, "INSERT INTO canvas.node(id,document_id,node_type,ref_type,ref_id,x,y) VALUES(?,?,'audio','media_asset',?,0,0)", nodeID, docID, assetID)
	lifecycleFixtureSQL(t, db, `INSERT INTO mediatool.transcription_job (id,project_id,org_id,actor_id,canvas_id,node_id,source_revision,language,frozen,status,stage,progress,attempt,revision,inference_state,result,result_sha256,created_at,updated_at)
		VALUES(?,?,?,?,?,?,1,'zh',jsonb_build_object('input',jsonb_build_object('asset_id',?::uuid,'revision',1,'sha256',repeat('a',64),'kind','audio')),
		'succeeded','complete',100,1,2,'terminal','{"version":1,"language":"zh","duration_ms":3000,"segments":[{"start_ms":1000,"end_ms":2600,"text":"听见一声低沉汽笛"}]}',repeat('b',64),now(),now())`,
		asrID, project, actor.OrgID, actor.ID, docID, nodeID, assetID)
	asr, _ := invoke(evidence.SearchTool, evidence.Query{Text: "低沉汽笛", Kinds: []string{"transcript"}})
	if len(asr.Items) != 1 || !asr.Items[0].Draft || asr.Items[0].Start == nil || asr.Items[0].End == nil ||
		*asr.Items[0].Start != 1000 || *asr.Items[0].End != 2600 || asr.Items[0].PositionUnit != "millisecond" {
		t.Fatal(asr)
	}
	lifecycleFixtureSQL(t, db, "UPDATE media.media_asset SET revision=2 WHERE id=?", assetID)
	expired, _ := invoke(evidence.ReadTool, asr.Items[0].Reference)
	if expired.Status != "unavailable" || len(expired.Items) != 0 {
		t.Fatal(expired)
	}
}
