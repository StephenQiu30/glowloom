package microservices_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/credentials/insecure"

	"github.com/StephenQiu30/glowloom-server/backend/creation/prompt/adapter/grpchandoff"
	prompt "github.com/StephenQiu30/glowloom-server/backend/creation/prompt/application"
	"github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/creationhandoff"
	executionpg "github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

func TestCreationFrozenRecoveryWorkerClosesCancelledDeliveryAndPersistsExactProof(t *testing.T) {
	f := newFrozenFixture(t)
	frozen := f.freeze(t)
	cancelCommand := f.cancelCommand(t)
	if _, err := f.store.BlockFrozenInput(t.Context(), frozen.Key(), cancelCommand); err != nil {
		t.Fatal(err)
	}
	sourceToken, targetToken := strings.Repeat("cancel-source-", 10), strings.Repeat("cancel-target-", 10)
	sourceServer, err := grpchandoff.NewServer(f.store, sourceToken, insecure.NewCredentials())
	if err != nil {
		t.Fatal(err)
	}
	source, err := creationhandoff.NewSourceClient(creationhandoff.Target{Address: serveSourcePipeline(t, sourceServer), Token: sourceToken, Transport: insecure.NewCredentials()})
	if err != nil {
		t.Fatal(err)
	}
	targetStore := executionpg.NewFrozenInputStore(f.execution, source)
	targetServer, err := creationhandoff.NewConsumerServer(targetStore, targetToken, insecure.NewCredentials())
	if err != nil {
		t.Fatal(err)
	}
	target := grpchandoff.Target{Address: serveSourcePipeline(t, targetServer), Token: targetToken, Transport: insecure.NewCredentials()}
	deliveryClient, err := grpchandoff.NewDeliveryClient(target)
	if err != nil {
		t.Fatal(err)
	}
	cancelClient, err := grpchandoff.NewCancellationClient(target)
	if err != nil {
		t.Fatal(err)
	}
	delivery := prompt.NewFrozenInputDelivery(f.store, deliveryClient)
	cancellation := prompt.NewFrozenInputCancellationRecovery(f.store, cancelClient)
	worker := &taskruntime.Worker{Store: taskruntime.NewStore(f.creation).ForBusiness(frozen.Input.HandoffID.String()), Handlers: map[string]taskruntime.Handler{prompt.FrozenInputDeliveryTask: delivery.Handle, prompt.FrozenInputCancellationTask: cancellation.Handle}, Concurrency: 1, Lease: time.Second, Poll: 5 * time.Millisecond}
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
		var complete int
		if err := f.admin.Raw(`SELECT count(*) FROM infra.task_work WHERE service_owner='creation' AND business_id=? AND kind IN (?,?) AND completed_at IS NOT NULL`, frozen.Input.HandoffID.String(), prompt.FrozenInputDeliveryTask, prompt.FrozenInputCancellationTask).Scan(&complete).Error; err != nil {
			t.Fatal(err)
		}
		if complete == 2 {
			break
		}
		select {
		case <-ticker.C:
		case <-deadline.Done():
			t.Fatal("source recovery worker did not finish delivery/cancellation responsibilities")
		}
	}
	recovered, err := f.store.ReadFrozenInput(t.Context(), frozen.Key())
	if err != nil {
		t.Fatal(err)
	}
	if !recovered.AdoptionBlocked || recovered.CancelProof == nil || recovered.CancelProof.CancelCommandID != cancelCommand.ID || recovered.CancelProof.Outcome != "not_received_frozen" {
		t.Fatalf("source did not retain the exact permanent cancellation proof: %+v", recovered.CancelProof)
	}
	original, err := pgfence.New(f.execution).Load(t.Context(), frozen.Input.Command.ID)
	if err != nil || original.State != writefence.Frozen {
		t.Fatalf("consumer identity was not durably frozen: %+v %v", original, err)
	}
	sourceOriginal, err := pgfence.New(f.creation).Load(t.Context(), frozen.Input.SourceCommand.ID)
	if err != nil || sourceOriginal.State != writefence.Committed {
		t.Fatalf("cancellation rewrote the completed source responsibility: %+v %v", sourceOriginal, err)
	}
	var accepted int
	if err := f.admin.Raw(`SELECT count(*) FROM operation.frozen_input_acceptance WHERE handoff_id=?`, frozen.Input.HandoffID).Scan(&accepted).Error; err != nil || accepted != 0 {
		t.Fatalf("cancelled queued delivery admitted new input: %d %v", accepted, err)
	}
}
