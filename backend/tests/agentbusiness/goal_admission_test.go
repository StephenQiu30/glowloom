package agentbusiness_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	agentpg "github.com/StephenQiu30/glowloom-server/backend/internal/agent/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	agentwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/agent"
	canvasapp "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	canvasdomain "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	operationdomain "github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
)

// Decode the public JSON shape so the RED test also detects an ignored contract.
func admissionGoal(t *testing.T, a domain.Admission, goal string) domain.Admission {
	t.Helper()
	require.NoError(t, json.Unmarshal([]byte(`{"goal_contract":`+goal+`}`), &a))
	return a
}

func TestGoalAdmissionNormalizesAndFreezesIntent(t *testing.T) {
	s, a, _ := fixture(t)
	stored, err := s.CurrentAuthority(t.Context(), a.RootRunID)
	require.NoError(t, err)
	raw, err := json.Marshal(stored)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))
	require.Equal(t, map[string]any{"kind": "chat", "required_deliveries": float64(0)}, got["goal_contract"])
	production := admissionGoal(t, a, `{"kind":"production"}`)
	require.ErrorIs(t, s.AdmitRun(t.Context(), production), domain.ErrConflict, "goal changes cannot replay an admitted request even with the old content hash")
	production.RootRunID, production.SessionID, production.SubmissionID, production.RequestID = uuid.New(), uuid.New(), uuid.New(), uuid.New()
	require.NoError(t, s.AdmitRun(t.Context(), production))
	require.NoError(t, s.AdmitRun(t.Context(), admissionGoal(t, production, `{"kind":"production","required_deliveries":1}`)))
	stored, err = s.CurrentAuthority(t.Context(), production.RootRunID)
	require.NoError(t, err)
	raw, err = json.Marshal(stored)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &got))
	require.Equal(t, map[string]any{"kind": "production", "required_deliveries": float64(1)}, got["goal_contract"])
	anotherRoot := production
	anotherRoot.RootRunID = uuid.New()
	require.ErrorIs(t, s.AdmitRun(t.Context(), anotherRoot), domain.ErrConflict, "one request cannot be bound to another root")
	for _, goal := range []string{`{"kind":"episode"}`, `{"kind":"chat","required_deliveries":1}`, `{"kind":"production","required_deliveries":65}`, `{"kind":"production","required_deliveries":-1}`, `{"kind":"production","delivery":{"duration_ms":1,"aspect_ratio":"1:1","fps":25,"project_format":"fcpxml"}}`, `{"kind":"production","delivery":{"duration_ms":500,"source_start_ms":9223372036854775807,"aspect_ratio":"1:1","fps":25,"project_format":"fcpxml"}}`, `{"kind":"production","source":{"canvas_id":"00000000-0000-0000-0000-000000000001","node_id":"00000000-0000-0000-0000-000000000002","revision":1,"row_id":"00000000-0000-0000-0000-000000000003"}}`} {
		invalid := admissionGoal(t, a, goal)
		require.ErrorIs(t, s.AdmitRun(t.Context(), invalid), domain.ErrInvalid, goal)
	}
}

