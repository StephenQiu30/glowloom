package application

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	op "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

type memoryStore struct {
	sync.Mutex
	session    domain.Session
	operations Operations
}

func (m *memoryStore) Create(context.Context, identity.Principal, uuid.UUID, uuid.UUID, string) (domain.Session, error) {
	return domain.Session{}, domain.ErrInvalid
}
func (m *memoryStore) List(context.Context, identity.Principal, uuid.UUID) ([]domain.Session, error) {
	return nil, nil
}
func (m *memoryStore) Read(ctx context.Context, a identity.Principal, p, s uuid.UUID) (domain.Session, error) {
	return m.Mutate(ctx, a, p, s, func(*domain.Session, Operations) error { return nil })
}
func (m *memoryStore) Mutate(_ context.Context, a identity.Principal, p, s uuid.UUID, f func(*domain.Session, Operations) error) (domain.Session, error) {
	m.Lock()
	defer m.Unlock()
	if a.ID != m.session.ActorID || p != m.session.ProjectID || s != m.session.ID {
		return domain.Session{}, domain.ErrNotFound
	}
	raw, _ := json.Marshal(m.session)
	var next domain.Session
	_ = json.Unmarshal(raw, &next)
	next.ActorID = m.session.ActorID
	if err := f(&next, m.operations); err != nil {
		return domain.Session{}, err
	}
	before, _ := json.Marshal(m.session.Turns)
	after, _ := json.Marshal(next.Turns)
	if string(before) != string(after) {
		next.Revision++
	}
	m.session = next
	return next, nil
}

type fakeOperations struct {
	quotes        map[string]op.CreateFreeQuoteResult
	tasks         map[uuid.UUID]op.TaskDetail
	confirmations map[string]bool
	inputs        []op.CreateFreeQuoteInput
}

func (f *fakeOperations) CreateFreeQuote(_ context.Context, _ identity.Principal, in op.CreateFreeQuoteInput) (op.CreateFreeQuoteResult, error) {
	if got, ok := f.quotes[in.RequestID]; ok {
		return got, nil
	}
	id := uuid.New()
	got := op.CreateFreeQuoteResult{OperationID: id, QuoteMicros: 123, ExpiresAt: time.Now().Add(time.Minute)}
	f.quotes[in.RequestID] = got
	f.inputs = append(f.inputs, in)
	f.tasks[id] = op.TaskDetail{TaskSummary: op.TaskSummary{ID: id, ProjectID: in.ProjectID, Status: "quoted"}}
	return got, nil
}
func (f *fakeOperations) ConfirmSingleQuote(_ context.Context, _ identity.Principal, in op.ConfirmSingleQuoteInput) (op.ConfirmSingleQuoteResult, error) {
	f.confirmations[in.RequestID] = true
	task := f.tasks[in.OperationID]
	task.Status = "confirmed"
	f.tasks[in.OperationID] = task
	return op.ConfirmSingleQuoteResult{}, nil
}
func (f *fakeOperations) ReadPublicTask(_ context.Context, _ identity.Principal, p, id uuid.UUID) (op.TaskDetail, error) {
	task := f.tasks[id]
	if task.ProjectID != p {
		return task, domain.ErrNotFound
	}
	return task, nil
}
func (f *fakeOperations) complete(id uuid.UUID, text string) {
	task := f.tasks[id]
	task.Status = "completed"
	task.Outputs = []op.TaskOutput{{ID: uuid.New(), Kind: "json", ModerationStatus: "passed", Text: &text}}
	f.tasks[id] = task
}

func agentFixture() (*Service, *memoryStore, *fakeOperations, identity.Principal, domain.TurnInput) {
	a := identity.Principal{ID: uuid.New(), OrgID: uuid.New()}
	store := &memoryStore{session: domain.Session{ID: uuid.New(), ProjectID: uuid.New(), ActorID: a.ID, Revision: 1, Turns: []domain.Turn{}}}
	ops := &fakeOperations{quotes: map[string]op.CreateFreeQuoteResult{}, tasks: map[uuid.UUID]op.TaskDetail{}, confirmations: map[string]bool{}}
	in := domain.TurnInput{RequestID: uuid.New(), ExpectedRevision: 1, Message: "海边海报", Planner: domain.ModelChoice{Key: "planner", Mode: "text_to_text", Params: json.RawMessage(`{}`)}, Image: domain.ModelChoice{Key: "image", Mode: "text2image", Params: json.RawMessage(`{}`)}}
	store.operations = ops
	return &Service{Store: store}, store, ops, a, in
}

