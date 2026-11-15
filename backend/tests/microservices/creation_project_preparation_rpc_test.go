package microservices_test

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"google.golang.org/grpc"

	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/executionbudget"
	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/grpcpreparation"
	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/pgsource"
	workspacesource "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"

	budgetsource "github.com/StephenQiu30/glowloom-server/backend/execution/billing/adapter/creationpreparation"

	budgetrpc "github.com/StephenQiu30/glowloom-server/backend/execution/billing/adapter/grpcprojectbudget"

	budgetpg "github.com/StephenQiu30/glowloom-server/backend/execution/billing/adapter/postgres"

	billing "github.com/StephenQiu30/glowloom-server/backend/execution/billing/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"

	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

func serveProjectPreparation(t *testing.T, server *grpc.Server) string {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	stopped := make(chan error, 1)
	go func() { stopped <- server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
		if err := <-stopped; err != nil && !errors.Is(err, grpc.ErrServerStopped) && !errors.Is(err, net.ErrClosed) {
			t.Error(err)
		}
	})
	return listener.Addr().String()
}

type lostProjectBudgetACK struct {
	mu                 sync.Mutex
	actual             *billing.ProjectBudgets
	prepares, resolves int
	lose               bool
}

func (b *lostProjectBudgetACK) Prepare(ctx context.Context, key billing.ProjectBudgetPreparation) (billing.ProjectBudgetReceipt, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.prepares++
	receipt, err := b.actual.Prepare(ctx, key)
	if err == nil && b.lose {
		b.lose = false
		return billing.ProjectBudgetReceipt{}, errors.New("synthetic lost ACK after actual budget commit")
	}
	return receipt, err
}
func (b *lostProjectBudgetACK) Resolve(ctx context.Context, key billing.ProjectBudgetPreparation) (billing.ProjectBudgetReceipt, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.resolves++
	return b.actual.Resolve(ctx, key)
}

func projectPreparationParticipants(t *testing.T, f *sourcePolicyFixture, loseBudget, loseMedia bool) (func() *workspacesource.ProjectPreparations, *lostProjectBudgetACK, *scopeBootstrapWriters, *executionbudget.Client) {
	t.Helper()
	store := pgsource.New(f.creation)
	sourceTransport, executionTransport, _ := ownerPeerTLS(t, "execution")
	sourceToken := strings.Repeat("s", 32)
	sourceServer, err := grpcpreparation.NewServer(store, sourceToken, "execution", sourceTransport)
	if err != nil {
		t.Fatal(err)
	}
	sourceAddress := serveProjectPreparation(t, sourceServer)
	sourceClient, err := budgetsource.New(budgetsource.Target{Address: sourceAddress, Token: sourceToken, Transport: executionTransport})
	if err != nil {
		t.Fatal(err)
	}
	actual, err := billing.NewProjectBudgets(budgetpg.NewProjectBudgetStore(f.execution), sourceClient)
	if err != nil {
		t.Fatal(err)
	}
	participant := &lostProjectBudgetACK{actual: actual, lose: loseBudget}
	budgetTransport, creationTransport, _ := ownerPeerTLS(t, "creation")
	budgetToken := strings.Repeat("b", 32)
	budgetServer, err := budgetrpc.NewServer(participant, budgetToken, budgetTransport)
	if err != nil {
		t.Fatal(err)
	}
	budgetAddress := serveProjectPreparation(t, budgetServer)
	client, err := executionbudget.New(budgetAddress, budgetToken, creationTransport)
	if err != nil {
		t.Fatal(err)
	}
	writers := &scopeBootstrapWriters{stores: map[writefence.Writer]*pgfence.Store{}, calls: map[writefence.Writer]int{}, lostMediaACK: loseMedia}
	for _, writer := range writefence.Writers() {
		writers.stores[writer] = pgfence.New(authorityDatabase(t, string(writer)))
	}
	newUseCase := func() *workspacesource.ProjectPreparations {
		useCase, err := workspacesource.NewProjectPreparations(pgsource.New(f.creation), client, writefence.NewCoordinator(pgfence.NewAuthorityStore(f.creation), writers))
		if err != nil {
			t.Fatal(err)
		}
		return useCase
	}
	return newUseCase, participant, writers, client
}

