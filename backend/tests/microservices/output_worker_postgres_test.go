package microservices_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	billingpg "github.com/StephenQiu30/glowloom-server/backend/execution/billing/adapter/postgres"
	outputworker "github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/mediaoutput"
	executionpg "github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/settlement"
	executionapp "github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
	contract "github.com/StephenQiu30/glowloom-server/backend/internal/contract/outputpublication"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

type workerArtifacts struct {
	identity executionapp.ProviderDispatchIdentity
	artifact executionapp.ManagedArtifact
	body     []byte
	opens    atomic.Int32
}

func (a *workerArtifacts) OpenManagedArtifact(_ context.Context, id executionapp.ProviderDispatchIdentity, artifact executionapp.ManagedArtifact) (io.ReadCloser, error) {
	if id != a.identity || artifact != a.artifact {
		return nil, executionapp.ErrProviderCallConflict
	}
	a.opens.Add(1)
	return io.NopCloser(bytes.NewReader(a.body)), nil
}

type unusedOriginalArchive struct{ calls atomic.Int32 }

func (a *unusedOriginalArchive) IngestOriginal(context.Context, executionapp.WorkflowControlDelivery) error {
	a.calls.Add(1)
	return executionapp.ErrInvalidWorkflowControl
}

// workerMediaACK deliberately fixtures the independent Media response here; real
// byte archival/FFmpeg/COS/mTLS is accepted by the separate actual archive test.
type workerMediaACK struct {
	mu                  sync.Mutex
	ack                 contract.OperationOutputReceipt
	allow               bool
	publishes, resolves int
	firstResolve        chan struct{}
	once                sync.Once
}

func (m *workerMediaACK) Publish(_ context.Context, req contract.Request, body io.Reader) (contract.OperationOutputReceipt, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return contract.OperationOutputReceipt{}, err
	}
	sum := sha256.Sum256(data)
	if int64(len(data)) != req.Output.ByteSize || hex.EncodeToString(sum[:]) != req.Output.SHA256 {
		return contract.OperationOutputReceipt{}, contract.ErrOutputPublicationConflict
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.publishes++
	m.ack.CommandID = req.Command.ID
	m.ack.RequestSHA256 = req.Command.SHA256
	m.ack.PayloadSHA256 = req.Output.Digest()
	m.ack.Output = req.Output
	return contract.OperationOutputReceipt{}, writefence.ErrUnavailable
}
func (m *workerMediaACK) Resolve(_ context.Context, req contract.Request) (contract.OperationOutputReceipt, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resolves++
	if !m.allow {
		m.once.Do(func() { close(m.firstResolve) })
		return contract.OperationOutputReceipt{}, contract.ErrOutputPublicationUnknown
	}
	if err := req.Matches(m.ack); err != nil {
		return contract.OperationOutputReceipt{}, err
	}
	return m.ack, nil
}
func (m *workerMediaACK) enable() { m.mu.Lock(); defer m.mu.Unlock(); m.allow = true }
func (m *workerMediaACK) stats() (int, int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.publishes, m.resolves
}