func TestGoalCompletionEmptyChatAndUnplannedProduction(t *testing.T) {
	_, b, _, _, g := realCanvasBoundary(t)
	view, err := b.Capabilities.RunSnapshot(t.Context(), g)
	require.NoError(t, err)
	cost, ok := view.(map[string]any)["cost"].(domain.RootCostSnapshot)
	require.True(t, ok, "run snapshot carries the existing Go-owned cost contract")
	require.Equal(t, g.RootRunID, cost.RootRunID)
	require.EqualValues(t, 100, cost.AuthorizedLimit)
	forged := g
	forged.GrantID = uuid.New()
	_, err = b.Capabilities.RunSnapshot(t.Context(), forged)
	require.ErrorIs(t, err, domain.ErrGrant)
	raw, err := json.Marshal(view)
	require.NoError(t, err)
	var got struct {
		GoalContract   map[string]any `json:"goal_contract"`
		GoalCompletion struct {
			Status  domain.DeliveryStatus `json:"status"`
			Missing []string              `json:"missing"`
		} `json:"goal_completion"`
	}
	require.NoError(t, json.Unmarshal(raw, &got))
	require.Equal(t, domain.DeliveryCompleted, got.GoalCompletion.Status)
	require.NotNil(t, got.GoalCompletion.Missing)
	current, err := b.Store.CurrentAuthority(t.Context(), g.RootRunID)
	require.NoError(t, err)
	production := current.Admission
	production.RootRunID, production.SessionID, production.SubmissionID, production.RequestID = uuid.New(), uuid.New(), uuid.New(), uuid.New()
	production.GoalContract = domain.GoalContract{Kind: domain.GoalProduction}
	require.NoError(t, b.Store.AdmitRun(t.Context(), production))
	g, err = b.Store.RegisterGrant(t.Context(), domain.GrantRequest{RootRunID: production.RootRunID, SubmissionID: production.SubmissionID, OwnerEpoch: 1, AuthorizationRevision: 1, LeaseUntil: time.Now().Add(time.Minute)})
	require.NoError(t, err)
	view, err = b.Capabilities.RunSnapshot(t.Context(), g)
	require.NoError(t, err)
	raw, err = json.Marshal(view)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &got))
	require.Equal(t, domain.DeliveryIncomplete, got.GoalCompletion.Status)
	require.Contains(t, got.GoalCompletion.Missing, "goal_source_unplanned")
}

func TestProductionOperationSnapshotReadsOnlyBoundOriginals(t *testing.T) {
	db, b, _, _, g := realCanvasBoundary(t)
	view, err := b.Capabilities.RunSnapshot(t.Context(), g)
	require.NoError(t, err)
	require.Equal(t, []map[string]any{}, view.(map[string]any)["production_operations"])
	op, unbound, res := uuid.New(), uuid.New(), uuid.New()
	require.NoError(t, db.Exec(`INSERT INTO operation.operation(id,project_id,target_type,capability,mode,input_hash,origin,status,quote_micros) VALUES(?,?,'free','video.generate','text2video','goal-operation','upload','confirmed',10),(?,?,'free','video.generate','text2video','goal-unbound','upload','completed',0)`, op, g.Scope.ProjectID, unbound, g.Scope.ProjectID).Error)
	require.NoError(t, db.Exec(`INSERT INTO billing.budget(id,project_id,limit_micros,reserved_micros) VALUES(?,?,100,10)`, uuid.New(), g.Scope.ProjectID).Error)
	require.NoError(t, db.Exec(`INSERT INTO billing.reservation(id,project_id,operation_id,amount_micros,status) VALUES(?,?,?,10,'held')`, res, g.Scope.ProjectID, op).Error)
	require.NoError(t, db.Exec(`UPDATE operation.operation SET reservation_id=? WHERE id=?`, res, op).Error)
	require.NoError(t, db.Exec(`INSERT INTO billing.ledger_entry(id,project_id,operation_id,entry_type,amount_micros) VALUES(?,?,?,'reserve',10)`, uuid.New(), g.Scope.ProjectID, op).Error)
	input := costCall(g, 10)
	_, err = b.Store.ExecuteAtomicTool(t.Context(), "original-confirm", input, func(tx *gorm.DB, _ domain.Scope, id uuid.UUID) (domain.CallResult, error) {
		err := agentpg.BindOperationCostInTransaction(tx, id, op)
		return domain.CallResult{Status: "completed", Result: json.RawMessage(`{}`)}, err
	})
	require.NoError(t, err)
	model := costCall(g, 10)
	model.Call.CallID = "model-is-not-a-production-operation"
	_, err = b.Store.ExecuteCall(t.Context(), "model", "model", model, func(context.Context, domain.Scope, uuid.UUID) (domain.CallResult, error) {
		return domain.CallResult{Status: "completed", Result: json.RawMessage(`{"operation_id":"` + unbound.String() + `"}`)}, nil
	})
	require.NoError(t, err)
	for _, state := range []string{"confirmed", "unknown", "reconciling", "manual", "completed"} {
		require.NoError(t, db.Exec(`UPDATE operation.operation SET status=? WHERE id=?`, state, op).Error)
		view, err = b.Capabilities.RunSnapshot(t.Context(), g)
		require.NoError(t, err)
		raw, err := json.Marshal(view.(map[string]any)["production_operations"])
		require.NoError(t, err)
		var facts []struct {
			OperationID uuid.UUID `json:"operation_id"`
			Status      string    `json:"status"`
		}
		require.NoError(t, json.Unmarshal(raw, &facts))
		require.Len(t, facts, 1, "unbound operations and model result JSON cannot enter the production wait set")
		require.Equal(t, op, facts[0].OperationID)
		require.Equal(t, state, facts[0].Status)
	}
}

