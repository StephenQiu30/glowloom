package operation_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	agentpg "github.com/StephenQiu30/glowloom-server/backend/internal/agent/adapter/postgres"
	agentapp "github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	agent "github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	pgop "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/postgres"
	op "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

type agentProjectAccess struct{}

func (agentProjectAccess) AuthorizeOperation(context.Context, identity.Principal, uuid.UUID, bool) error {
	return nil
}

func TestAgentSessionPersistsAndRemainsPrivateWithinProject(t *testing.T) {
	db := operationStoreDB(t)
	actor, pid := operationStoreProject(t, db)
	store := agentpg.NewStore(db, func(*gorm.DB) op.ProjectAccess { return agentProjectAccess{} }, func(tx *gorm.DB) agentapp.Operations { return operationStore(tx) })
	key := uuid.New()
	one, err := store.Create(t.Context(), actor, pid, key, "海边海报")
	if err != nil {
		t.Fatal(err)
	}
	two, err := store.Create(t.Context(), actor, pid, key, "海边海报")
	if err != nil || one.ID != two.ID {
		t.Fatal("creation replay", err)
	}
	_, err = store.Mutate(t.Context(), actor, pid, one.ID, func(s *agent.Session, _ agentapp.Operations) error {
		s.Turns = append(s.Turns, agent.Turn{ID: uuid.New(), Status: "preparing", Phase: "plan", Policy: agent.Policy, CreatedAt: time.Now()})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	fresh := agentpg.NewStore(db, func(*gorm.DB) op.ProjectAccess { return agentProjectAccess{} }, func(tx *gorm.DB) agentapp.Operations { return operationStore(tx) })
	saved, err := fresh.Read(t.Context(), actor, pid, one.ID)
	if err != nil || len(saved.Turns) != 1 || saved.Revision != 2 {
		t.Fatalf("readback=%+v err=%v", saved, err)
	}
	peer := actor
	peer.ID = uuid.New()
	if _, err := fresh.Read(t.Context(), peer, pid, one.ID); !errors.Is(err, agent.ErrNotFound) {
		t.Fatal("project peer read private session", err)
	}
	if list, err := fresh.List(t.Context(), peer, pid); err != nil || len(list) != 0 {
		t.Fatal("project peer listed private sessions", err)
	}
	if _, err := fresh.Read(t.Context(), actor, uuid.New(), one.ID); !errors.Is(err, agent.ErrNotFound) {
		t.Fatal("cross project read", err)
	}
}

func TestAgentQuoteAndConfirmShareSessionTransaction(t *testing.T) {
	db := operationStoreDB(t)
	actor, pid := operationStoreProject(t, db)
	_, model, _ := seedQuoteCatalog(t, db, pid, time.Now().UTC())
	if err := db.Exec(`UPDATE catalog.model_profile SET capability='image.generate' WHERE id=?`, model).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`UPDATE catalog.model_profile_version SET modes=ARRAY['text2image'], limits='{"max_outputs":1}'::jsonb WHERE model_profile_id=?`, model).Error; err != nil {
		t.Fatal(err)
	}
	store := agentpg.NewStore(db, operationRequestAccess, func(tx *gorm.DB) agentapp.Operations { return operationStore(tx) })
	saved, err := store.Create(t.Context(), actor, pid, uuid.New(), "事务回归")
	if err != nil {
		t.Fatal(err)
	}
	saved, err = store.Mutate(t.Context(), actor, pid, saved.ID, func(s *agent.Session, _ agentapp.Operations) error {
		s.Turns = append(s.Turns, agent.Turn{ID: uuid.New(), Policy: agent.Policy, Phase: "image", Status: "preparing", Stages: []agent.Stage{}, Input: agent.TurnInput{Image: agent.ModelChoice{Key: "quote-model-" + model.String(), Mode: "text2image", Params: json.RawMessage(`{}`)}}, Plan: &agent.Plan{Action: "generate_image", Message: "图片方案", Prompt: "有风的海面"}})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	service := agentapp.Service{Store: store}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	quoted, err := service.Advance(ctx, actor, pid, saved.ID)
	if err != nil {
		t.Fatal("quote must not block on its own project lock", err)
	}
	turn := quoted.Turns[0]
	if turn.Status != "awaiting_confirmation" || len(turn.Stages) != 1 {
		t.Fatal(quoted)
	}
	peer := actor
	peer.ID = uuid.New()
	// The permissive project port isolates the private-operation owner check.
	public := pgop.NewScopedStore(db, func(*gorm.DB) op.ProjectAccess { return agentProjectAccess{} }, nil, nil)
	id := turn.Stages[0].OperationID
	if _, err := public.ReadPublicTask(ctx, actor, pid, id); err != nil {
		t.Fatal("owner read", err)
	}
	if _, err := public.ReadPublicTask(ctx, peer, pid, id); !errors.Is(err, op.ErrPublicNotFound) {
		t.Fatal("peer read", err)
	}
	if _, err := public.ConfirmSingleQuote(ctx, peer, op.ConfirmSingleQuoteInput{ProjectID: pid, OperationID: id, RequestID: uuid.NewString()}); !errors.Is(err, op.ErrPublicNotFound) {
		t.Fatal("peer confirm", err)
	}
	if _, err := public.RequestWorkflowControl(ctx, peer, op.WorkflowControlInput{ProjectID: pid, TargetType: "operation", TargetID: id, Action: "cancel", RequestID: uuid.NewString()}); !errors.Is(err, op.ErrPublicNotFound) {
		t.Fatal("peer cancel", err)
	}
	in := agentapp.ConfirmInput{TurnID: turn.ID, OperationID: turn.Stages[0].OperationID, ExpectedRevision: quoted.Revision}
	// Fail after a real reservation has been created: the outer session
	// transaction must roll back both the operation and billing effects.
	rollbackErr := errors.New("simulated session persistence failure")
	failing := agentapp.Service{Store: agentpg.NewStore(db, operationRequestAccess, func(tx *gorm.DB) agentapp.Operations {
		return agentConfirmFailure{Operations: operationStore(tx), err: rollbackErr}
	})}
	if _, err := failing.Confirm(ctx, actor, pid, saved.ID, in); !errors.Is(err, rollbackErr) {
		t.Fatal("expected injected failure", err)
	}
	var rolledBack int64
	if err := db.Raw(`SELECT count(*) FROM billing.reservation WHERE operation_id=?`, in.OperationID).Scan(&rolledBack).Error; err != nil || rolledBack != 0 {
		t.Fatal("reservation escaped rollback", rolledBack, err)
	}
	unchanged, err := store.Read(ctx, actor, pid, saved.ID)
	if err != nil || unchanged.Revision != quoted.Revision || unchanged.Turns[0].Status != "awaiting_confirmation" {
		t.Fatal("session escaped rollback", err)
	}
	confirmed, err := service.Confirm(ctx, actor, pid, saved.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := service.Confirm(ctx, actor, pid, saved.ID, in)
	if err != nil || replay.Revision != confirmed.Revision {
		t.Fatal("confirmation replay", err)
	}
	advanced, err := service.Advance(ctx, actor, pid, saved.ID)
	if err != nil || advanced.Turns[0].Status != "running" {
		t.Fatal("read running operation in session transaction", err)
	}
	var count int64
	if err := db.Raw(`SELECT count(*) FROM billing.reservation WHERE operation_id=?`, in.OperationID).Scan(&count).Error; err != nil || count != 1 {
		t.Fatal("exactly one reservation", count, err)
	}
}

// agentConfirmFailure fails after the real database confirmation writes.
type agentConfirmFailure struct {
	agentapp.Operations
	err error
}

func (f agentConfirmFailure) ConfirmSingleQuote(ctx context.Context, actor identity.Principal, in op.ConfirmSingleQuoteInput) (op.ConfirmSingleQuoteResult, error) {
	out, err := f.Operations.ConfirmSingleQuote(ctx, actor, in)
	if err != nil {
		return out, err
	}
	return out, f.err
}
