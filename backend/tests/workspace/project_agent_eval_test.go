package workspace_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	canvas "github.com/StephenQiu30/glowloom-server/backend/creation/canvas/domain"
	"github.com/StephenQiu30/glowloom-server/backend/creation/evidence"
	workspacesource "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"
	billing "github.com/StephenQiu30/glowloom-server/backend/execution/billing/domain"
	operation "github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
	operationdomain "github.com/StephenQiu30/glowloom-server/backend/execution/operation/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	agentpg "github.com/StephenQiu30/glowloom-server/backend/internal/agent/adapter/postgres"
	agent "github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	agentwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/agent"
)

type agentEvalCase struct {
	Version      int      `json:"version"`
	CaseID       string   `json:"case_id"`
	SourceName   string   `json:"source_name"`
	ForeignText  string   `json:"foreign_text"`
	Timeline     []string `json:"timeline"`
	ShotVersions []struct {
		Revision int64  `json:"revision"`
		Text     string `json:"text"`
	} `json:"shot_versions"`
}

type evalModelInput struct {
	OperationID uuid.UUID     `json:"operation_id"`
	Receipts    []uuid.UUID   `json:"project_evidence_receipts"`
	Source      evidence.Item `json:"source"`
}

type evalObservation struct {
	Call     *agent.CallReceipt
	Outbound *evalModelInput
	Order    *operation.TaskDetail
	Sends    int
}

// evaluateAgentTrial uses observed calls and the persisted operation. Missing
// observation and observed wrong evidence have different outcomes.
func evaluateAgentTrial(observed evalObservation, originalOrder, sourceReceipt uuid.UUID, source evidence.Item) (status, reason string) {
	if observed.Call == nil || observed.Call.ReceiptID == uuid.Nil || observed.Outbound == nil || observed.Order == nil {
		return "unverified", "missing_observation"
	}
	if observed.Outbound.OperationID != originalOrder || observed.Order.ID != originalOrder || observed.Order.Status != operationdomain.StatusConfirmed {
		return "failed", "wrong_order"
	}
	if !slices.Equal(observed.Outbound.Receipts, []uuid.UUID{sourceReceipt}) ||
		observed.Outbound.Source.Reference != source.Reference || observed.Outbound.Source.Revision != source.Revision ||
		observed.Outbound.Source.Excerpt != source.Excerpt {
		return "failed", "source_not_in_request"
	}
	if observed.Sends != 1 || observed.Call.Status != agent.CallStatusCompleted {
		return "failed", "outbound_count_or_receipt"
	}
	return "passed", ""
}