func goalSourceFixture(t *testing.T, router http.Handler, g domain.Grant) operationdomain.CanvasSource {
	t.Helper()
	status, created, message := businessToolRequest(t, router, g, "canvas_create", "goal-replay-source-canvas", map[string]any{"name": "goal replay source"})
	require.Equal(t, 200, status, message)
	var document canvasdomain.Document
	require.NoError(t, json.Unmarshal(created.Result, &document))
	text := "original goal source"
	node := canvasdomain.Node{ID: uuid.New(), Title: "goal source", NodeType: canvasdomain.NodeTypeText, NodeAction: canvasdomain.NodeActionResource, Config: canvasdomain.NodeConfig{Text: &text}}
	status, created, message = businessToolRequest(t, router, g, "canvas_commands", "goal-replay-source-node", agentwire.CanvasCommandsInput{CanvasID: document.ID, ExpectedRevision: document.Revision, Operations: []canvasdomain.Operation{{Type: canvasdomain.OperationCreateNode, Node: &node}}})
	require.Equal(t, 200, status, message)
	var changed struct {
		Revision canvasapp.Revision `json:"revision"`
	}
	require.NoError(t, json.Unmarshal(created.Result, &changed))
	return operationdomain.CanvasSource{CanvasID: document.ID, NodeID: node.ID, Revision: int64(changed.Revision)}
}