func TestExecutionOutputActualWorkerReclaimsNewEpochAndOnlyResolvesOriginalACK(t *testing.T) {
	f := newExecutionOutputFixture(t)
	body := []byte("original staged worker output")
	sum := sha256.Sum256(body)
	manifest, err := f.store.ReadOutputManifest(t.Context(), f.req.Publication.Output.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Artifacts[0].Size = int64(len(body))
	manifest.Artifacts[0].SHA256 = hex.EncodeToString(sum[:])
	raw, _ := json.Marshal(manifest)
	if err := f.admin.Exec(`UPDATE operation.provider_result SET result=?::jsonb WHERE operation_id=?`, string(raw), manifest.Identity.OperationID).Error; err != nil {
		t.Fatal(err)
	}
	artifacts := &workerArtifacts{identity: manifest.Identity, artifact: manifest.Artifacts[0], body: body}
	original := &unusedOriginalArchive{}
	media := &workerMediaACK{ack: f.ack, firstResolve: make(chan struct{})}
	makeWorker := func() *taskruntime.Worker {
		finalizer := settlement.NewOperationFinalizer(f.execution, executionpg.NewStore(f.execution), billingpg.NewStore(f.execution))
		store := executionpg.NewOutputDeliveryStore(f.execution, finalizer)
		delivery, err := executionapp.NewOutputDelivery(store, media)
		if err != nil {
			t.Fatal(err)
		}
		activities, err := outputworker.NewActivities(store, artifacts, delivery, original)
		if err != nil {
			t.Fatal(err)
		}
		handlers := map[string]taskruntime.Handler{}
		if err := outputworker.RegisterGeneratedMedia(handlers, activities); err != nil {
			t.Fatal(err)
		}
		return &taskruntime.Worker{Store: taskruntime.NewStore(f.execution).ForBusiness(manifest.Identity.OperationID.String()), Handlers: handlers, Concurrency: 1, Lease: 600 * time.Millisecond, Poll: 10 * time.Millisecond}
	}
	if err := f.execution.Exec(`INSERT INTO infra.task_work(service_owner,kind,business_id,next_run_at,reference) VALUES('execution','operation.ingest',?,now(),'null')`, manifest.Identity.OperationID.String()).Error; err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	first, stopFirst := context.WithCancel(ctx)
	firstDone := make(chan error, 1)
	worker := makeWorker()
	go func() { firstDone <- worker.Run(first) }()
	firstJoined := false
	defer func() {
		stopFirst()
		if !firstJoined {
			if err := <-firstDone; err != nil && !errors.Is(err, context.Canceled) {
				t.Error(err)
			}
		}
	}()
	select {
	case <-media.firstResolve:
	case <-ctx.Done():
		t.Fatal("first actual worker never persisted/transferred original request", ctx.Err())
	}
	stopFirst()
	firstErr := <-firstDone
	firstJoined = true
	if firstErr != nil && !errors.Is(firstErr, context.Canceled) {
		t.Fatal(firstErr)
	}
	saved, err := f.store.ReadOutputDelivery(ctx, manifest.Identity.OperationID, 1)
	if err != nil || saved == nil || !saved.Attempted || saved.Receipt != nil {
		t.Fatalf("lost original ACK responsibility: %+v %v", saved, err)
	}
	originalWorker := saved.Request.Publication.Output.Worker
	media.enable()
	second, stopSecond := context.WithCancel(ctx)
	secondDone := make(chan error, 1)
	restarted := makeWorker()
	go func() { secondDone <- restarted.Run(second) }()
	defer func() {
		stopSecond()
		if err := <-secondDone; err != nil && !errors.Is(err, context.Canceled) {
			t.Error(err)
		}
	}()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var state struct {
			Status        string
			Epoch         int64
			Completed     bool
			SettledMicros int64
		}
		if err := f.execution.WithContext(ctx).Raw(`SELECT o.status,o.settled_micros,t.epoch,t.completed_at IS NOT NULL AS completed FROM operation.operation o JOIN infra.task_work t ON t.kind='operation.ingest' AND t.business_id=o.id::text WHERE o.id=?`, manifest.Identity.OperationID).Scan(&state).Error; err != nil {
			t.Fatal(err)
		}
		if state.Completed {
			if state.Status != "completed" || state.SettledMicros != 1234 || state.Epoch <= originalWorker.Epoch {
				t.Fatalf("new claim did not atomically adopt original result: %+v", state)
			}
			break
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal("restarted actual worker did not recover original ACK", ctx.Err())
		}
	}
	preserved, err := f.store.ReadOutputDelivery(ctx, manifest.Identity.OperationID, 1)
	if err != nil || preserved == nil || preserved.Receipt == nil || preserved.Request.Publication.Output.Worker != originalWorker {
		t.Fatalf("recovery rewrote original worker evidence: %+v %v", preserved, err)
	}
	publishes, resolves := media.stats()
	if artifacts.opens.Load() != 1 || publishes != 1 || resolves < 2 || original.calls.Load() != 0 {
		t.Fatalf("worker repeated source or wrong responsibility: opens=%d publishes=%d resolves=%d originals=%d", artifacts.opens.Load(), publishes, resolves, original.calls.Load())
	}
}
