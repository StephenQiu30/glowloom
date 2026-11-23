package workspace_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/creation/evidence"
	ep "github.com/StephenQiu30/glowloom-server/backend/creation/evidence/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application"
	catalog "github.com/StephenQiu30/glowloom-server/backend/execution/catalog/application"
	gp "github.com/StephenQiu30/glowloom-server/backend/execution/gateway/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/adapter/provider"
	gd "github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	agent "github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	agentwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/agent"
	"github.com/StephenQiu30/glowloom-server/backend/internal/app/aigateway"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

type arkKeys struct{}

func (arkKeys) Open(string, uuid.UUID, uuid.UUID, []byte) ([]byte, error) {
	return []byte(`{"api_key":"fixture"}`), nil
}

type embeddingTransport func(*http.Request) (*http.Response, error)

func (f embeddingTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestProjectRAGIndexesSearchesAndFencesOriginalReceipts(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	actor, project := lifecycleActorProject(ctx, t, db)
	_, other := lifecycleActorProject(ctx, t, db)
	var target uuid.UUID
	for _, id := range []uuid.UUID{project, other} {
		canvas, node := uuid.New(), uuid.New()
		if id == project {
			target = node
		}
		lifecycleFixtureSQL(t, db, "INSERT INTO canvas.document(id,project_id,name) VALUES(?,?,'创作')", canvas, id)
		lifecycleFixtureSQL(t, db, `INSERT INTO canvas.node(id,document_id,node_type,title,x,y,config) VALUES(?,?,'text','晨渡',0,0,'{"text":"轮船穿过浓雾，码头渐显"}')`, node, canvas)
	}
	admin := gp.New(db, actor, func(context.Context, *gorm.DB, identity.Principal, uuid.UUID, uuid.UUID, bool) error { return nil })
	price := gd.Price{Input: 1000, Currency: "CNY", FX: "1", Multiplier: "1", Discount: "1"}
	model := gd.Model{ID: uuid.New(), Name: "rag-fixture-" + project.String(), Protocol: gd.EmbeddingProtocol, Capability: "text.embed", MaxInput: 16384, MaxOutput: 1, Price: price, PurchaseCap: 10000}
	route := gd.Route{ID: uuid.New(), Group: project.String(), ModelID: model.ID, Supplier: uuid.New(), Credential: uuid.New(), Protocol: model.Protocol, BaseURL: gd.EmbeddingBaseURL, UpstreamModel: gd.EmbeddingModel, Purchase: price}
	require.NoError(t, admin.PublishModel(ctx, model))
	models := aigateway.Catalog{DB: db}
	list := func(capability string) []catalog.ModelCatalogItem {
		t.Helper()
		page, err := models.ListModelsForProject(ctx, actor, catalog.ListModelsInput{ProjectID: project, ModelKey: model.Name, Capability: capability})
		require.NoError(t, err)
		return page.Models
	}
	defaults := workspaceapp.NewModelDefaultsService(aigateway.Defaults{Runtime: &aigateway.Runtime{DB: db}}, time.Now)
	selectDefault := func(name string) error {
		current, err := defaults.Read(ctx, actor, project)
		require.NoError(t, err)
		if name == "" {
			delete(current.DefaultModels, "text.embed")
		} else {
			current.DefaultModels["text.embed"] = name
		}
		_, err = defaults.Save(ctx, actor, workspaceapp.ModelDefaultsChange{ProjectID: project, ExpectedRevision: current.Revision, DefaultModels: current.DefaultModels, IdempotencyKey: uuid.New(), RequestID: uuid.NewString()})
		return err
	}
	require.Empty(t, list("text.embed"))
	require.ErrorIs(t, selectDefault(model.Name), workspaceapp.ErrDefaultModelUnavailable)
	require.NoError(t, admin.PublishRoute(ctx, route, []byte("fixture"), "fixture", 8, 300))
	available := list("text.embed")
	require.Len(t, available, 1)
	require.Equal(t, "text.embed", available[0].Capability)
	require.Equal(t, model.ID, available[0].ReleaseID)
	require.True(t, available[0].Available)
	require.Equal(t, route.ID, available[0].ConfigVersionID)
	require.Equal(t, route.Credential, available[0].CredentialID)
	require.Empty(t, available[0].CurrentVersion.Modes)
	require.JSONEq(t, "[]", string(available[0].CurrentVersion.ParamSchema))
	require.Empty(t, list("text.generate"))
	require.ErrorIs(t, aigateway.ConfigureEvidence(ctx, db, project, actor.ID, 12, true), gd.ErrUnavailable)
	require.NoError(t, selectDefault(available[0].Key))
	require.NoError(t, admin.Grant(ctx, actor.ScopeID, project.String(), 100))
	sends := 0
	invalidReply := false
	transport := embeddingTransport(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, "ark.cn-beijing.volces.com", req.Host)
		require.Equal(t, "/api/v3/embeddings/multimodal", req.URL.Path)
		require.Equal(t, "Bearer fixture", req.Header.Get("Authorization"))
		var in struct {
			Model, Instructions string
			Dimensions          int
			Input               []struct{ Type, Text string }
		}
		require.NoError(t, json.NewDecoder(req.Body).Decode(&in))
		require.Len(t, in.Input, 1)
		require.Equal(t, "text", in.Input[0].Type)
		require.Equal(t, gd.EmbeddingDimensions, in.Dimensions)
		require.Equal(t, gd.EmbeddingModel, in.Model)
		vector := make([]float32, in.Dimensions)
		if strings.Contains(in.Input[0].Text, "浓雾") || in.Input[0].Text == "看不清远岸的那幕" {
			vector[0] = 1
		} else {
			vector[1] = 1
		}
		if in.Input[0].Text == "看不清远岸的那幕" || in.Input[0].Text == "完全无关" {
			require.Contains(t, in.Instructions, "Target_modality")
		} else {
			require.Contains(t, in.Instructions, "Compress")
		}
		sends++
		if invalidReply {
			vector = vector[:3]
		}
		body, _ := json.Marshal(map[string]any{"model": in.Model, "id": uuid.NewString(), "data": map[string]any{"embedding": vector}, "usage": map[string]int{"prompt_tokens": 22, "total_tokens": 22}})
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body)), Header: http.Header{}}, nil
	})
	runtime := &aigateway.Runtime{DB: db, Provider: provider.New(arkKeys{}, &http.Client{Transport: transport}).WithLookup(func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	})}
	require.NoError(t, aigateway.ConfigureEvidence(ctx, db, project, actor.ID, 12, true))
	var policy ep.Policy
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SET LOCAL ROLE glowloom_app").Error; err != nil {
			return err
		}
		var err error
		policy, err = ep.ReadPolicy(tx, project)
		return err
	}))
	require.Equal(t, model.ID, policy.ReleaseID)
	store := taskruntime.NewStore(db).ForBusiness(policy.Generation.String())
	job, err := store.Claim(ctx, []string{ep.IndexTask}, "rag-test", time.Minute)
	require.NoError(t, err)
	require.NotNil(t, job)
	for range 3 {
		_, err = runtime.IndexEvidence(ctx, *job)
		require.NoError(t, err)
	}
	require.Equal(t, 2, sends, "one project brief and one node encoded once")
	b := agentwire.NewBoundary(db, nil).WithGateway(runtime)
	scope := agent.Scope{ActorID: actor.ID, OrgID: actor.OrgID, ScopeID: actor.ScopeID, ProjectID: project}
	run := agent.Admission{RootRunID: uuid.New(), SessionID: uuid.New(), SubmissionID: uuid.New(), RequestID: uuid.New(), ContentHash: agent.Hash([]byte("rag")), Scope: scope}
	require.NoError(t, b.Store.AdmitRun(ctx, run))
	grant, err := b.Store.RegisterGrant(ctx, agent.GrantRequest{RootRunID: run.RootRunID, SubmissionID: run.SubmissionID, OwnerEpoch: 1, AuthorizationRevision: 1, LeaseUntil: time.Now().Add(time.Minute)})
	require.NoError(t, err)
	token := "isolated-project-rag-business-token"
	router := gin.New()
	b.BusinessHandler(token).Register(router)
	invoke := func(query, id string) (int, evidence.Result) {
		raw, _ := json.Marshal(evidence.Query{Text: query, Kinds: []string{"canvas_node"}})
		in := agent.CallInput{Grant: grant, Call: agent.CallIdentity{TaskID: "rag", CallID: id, CheckpointID: "verified", Attempt: 1, ContractVersion: "v1", InputHash: agent.Hash(raw)}, Input: raw}
		body, _ := json.Marshal(in)
		req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/internal/agent-business/tools/"+evidence.SearchTool+"/execute", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Agent-Audience", "agent-business")
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		var result evidence.Result
		if w.Code == 200 {
			var receipt agent.CallReceipt
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &receipt))
			require.Equal(t, agent.CallStatusCompleted, receipt.Status)
			require.NoError(t, json.Unmarshal(receipt.Result, &result))
		}
		return w.Code, result
	}
	check := func(query, id string, want bool) {
		t.Helper()
		status, result := invoke(query, id)
		require.Equal(t, 200, status)
		if want {
			require.Len(t, result.Items, 1)
			require.Equal(t, target, result.Items[0].ID)
		} else {
			require.Empty(t, result.Items)
		}
	}
	check("晨渡", "exact", true)
	require.Equal(t, 2, sends)
	check("看不清远岸的那幕", "semantic", true)
	check("看不清远岸的那幕", "semantic", true)
	require.Equal(t, 3, sends)
	check("完全无关", "negative", false)
	var charged int64
	require.NoError(t, db.Raw("SELECT spent FROM ai.budget WHERE id=?", policy.BudgetID).Row().Scan(&charged))
	require.EqualValues(t, 4, charged)
	require.NoError(t, aigateway.ConfigureEvidence(ctx, db, project, actor.ID, charged, true))
	check("看不清远岸的那幕", "over-budget", false)
	require.Equal(t, 4, sends)
	require.NoError(t, aigateway.ConfigureEvidence(ctx, db, project, actor.ID, 12, true))
	var label evidence.Reference
	require.NoError(t, db.Raw("SELECT kind,id,part,version FROM workspace.current_project_evidence WHERE project_id=? AND id=?", project, target).Scan(&label).Error)
	cases := []aigateway.EvidenceCase{
		{ID: "description", Query: evidence.Query{Text: "看不清远岸的那幕", Kinds: []string{"canvas_node"}}, Expected: []evidence.Reference{label}},
		{ID: "no-answer", Query: evidence.Query{Text: "完全无关", Kinds: []string{"canvas_node"}}, Expected: []evidence.Reference{}},
	}
	trial := uuid.New()
	metrics, err := runtime.EvaluateEvidence(ctx, project, actor.ID, trial, cases)
	require.NoError(t, err)
	require.Equal(t, 2, metrics.Cases)
	require.EqualValues(t, 2, metrics.Charged)
	require.Equal(t, 0.0, *metrics.Lexical.RecallAt6)
	require.Equal(t, 1.0, *metrics.Hybrid.RecallAt6)
	require.Equal(t, 1.0, *metrics.Hybrid.MRR)
	require.Equal(t, 0.0, *metrics.Hybrid.NoAnswerFalsePositiveRate)
	_, err = runtime.EvaluateEvidence(ctx, project, actor.ID, trial, cases)
	require.NoError(t, err)
	require.Equal(t, 6, sends)
	lifecycleFixtureSQL(t, db, "UPDATE canvas.node SET is_delete=true WHERE id=?", target)
	status, _ := invoke("看不清远岸的那幕", "semantic")
	require.NotEqual(t, 200, status)
	raw, _ := json.Marshal(evidence.Query{Text: "看不清远岸的那幕", Kinds: []string{"canvas_node"}})
	replay, err := b.Store.ReadCall(ctx, run.RootRunID, run.SubmissionID, agent.CallIdentity{TaskID: "rag", CallID: "semantic", CheckpointID: "verified", Attempt: 1, ContractVersion: "v1", InputHash: agent.Hash(raw)})
	require.ErrorIs(t, err, agent.ErrConflict)
	require.Empty(t, replay.Result)
	_, err = runtime.IndexEvidence(ctx, *job)
	require.NoError(t, err)
	var count int
	require.NoError(t, db.Raw("SELECT count(*) FROM workspace.evidence_embedding WHERE project_id=? AND id=?", project, target).Row().Scan(&count))
	require.Zero(t, count)
	require.NoError(t, aigateway.ConfigureEvidence(ctx, db, project, uuid.Nil, 0, false))
	check("看不清远岸的那幕", "disabled", false)
	require.Equal(t, 6, sends)
	// A fresh policy generation can rebuild unchanged inputs from their settled receipts.
	require.NoError(t, aigateway.ConfigureEvidence(ctx, db, project, actor.ID, 12, true))
	var live ep.Policy
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { var err error; live, err = ep.ReadPolicy(tx, project); return err }))
	require.Equal(t, policy.BudgetID, live.BudgetID)
	require.NotEqual(t, policy.Generation, live.Generation)
	nextJob, err := taskruntime.NewStore(db).ForBusiness(live.Generation.String()).Claim(ctx, []string{ep.IndexTask}, "rag-rebuild", time.Minute)
	require.NoError(t, err)
	require.NotNil(t, nextJob)
	for range 2 {
		_, err = runtime.IndexEvidence(ctx, *nextJob)
		require.NoError(t, err)
	}
	require.Equal(t, 6, sends)
	invalidReply = true
	check("完全无关", "unknown-reply", false)
	check("完全无关", "unknown-reply", false)
	require.Equal(t, 7, sends)
	var unknown int
	require.NoError(t, db.Raw("SELECT count(*) FROM ai.request WHERE run_id=? AND state='unknown'", live.BudgetID).Row().Scan(&unknown))
	require.Equal(t, 1, unknown)
	// A new release under the same project model key automatically rebuilds the index.
	invalidReply = false
	replacement := model
	replacement.ID = uuid.New()
	nextRoute := route
	nextRoute.ID, nextRoute.ModelID, nextRoute.Credential = uuid.New(), replacement.ID, uuid.New()
	require.NoError(t, admin.PublishModel(ctx, replacement))
	require.NoError(t, admin.PublishRoute(ctx, nextRoute, []byte("fixture"), "fixture", 8, 300))
	readPolicy := func() ep.Policy {
		t.Helper()
		p, err := ep.ReadPolicy(db, project)
		require.NoError(t, err)
		return p
	}
	switched := readPolicy()
	require.Equal(t, replacement.ID, switched.ReleaseID)
	require.Equal(t, policy.BudgetID, switched.BudgetID)
	require.False(t, switched.QueryReady())
	check("完全无关", "switching", false)
	require.Equal(t, 7, sends)
	for range 2 {
		_, err = runtime.IndexEvidence(ctx, *nextJob)
		require.NoError(t, err)
	}
	require.True(t, readPolicy().QueryReady())
	require.Equal(t, 8, sends)
	require.NoError(t, selectDefault(""))
	require.False(t, readPolicy().QueryReady())
	check("完全无关", "removed-default", false)
	require.NoError(t, selectDefault(replacement.Name))
	lifecycleFixtureSQL(t, db, "UPDATE ai.credential SET state='revoked' WHERE id=?", nextRoute.Credential)
	require.Empty(t, list("text.embed"))
	require.False(t, readPolicy().QueryReady())
	check("完全无关", "withdrawn", false)
	_, err = runtime.IndexEvidence(ctx, *nextJob)
	require.NoError(t, err)
	require.Equal(t, 8, sends)
	require.NoError(t, aigateway.ConfigureEvidence(ctx, db, project, uuid.Nil, 0, false))
}