func TestGoalAdmissionReplaySurvivesSourceChangesWithoutLosingScope(t *testing.T) {
	db, b, router, _, g := realCanvasBoundary(t)
	source := goalSourceFixture(t, router, g)
	current, err := b.Store.CurrentAuthority(t.Context(), g.RootRunID)
	require.NoError(t, err)
	admitted := current.Admission
	admitted.SessionID, admitted.RequestID = uuid.New(), uuid.New()
	admitted.RootRunID = domain.RootRunID(admitted.SessionID, admitted.RequestID)
	admitted.SubmissionID = domain.SubmissionID(admitted.RootRunID)
	admitted.GoalContract = domain.GoalContract{Kind: domain.GoalProduction, Source: &source}
	admitted.Scope.CanvasID = &source.CanvasID
	require.NoError(t, b.Store.AdmitRun(t.Context(), admitted))
	before, err := b.Store.CurrentAuthority(t.Context(), admitted.RootRunID)
	require.NoError(t, err)
	// Edit and delete through the real Yjs acceptance path; the original
	// admission remains frozen intent instead of becoming a new source read.
	revision := changeGoalSource(t, router, g, source.CanvasID, source.Revision, "edit-admitted-source", canvasdomain.Operation{Type: "set", NodeID: source.NodeID, Path: []string{"title"}, Value: json.RawMessage(`"edited after admission"`)})
	require.NoError(t, b.Store.AdmitRun(t.Context(), admitted))
	changeGoalSource(t, router, g, source.CanvasID, revision, "delete-admitted-source", canvasdomain.Operation{Type: "delete_node", NodeID: source.NodeID})
	require.NoError(t, b.Store.AdmitRun(t.Context(), admitted), "a lost forward can replay the accepted request after the source node is deleted")
	after, err := b.Store.CurrentAuthority(t.Context(), admitted.RootRunID)
	require.NoError(t, err)
	require.Equal(t, before.Admission, after.Admission)
	changed := admitted
	changed.GoalContract.RequiredDeliveries = 2
	require.ErrorIs(t, b.Store.AdmitRun(t.Context(), changed), domain.ErrConflict, "source deletion must not hide a conflicting replay")
	changedSource := *admitted.GoalContract.Source
	changedSource.NodeID = uuid.New()
	changed = admitted
	changed.GoalContract.Source = &changedSource
	require.ErrorIs(t, b.Store.AdmitRun(t.Context(), changed), domain.ErrConflict, "a changed frozen source is still a conflicting replay")
	anotherRoot := admitted
	anotherRoot.RootRunID = uuid.New()
	anotherRoot.SubmissionID = domain.SubmissionID(anotherRoot.RootRunID)
	require.ErrorIs(t, b.Store.AdmitRun(t.Context(), anotherRoot), domain.ErrConflict, "the accepted request cannot bind a different root even after source deletion")
	fresh := admitted
	fresh.RequestID = uuid.New()
	fresh.RootRunID = domain.RootRunID(fresh.SessionID, fresh.RequestID)
	fresh.SubmissionID = domain.SubmissionID(fresh.RootRunID)
	require.ErrorIs(t, b.Store.AdmitRun(t.Context(), fresh), domain.ErrNotFound, "a new request still requires an existing source")
	wrongScope := admitted
	wrongScope.Scope.ScopeID = uuid.New()
	require.Error(t, b.Store.AdmitRun(t.Context(), wrongScope), "replay does not bypass current scope authorization")
	require.NoError(t, db.Exec(`UPDATE workspace.project SET is_delete=true WHERE id=?`, g.Scope.ProjectID).Error)
	require.Error(t, b.Store.AdmitRun(t.Context(), admitted), "replay does not bypass revoked current project access")
}

func changeGoalSource(t *testing.T, router http.Handler, g domain.Grant, canvasID uuid.UUID, revision int64, callID string, operation canvasdomain.Operation) int64 {
	t.Helper()
	status, result, message := businessToolRequest(t, router, g, "canvas_commands", callID, agentwire.CanvasCommandsInput{CanvasID: canvasID, ExpectedRevision: revision, Operations: []canvasdomain.Operation{operation}})
	require.Equal(t, 200, status, message)
	var changed struct {
		Revision canvasapp.Revision `json:"revision"`
	}
	require.NoError(t, json.Unmarshal(result.Result, &changed))
	return int64(changed.Revision)
}

func TestGoalAdmissionReplayPreservesQuiescedContinuation(t *testing.T) {
	s, original, _ := fixture(t)
	pause := decide(t, s, original, "pause", 1)
	resume := decide(t, s, original, "resume", pause.AuthorizationRevision)
	continuation, err := s.Quiesce(t.Context(), original.RootRunID, domain.QuiescedReceipt{ReceiptID: uuid.New(), DecisionID: resume.DecisionID, AuthorizationRevision: resume.AuthorizationRevision, OwnerEpoch: 1, ContinuationID: domain.ContinuationID(original.RootRunID, resume.DecisionID), LeaseUntil: time.Now().Add(time.Minute)})
	require.NoError(t, err)
	require.NotEqual(t, original.SubmissionID, continuation.SubmissionID)
	require.NoError(t, s.AdmitRun(t.Context(), original), "the execution segment can change without changing the accepted request")
	current, err := s.CurrentAuthority(t.Context(), original.RootRunID)
	require.NoError(t, err)
	require.Equal(t, continuation.SubmissionID, current.SubmissionID, "admission replay must not reset the running continuation")
	require.Equal(t, original.ContentHash, current.ContentHash)
	require.Equal(t, original.RequestID, current.RequestID)
}