func TestProjectAgentEvalFixtureDistinguishesResults(t *testing.T) {
	rawDSN := os.Getenv("GLOWLOOM_TEST_WORKSPACE_DB_DSN")
	if rawDSN != "" {
		dsn, err := url.Parse(rawDSN)
		require.NoError(t, err)
		require.True(t, strings.HasSuffix(dsn.Path, "_test") && (dsn.Hostname() == "127.0.0.1" || dsn.Hostname() == "localhost"), "evaluation fixture requires a loopback *_test database")
	}
	ctx, db := workspaceMigrationDB(t)
	tx := db.Begin()
	require.NoError(t, tx.Error)
	defer rollbackLifecycleTest(t, tx)

	raw, err := os.ReadFile("testdata/agent_eval_v1.json")
	require.NoError(t, err)
	var spec agentEvalCase
	require.NoError(t, json.Unmarshal(raw, &spec))
	require.Equal(t, 1, spec.Version)
	require.Len(t, spec.ShotVersions, 2)
	var steps []string
	var now time.Time
	require.NoError(t, tx.Raw("SELECT transaction_timestamp()").Row().Scan(&now))
	org := insertWorkspaceOrganization(ctx, t, tx)
	owner := workspaceTeamActor(t, tx, insertWorkspaceActor(ctx, t, tx, org))
	member := insertWorkspaceActor(ctx, t, tx, org)
	project := uuid.MustParse(insertWorkspaceProject(ctx, t, tx, owner, "16:9", "realistic", nil))
	otherProject := uuid.MustParse(insertWorkspaceProject(ctx, t, tx, member, "16:9", "realistic", nil))
	lifecycleFixtureSQL(t, tx, "INSERT INTO billing.budget(id,project_id,limit_micros) VALUES(?,?,?)", uuid.New(), project, 1000*billing.MicrosPerCredit)
	var team uuid.UUID
	require.NoError(t, tx.Raw("SELECT team_id FROM workspace.creation_scope WHERE id=?", owner.ScopeID).Row().Scan(&team))
	lifecycleFixtureSQL(t, tx, `INSERT INTO workspace.team_member(team_id,user_id,role,status,joined_at,source)
		VALUES(?,?,'member','active',?,'invited')`, team, member.ID, now)
	member.ScopeID = owner.ScopeID

	shotContent := func(shotID uuid.UUID, revision int64, text string) string {
		config := canvas.NodeConfig{Storyboard: &canvas.Storyboard{Shots: []canvas.StoryboardShot{{
			ID: shotID, Revision: revision, StoryboardDraft: canvas.StoryboardDraft{Title: spec.SourceName, Action: text, DurationMS: 4000},
		}}}}
		raw, err := json.Marshal(config)
		require.NoError(t, err)
		return string(raw)
	}
	seedSource := func(projectID uuid.UUID, revision int64, text string) (canvasID, boardID, shotID uuid.UUID) {
		canvasID, boardID, shotID = uuid.New(), uuid.New(), uuid.New()
		lifecycleFixtureSQL(t, tx, "INSERT INTO canvas.document(id,project_id,name) VALUES(?,?,?)", canvasID, projectID, spec.CaseID)
		lifecycleFixtureSQL(t, tx, `INSERT INTO canvas.node(id,document_id,node_type,node_action,title,x,y,config)
			VALUES(?,?,'storyboard','tool','评测故事板',0,0,?::jsonb)`, boardID, canvasID, shotContent(shotID, revision, text))
		return canvasID, boardID, shotID
	}
	first, updated := spec.ShotVersions[0], spec.ShotVersions[1]
	doc, board, shot := seedSource(project, first.Revision, first.Text)
	_, _, foreignShot := seedSource(otherProject, first.Revision, spec.ForeignText)

	var observed evalObservation
	boundary := agentwire.NewBoundary(tx, func(_ context.Context, scope agent.Scope, _ uuid.UUID, raw json.RawMessage, _ int64) (agent.CallResult, error) {
		observed.Sends++
		require.Equal(t, project, scope.ProjectID)
		var input evalModelInput
		require.NoError(t, json.Unmarshal(raw, &input))
		observed.Outbound = &input
		return agent.CallResult{Status: agent.CallStatusCompleted, Result: json.RawMessage(`{"output":"fixture"}`)}, nil
	})
	boundary.Store = agentpg.NewStore(tx, agentwire.ProvideControlAccess, func() time.Time { return now })
	const token = "isolated-project-agent-eval-fixture-token"
	router := gin.New()
	boundary.BusinessHandler(token).Register(router)
	grantFor := func(actor identity.Principal) agent.Grant {
		a := agent.Admission{RootRunID: uuid.New(), SessionID: uuid.New(), SubmissionID: uuid.New(), RequestID: uuid.New(),
			ContentHash: agent.Hash(raw), BudgetMicros: 100,
			Scope: agent.Scope{ActorID: actor.ID, OrgID: actor.OrgID, ScopeID: actor.ScopeID, ProjectID: project, CanvasID: &doc}}
		require.NoError(t, boundary.Store.AdmitRun(ctx, a))
		g, err := boundary.Store.RegisterGrant(ctx, agent.GrantRequest{RootRunID: a.RootRunID, SubmissionID: a.SubmissionID,
			OwnerEpoch: 1, AuthorizationRevision: 1, LeaseUntil: now.Add(30 * time.Second)})
		require.NoError(t, err)
		return g
	}
	newCall := func(grant agent.Grant, input any) agent.CallInput {
		body, err := json.Marshal(input)
		require.NoError(t, err)
		return agent.CallInput{Grant: grant, Input: body, Call: agent.CallIdentity{TaskID: spec.CaseID,
			CallID: uuid.NewString(), CheckpointID: "fixture", Attempt: 1, ContractVersion: "v1", InputHash: agent.Hash(body)}}
	}
	post := func(path string, input agent.CallInput, status int) agent.CallReceipt {
		body, err := json.Marshal(input)
		require.NoError(t, err)
		req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/internal/agent-business/"+path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Agent-Audience", "agent-business")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		require.Equal(t, status, res.Code, "%s", res.Body.Bytes())
		var receipt agent.CallReceipt
		if status == http.StatusOK {
			require.NoError(t, json.Unmarshal(res.Body.Bytes(), &receipt))
			require.Equal(t, agent.CallStatusCompleted, receipt.Status)
		}
		return receipt
	}
	search := func(grant agent.Grant) (agent.CallReceipt, evidence.Item) {
		receipt := post("tools/"+evidence.SearchTool+"/execute", newCall(grant, evidence.Query{Text: spec.SourceName}), http.StatusOK)
		var result evidence.Result
		require.NoError(t, json.Unmarshal(receipt.Result, &result))
		require.Len(t, result.Items, 1)
		require.Equal(t, shot, result.Items[0].ID)
		return receipt, result.Items[0]
	}
	ownerGrant, memberGrant := grantFor(owner), grantFor(member)
	_, oldSource := search(memberGrant)
	require.Equal(t, first.Revision, oldSource.Revision)
	require.Equal(t, first.Text, oldSource.Excerpt)
	steps = append(steps, "team_member_can_read")
	foreign := post("tools/"+evidence.ReadTool+"/execute", newCall(ownerGrant, evidence.Reference{Kind: "storyboard_shot", ID: foreignShot}), http.StatusOK)
	require.JSONEq(t, `{"status":"unavailable","items":[]}`, string(foreign.Result))
	lifecycleFixtureSQL(t, tx, `UPDATE canvas.node SET config=?::jsonb WHERE id=?`, shotContent(shot, updated.Revision, updated.Text), board)
	evidenceReceipt, latest := search(ownerGrant)
	require.Equal(t, updated.Revision, latest.Revision)
	require.Equal(t, updated.Text, latest.Excerpt)
	require.NotEqual(t, oldSource.Version, latest.Version)
	steps = append(steps, "shot_revision_changes")
	lifecycleFixtureSQL(t, tx, "UPDATE workspace.team_member SET status='removed' WHERE team_id=? AND user_id=?", team, member.ID)
	_, err = boundary.Store.ValidateGrant(ctx, memberGrant)
	require.ErrorIs(t, err, workspacesource.ErrScopeNotFound)
	// The current business HTTP adapter maps this workspace denial to unavailable.
	post("tools/"+evidence.SearchTool+"/execute", newCall(memberGrant, evidence.Query{Text: spec.SourceName}), http.StatusServiceUnavailable)
	steps = append(steps, "team_member_is_revoked")

	store := workspaceOperationStore(tx)
	pendingOrder := func() uuid.UUID {
		q := lifecycleQuote(t, tx, owner, project)
		_, err := store.ConfirmSingleQuote(ctx, owner, operation.ConfirmSingleQuoteInput{ProjectID: project, OperationID: q.OperationID, RequestID: uuid.NewString()})
		require.NoError(t, err)
		return q.OperationID
	}
	original, wrong := pendingOrder(), pendingOrder()
	run := newCall(ownerGrant, evalModelInput{OperationID: original, Receipts: []uuid.UUID{evidenceReceipt.ReceiptID}, Source: latest})
	receipt := post("models/stream", run, http.StatusOK)
	replay := post("models/stream", run, http.StatusOK)
	require.Equal(t, receipt.ReceiptID, replay.ReceiptID)
	read, err := boundary.Store.ReadCall(ctx, run.Grant.RootRunID, run.Grant.SubmissionID, run.Call)
	require.NoError(t, err)
	require.Equal(t, receipt.ReceiptID, read.ReceiptID)
	observed.Call = &read
	steps = append(steps, "owner_replays_original_call")
	require.Equal(t, spec.Timeline, steps)

	order, err := store.ReadPublicTask(ctx, owner, project, original)
	require.NoError(t, err)
	otherOrder, err := store.ReadPublicTask(ctx, owner, project, wrong)
	require.NoError(t, err)
	require.Equal(t, order.Status, otherOrder.Status)
	observed.Order = &order
	check := func(input evalObservation, status, reason string) {
		got, problem := evaluateAgentTrial(input, original, evidenceReceipt.ReceiptID, latest)
		require.Equal(t, status, got, problem)
		require.Equal(t, reason, problem)
	}
	check(observed, "passed", "")
	wrongOrder := observed
	wrongOrder.Order = &otherOrder
	wrongInput := *observed.Outbound
	wrongInput.OperationID = wrong
	wrongOrder.Outbound = &wrongInput
	check(wrongOrder, "failed", "wrong_order")
	missingSource := observed
	missingSource.Outbound = &evalModelInput{OperationID: original, Source: latest}
	check(missingSource, "failed", "source_not_in_request")
	noTrace := observed
	noTrace.Outbound = nil
	check(noTrace, "unverified", "missing_observation")
	var count int64
	require.NoError(t, tx.Raw("SELECT count(*) FROM operation.operation WHERE project_id=?", project).Row().Scan(&count))
	require.EqualValues(t, 2, count, "replaying the model call must not create another business order")
}