// Identity/scope heads remain explicit policy fixtures here. The source facts,
// both private mTLS directions, Execution wallet/budget and all four writer
// installations are real; this is not native public registration/API acceptance.
func TestCreationProjectPreparationActualBudgetAndFourWriterACKRecoverOriginalActiveDecision(t *testing.T) {
	f, spec := projectPreparationFixture(t)
	store := pgsource.New(f.creation)
	newUseCase, participant, writers, client := projectPreparationParticipants(t, f, true, true)
	if _, err := newUseCase().Prepare(t.Context(), spec); err == nil {
		t.Fatal("lost actual budget ACK returned public success")
	}
	state, err := store.ReadProjectPreparation(t.Context(), spec.BudgetRequest())
	if err != nil || state.Budget != nil || state.Activation != nil {
		t.Fatalf("unknown ACK replaced source decision: %v", err)
	}
	var status string
	if err := f.creation.Raw(`SELECT status FROM workspace.project WHERE id=?`, spec.Project.ID).Scan(&status).Error; err != nil || status != "copying" {
		t.Fatalf("premature active project: %s %v", status, err)
	}
	if _, err := newUseCase().Prepare(t.Context(), spec); err == nil {
		t.Fatal("missing original Media ACK returned public success")
	}
	state, err = store.ReadProjectPreparation(t.Context(), spec.BudgetRequest())
	if err != nil || state.Budget == nil || state.Activation != nil {
		t.Fatalf("partial four-writer readiness: %v", err)
	}
	created, err := newUseCase().Prepare(t.Context(), spec)
	if err != nil || created.ID != spec.Project.ID || created.CanvasID != spec.Project.CanvasID || created.Status != "active" || created.Revision != 1 {
		t.Fatalf("original unique active decision: %+v %v", created, err)
	}
	if participant.prepares != 1 || participant.resolves != 2 {
		t.Fatalf("lost ACK repeated prepare instead of original Resolve: prepare=%d resolve=%d", participant.prepares, participant.resolves)
	}
	for _, writer := range writefence.Writers() {
		expected := 1
		if writer == writefence.WriterMedia {
			expected = 2
		}
		if writers.calls[writer] != expected {
			t.Fatalf("replaced/repeated original %s control: %d", writer, writers.calls[writer])
		}
	}
	original, err := pgfence.New(f.creation).Load(t.Context(), spec.Command.ID)
	if err != nil || original.State != writefence.Committed {
		t.Fatalf("activation escaped final command transaction: %+v %v", original, err)
	}
	var facts struct{ Budget, Wallet, Receipt, Events int64 }
	if err := f.admin.Raw(`SELECT (SELECT count(*) FROM billing.budget WHERE project_id=?) AS budget,(SELECT count(*) FROM billing.project_wallet WHERE project_id=?) AS wallet,(SELECT count(*) FROM workspace.project_creation_receipt WHERE actor_id=? AND idem_key=?) AS receipt,(SELECT count(*) FROM infra.outbox WHERE partition_key=? AND topic IN ('glowloom.workspace.project_changed.v1','glowloom.audit.recorded.v1')) AS events`, spec.Project.ID, spec.Project.ID, spec.ActorID, spec.IdempotencyKey, spec.Project.ID.String()).Scan(&facts).Error; err != nil {
		t.Fatal(err)
	}
	if facts.Budget != 1 || facts.Wallet != 1 || facts.Receipt != 1 || facts.Events != 2 {
		t.Fatalf("non-unique actual final facts: %+v", facts)
	}
	if _, err := newUseCase().Prepare(t.Context(), spec); err != nil {
		t.Fatal("committed original replay", err)
	}
	if participant.prepares != 1 || participant.resolves != 2 {
		t.Fatal("committed replay contacted participant")
	}
	changed := spec.BudgetRequest()
	changed.InputSHA256 = strings.Repeat("f", 64)
	if _, err := client.ResolveProjectBudget(t.Context(), changed); !errors.Is(err, workspacesource.ErrProjectPreparationConflict) {
		t.Fatalf("participant original ACK retargeted: %v", err)
	}
	if err := f.creation.Exec(`SELECT id FROM billing.budget LIMIT 1`).Error; err == nil {
		t.Fatal("Creation acquired Execution SQL capability")
	}
	if created.OrgID == uuid.Nil {
		t.Fatal("missing real organization")
	}
	t.Run("original-retention-keeps-preparation", func(t *testing.T) { assertProjectPreparationRetention(t, f, spec) })
}