func TestAgentRequiresEachConfirmationAndRecoversOriginalOperations(t *testing.T) {
	s, store, ops, a, in := agentFixture()
	pid, sid := store.session.ProjectID, store.session.ID
	value, err := s.Start(t.Context(), a, pid, sid, in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Start(t.Context(), a, pid, sid, in); err != nil {
		t.Fatal("same start not replayed", err)
	}
	if len(value.Turns) != 1 || len(ops.inputs) != 0 {
		t.Fatal("start must only persist intent")
	}
	advance := func() domain.Session {
		t.Helper()
		v, e := s.Advance(t.Context(), a, pid, sid)
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	value = advance()
	_ = advance()
	if len(ops.inputs) != 1 || len(ops.confirmations) != 0 {
		t.Fatal("quote polling charged or duplicated")
	}
	confirm := func(v domain.Session) {
		t.Helper()
		turn := v.Turns[0]
		stage := turn.Stages[len(turn.Stages)-1]
		input := ConfirmInput{TurnID: turn.ID, OperationID: stage.OperationID, ExpectedRevision: v.Revision}
		if _, e := s.Confirm(t.Context(), a, pid, sid, input); e != nil {
			t.Fatal(e)
		}
		if _, e := s.Confirm(t.Context(), a, pid, sid, input); e != nil {
			t.Fatal("confirmation replay", e)
		}
	}
	confirm(value)
	planID := value.Turns[0].Stages[0].OperationID
	ops.complete(planID, `{"action":"generate_image","message":"制作海边海报","prompt":"温暖海边咖啡馆"}`)
	value = advance()
	if len(ops.inputs) != 1 || value.Turns[0].Phase != "image" {
		t.Fatal("unexpected automatic image dispatch")
	}
	value = advance()
	if len(ops.inputs) != 2 || len(ops.confirmations) != 1 {
		t.Fatal("image must have independent confirmation")
	}
	confirm(value)
	imageID := value.Turns[0].Stages[1].OperationID
	task := ops.tasks[imageID]
	task.Status = "succeeded"
	asset := uuid.New()
	task.Outputs = []op.TaskOutput{{ID: uuid.New(), MediaAssetID: &asset, Kind: "image", ModerationStatus: "passed"}}
	ops.tasks[imageID] = task
	value = advance()
	if value.Turns[0].Phase != "image" {
		t.Fatal("unarchived succeeded result advanced")
	}
	task.Status = "completed"
	ops.tasks[imageID] = task
	value = advance()
	value = advance()
	if len(ops.inputs) != 3 || len(ops.confirmations) != 2 {
		t.Fatal("summary must also be confirmed")
	}
	confirm(value)
	ops.complete(value.Turns[0].Stages[2].OperationID, "图片已归档。")
	value = advance()
	if value.Turns[0].Status != "completed" || value.Turns[0].MediaAssetID == nil || len(ops.confirmations) != 3 {
		t.Fatal("incomplete tool loop")
	}
	for _, input := range ops.inputs {
		if input.AgentContext == nil || !input.AgentContext.Valid() || !input.ForceRegenerate {
			t.Fatal("missing server-owned quote provenance")
		}
	}
}

func TestAgentUnknownAndOtherActorCannotAdvanceOrSpend(t *testing.T) {
	s, store, ops, a, in := agentFixture()
	pid, sid := store.session.ProjectID, store.session.ID
	_, _ = s.Start(t.Context(), a, pid, sid, in)
	v, _ := s.Advance(t.Context(), a, pid, sid)
	id := v.Turns[0].Stages[0].OperationID
	task := ops.tasks[id]
	task.Status = "unknown"
	ops.tasks[id] = task
	for range 3 {
		v, e := s.Advance(t.Context(), a, pid, sid)
		if e != nil || v.Turns[0].Status != "needs_reconciliation" {
			t.Fatal(e)
		}
	}
	if len(ops.inputs) != 1 || len(ops.confirmations) != 0 {
		t.Fatal("unknown caused paid retry")
	}
	other := a
	other.ID = uuid.New()
	if _, err := s.Advance(t.Context(), other, pid, sid); err == nil {
		t.Fatal("other actor accessed private conversation")
	}
}
