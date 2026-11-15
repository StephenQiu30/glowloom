package microservices_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc/credentials/insecure"

	"github.com/StephenQiu30/glowloom-server/backend/creation/prompt/adapter/grpchandoff"
	prompt "github.com/StephenQiu30/glowloom-server/backend/creation/prompt/application"
	"github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/creationhandoff"
	executionpg "github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

type lostFrozenDeliveryACK struct {
	target prompt.FrozenInputDeliveryTarget
	calls  atomic.Int32
}

func (d *lostFrozenDeliveryACK) AcceptFrozenInput(ctx context.Context, key prompt.FrozenInputKey) (prompt.FrozenInputDeliveryReceipt, error) {
	receipt, err := d.target.AcceptFrozenInput(ctx, key)
	if err != nil {
		return receipt, err
	}
	if d.calls.Add(1) == 1 {
		return prompt.FrozenInputDeliveryReceipt{}, errors.New("synthetic lost ACK after durable acceptance")
	}
	return receipt, nil
}

func TestCreationFrozenDeliveryWorkerRecoversLostACKAndRejectsOldLease(t *testing.T) {
	f := newFrozenFixture(t)
	frozen := f.freeze(t)
	tokenToCreation, tokenToExecution := strings.Repeat("source-", 10), strings.Repeat("consumer-", 10)
	sourceServer, err := grpchandoff.NewServer(f.store, tokenToCreation, insecure.NewCredentials())
	if err != nil {
		t.Fatal(err)
	}
	sourceClient, err := creationhandoff.NewSourceClient(creationhandoff.Target{Address: serveSourcePipeline(t, sourceServer), Token: tokenToCreation, Transport: insecure.NewCredentials()})
	if err != nil {
		t.Fatal(err)
	}
	acceptance := executionpg.NewFrozenInputStore(f.execution, sourceClient)
	targetServer, err := creationhandoff.NewConsumerServer(acceptance, tokenToExecution, insecure.NewCredentials())
	if err != nil {
		t.Fatal(err)
	}
	target, err := grpchandoff.NewDeliveryClient(grpchandoff.Target{Address: serveSourcePipeline(t, targetServer), Token: tokenToExecution, Transport: insecure.NewCredentials()})
	if err != nil {
		t.Fatal(err)
	}
	lost := &lostFrozenDeliveryACK{target: target}
	delivery := prompt.NewFrozenInputDelivery(f.store, lost)
	tasks := taskruntime.NewStore(f.creation).ForBusiness(frozen.Input.HandoffID.String())
	job, err := tasks.Claim(t.Context(), []string{prompt.FrozenInputDeliveryTask}, "original-delivery-worker", time.Second)
	if err != nil || job == nil {
		t.Fatalf("claim original source task: %+v %v", job, err)
	}
	if job.BusinessID != frozen.Input.HandoffID.String() {
		t.Fatal("directed source claim changed the original immutable business identity")
	}
	if _, err := delivery.Handle(t.Context(), *job); err == nil {
		t.Fatal("lost target ACK was reported successful")
	}
	var accepted int
	if err := f.admin.Raw(`SELECT count(*) FROM operation.frozen_input_acceptance WHERE handoff_id=?`, frozen.Input.HandoffID).Scan(&accepted).Error; err != nil || accepted != 1 {
		t.Fatalf("target did not persist before ACK loss: %d %v", accepted, err)
	}
	if err := f.admin.Exec(`UPDATE infra.task_work SET lease_until=statement_timestamp()-interval '1 second' WHERE service_owner='creation' AND kind=? AND business_id=? AND owner=? AND epoch=?`, job.Kind, job.BusinessID, job.Owner, job.Epoch).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.ReadFrozenInputDelivery(t.Context(), *job); !errors.Is(err, taskruntime.ErrLeaseLost) {
		t.Fatalf("expired source lease read delivery responsibility: %v", err)
	}
	worker := &taskruntime.Worker{Store: tasks, Handlers: map[string]taskruntime.Handler{prompt.FrozenInputDeliveryTask: delivery.Handle}, Concurrency: 1, Lease: time.Second, Poll: 5 * time.Millisecond}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- worker.Run(ctx) }()
	defer func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	deadline, stop := context.WithTimeout(t.Context(), 10*time.Second)
	defer stop()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		var complete bool
		if err := f.admin.Raw(`SELECT completed_at IS NOT NULL FROM infra.task_work WHERE service_owner='creation' AND kind=? AND business_id=?`, job.Kind, job.BusinessID).Scan(&complete).Error; err != nil {
			t.Fatal(err)
		}
		if complete {
			break
		}
		select {
		case <-ticker.C:
		case <-deadline.Done():
			t.Fatal("reconstructed source worker did not recover the original delivery")
		}
	}
	if lost.calls.Load() != 2 {
		t.Fatalf("delivery did not retain exactly the original input identity: %d calls", lost.calls.Load())
	}
	if err := tasks.Complete(t.Context(), *job); !errors.Is(err, taskruntime.ErrLeaseLost) {
		t.Fatalf("old source worker completed a reclaimed task: %v", err)
	}
	if err := f.admin.Raw(`SELECT count(*) FROM operation.frozen_input_acceptance WHERE handoff_id=?`, frozen.Input.HandoffID).Scan(&accepted).Error; err != nil || accepted != 1 {
		t.Fatalf("recovery duplicated input acceptance: %d %v", accepted, err)
	}
}
