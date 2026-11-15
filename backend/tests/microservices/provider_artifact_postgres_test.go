package microservices_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	executionpg "github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/postgres"
	executionapp "github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
	contract "github.com/StephenQiu30/glowloom-server/backend/internal/contract/providerartifact"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/artifactrpc"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	mediaobjects "github.com/StephenQiu30/glowloom-server/backend/media/adapter/objectstorage"
	mediapg "github.com/StephenQiu30/glowloom-server/backend/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/media/application"
)

type artifactLostACK struct {
	artifactrpc.MediaService
	calls, resolves atomic.Int32
	first           atomic.Bool
}

func (m *artifactLostACK) StageArtifact(ctx context.Context, req contract.Request) (contract.Receipt, error) {
	m.calls.Add(1)
	ack, err := m.MediaService.StageArtifact(ctx, req)
	if err != nil {
		return ack, err
	}
	if !m.first.Swap(true) {
		return contract.Receipt{}, writefence.ErrUnavailable
	}
	return ack, nil
}
func (m *artifactLostACK) ResolveArtifact(ctx context.Context, req contract.Request) (contract.Receipt, error) {
	m.resolves.Add(1)
	return m.MediaService.ResolveArtifact(ctx, req)
}

func TestProviderArtifactActualWorkerStageLostACKResolveAndBoundedChunkGC(t *testing.T) {
	runProviderArtifactStage(t, false)
}
func TestProviderArtifactActualMusicRawCanonicalLostACKAndRestart(t *testing.T) {
	runProviderArtifactStage(t, true)
}

func runProviderArtifactStage(t *testing.T, music bool) {
	t.Helper()
	f, o := supplierObservationFixture(t)
	if music {
		for _, query := range []string{`UPDATE operation.operation SET capability='audio.generate' WHERE id=?`, `UPDATE catalog.provider_config_version SET config='{"adapter_key":"volcengine_music"}' WHERE id=(SELECT r.config_version_id FROM catalog.model_release r JOIN operation.provider_binding b ON b.release_id=r.id WHERE b.operation_id=?)`} {
			if err := f.admin.Exec(query, o.Identity.OperationID).Error; err != nil {
				t.Fatal(err)
			}
		}
		o.ImageCount = 0
		o.URLCount = 1
	}
	if err := executionpg.NewStore(f.execution).RecordSupplierObservation(t.Context(), o); err != nil {
		t.Fatal(err)
	}
	repo := executionpg.NewArtifactStore(f.execution)
	sourceTLS, mediaPeer, _ := ownerPeerTLS(t, "media")
	sourceServer, err := artifactrpc.NewSourceListener(repo, strings.Repeat("s", 40), 2, sourceTLS)
	if err != nil {
		t.Fatal(err)
	}
	source, err := artifactrpc.NewClient(artifactrpc.Target{Address: serveFrozenRPC(t, sourceServer), Token: strings.Repeat("s", 40), Transport: mediaPeer})
	if err != nil {
		t.Fatal(err)
	}
	registry, profile, writes := publicationCOS(t)
	routes := mediapg.NewStorageRoutes(f.media)
	if err := routes.RegisterProfiles(t.Context(), []mediaapp.StorageProfile{profile}); err != nil {
		t.Fatal(err)
	}
	if err := f.admin.Exec(`INSERT INTO media.storage_work_intent(id,kind,owner_id,work_id,profile_id,bucket,config_revision) VALUES(?,'operation',?,?,?,?,?)`, uuid.New(), o.Identity.ProjectID, o.Identity.OperationID, profile.ID, profile.Bucket, "original stage route").Error; err != nil {
		t.Fatal(err)
	}
	service, err := mediaapp.NewProviderArtifacts(mediapg.NewProviderArtifactStore(f.media), source, mediaobjects.NewGeneratedObjects(registry, routes), mediaobjects.NewRoutedReadObjects(registry), mediaflow.ProviderArtifactProcessor{})
	if err != nil {
		t.Fatal(err)
	}
	lossy := &artifactLostACK{MediaService: service}
	mediaTLS, execPeer, _ := ownerPeerTLS(t, "execution")
	server, err := artifactrpc.NewMediaListener(lossy, strings.Repeat("m", 40), 2, mediaTLS)
	if err != nil {
		t.Fatal(err)
	}
	client, err := artifactrpc.NewClient(artifactrpc.Target{Address: serveFrozenRPC(t, server), Token: strings.Repeat("m", 40), Transport: execPeer})
	if err != nil {
		t.Fatal(err)
	}
	body := bytes.Repeat([]byte{23}, contract.ChunkSize*2+17)
	mime, expectedWrites := "image/png", 1
	if music {
		body = providerMusicWAV()
		mime = "audio/wave"
		expectedWrites = 2
	}
	delivery := executionapp.NewArtifactDelivery(repo, client)
	ownedResults, err := executionapp.NewOwnedManagedResults(executionpg.NewStore(f.execution), delivery)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.admin.Exec(`DELETE FROM operation.provider_result WHERE operation_id=?`, o.Identity.OperationID).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.execution.Exec(`UPDATE infra.task_work SET owner=NULL,lease_until=NULL,next_run_at=now() WHERE kind='operation.run' AND business_id=?`, o.Identity.OperationID.String()).Error; err != nil {
		t.Fatal(err)
	}
	outcome := make(chan error, 1)
	worker := &taskruntime.Worker{Store: taskruntime.NewStore(f.execution).ForBusiness(o.Identity.OperationID.String()), Concurrency: 1, Lease: time.Second, Poll: 10 * time.Millisecond, Handlers: map[string]taskruntime.Handler{"operation.run": func(ctx context.Context, _ taskruntime.Job) (time.Time, error) {
		sum := sha256.Sum256(body)
		rawArtifact := executionapp.ManagedArtifact{Sequence: 1, Size: int64(len(body)), MIME: mime, SHA256: hex.EncodeToString(sum[:])}
		_, err := ownedResults.StageSource(ctx, o.Identity, rawArtifact, bytes.NewReader(body))
		if !errors.Is(err, writefence.ErrUnavailable) {
			outcome <- errors.New("first transfer must lose ACK: " + errorText(err))
			return time.Time{}, nil
		}
		var retained struct {
			Count, MaxBytes int
			State           string
		}
		if err := f.execution.Raw(`SELECT count(*) AS count,max(octet_length(data)) AS max_bytes,(SELECT state FROM operation.provider_artifact_delivery WHERE operation_id=?) AS state FROM operation.provider_artifact_chunk WHERE delivery_id=(SELECT delivery_id FROM operation.provider_artifact_delivery WHERE operation_id=?)`, o.Identity.OperationID, o.Identity.OperationID).Scan(&retained).Error; err != nil {
			outcome <- err
			return time.Time{}, nil
		}
		if retained.Count != (len(body)+contract.ChunkSize-1)/contract.ChunkSize || retained.MaxBytes != min(len(body), contract.ChunkSize) || retained.State != "prepared" {
			outcome <- errors.New("source lost or unbounded before ACK")
			return time.Time{}, nil
		}
		recovered, err := executionapp.NewOwnedManagedResults(executionpg.NewStore(f.execution), executionapp.NewArtifactDelivery(executionpg.NewArtifactStore(f.execution), client))
		if err != nil {
			outcome <- err
			return time.Time{}, nil
		}
		// Recovery receives only the original dispatch, without supplier bytes.
		if manifest, recoveryErr := recovered.Recover(ctx, o.Identity); recoveryErr != nil || manifest != nil {
			if recoveryErr == nil {
				recoveryErr = errors.New("missing manifest fabricated a completed result")
			}
			outcome <- recoveryErr
			return time.Time{}, nil
		}
		artifact, err := recovered.StageArtifact(ctx, o.Identity, 1, body, mime)
		if err == nil {
			err = recovered.Save(ctx, executionapp.ManagedResult{Identity: o.Identity, State: executionapp.ManagedResultSucceeded, Artifacts: []executionapp.ManagedArtifact{artifact}, Usage: json.RawMessage(`{"billable_requests":1,"output_count":1}`), SupplierUsage: json.RawMessage(`{"original":9007199254740993}`), ReceivedAt: &o.ReceivedAt})
		}
		if err == nil {
			var manifest *executionapp.ManagedResult
			manifest, err = recovered.Recover(ctx, o.Identity)
			if err == nil && (manifest == nil || len(manifest.Artifacts) != 1) {
				err = errors.New("owned manifest did not recover")
			}
		}
		outcome <- err
		return time.Time{}, nil
	}}}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	done := make(chan error, 1)
	go func() { done <- worker.Run(ctx) }()
	defer func() {
		cancel()
		if err := <-done; err != nil {
			t.Error("artifact worker stop", err)
		}
	}()
	select {
	case err := <-outcome:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if lossy.calls.Load() != 1 || lossy.resolves.Load() != 1 || writes() != expectedWrites {
		t.Fatalf("unexpected resend: stage=%d resolve=%d physicalwrites=%d", lossy.calls.Load(), lossy.resolves.Load(), writes())
	}
	var raw []byte
	var chunks int
	var cost int64
	if err := f.execution.Raw(`SELECT request FROM operation.provider_artifact_delivery WHERE operation_id=?`, o.Identity.OperationID).Row().Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var req contract.Request
	if err := json.Unmarshal(raw, &req); err != nil {
		t.Fatal(err)
	}
	if err := f.execution.Raw(`SELECT count(*) FROM operation.provider_artifact_chunk WHERE delivery_id=?`, req.DeliveryID).Row().Scan(&chunks); err != nil {
		t.Fatal(err)
	}
	if chunks != 0 {
		t.Fatalf("ACK did not atomically remove source chunks: %d", chunks)
	}
	if err := f.execution.Raw(`SELECT cost_micros FROM operation.provider_call WHERE operation_id=? AND action='submit'`, o.Identity.OperationID).Row().Scan(&cost); err != nil {
		t.Fatal(err)
	}
	if cost != 1234 {
		t.Fatalf("supplier cost changed: %d", cost)
	}
	reader, err := client.OpenArtifact(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	read, err := io.ReadAll(reader)
	closeErr := reader.Close()
	if err != nil || closeErr != nil || (!music && !bytes.Equal(read, body)) || (music && bytes.Equal(read, body)) {
		t.Fatalf("Media canonical read failed: readerr=%v closeerr=%v size=%d", err, closeErr, len(read))
	}
	if music {
		ack, err := client.ResolveArtifact(t.Context(), req)
		if err != nil {
			t.Fatal(err)
		}
		if req.Processing != "music_mp3" || ack.Canonical == nil || ack.Canonical.MIME != "audio/mpeg" || ack.Canonical.DurationMS < 1000 || ack.Canonical.DurationMS > 1200 || ack.Canonical.ByteSize != int64(len(read)) {
			t.Fatalf("music descriptor incomplete: request=%+v ack=%+v", req, ack)
		}
		sum := sha256.Sum256(read)
		if hex.EncodeToString(sum[:]) != ack.Canonical.SHA256 {
			t.Fatal("canonical bytes differ from immutable ACK")
		}
		var accepted int
		if err := f.media.Raw(`SELECT count(*) FROM media.object_location WHERE owner_intent_id=(SELECT storage_work_intent_id FROM media.provider_artifact_receipt WHERE command_id=?) AND state='accepted'`, req.Command.ID).Row().Scan(&accepted); err != nil {
			t.Fatal(err)
		}
		if accepted != 2 {
			t.Fatalf("raw/canonical ACK preceded physical acceptance: %d", accepted)
		}
	}
	if err := repo.VerifyArtifactSource(t.Context(), req); !errors.Is(err, contract.ErrUnknown) {
		t.Fatalf("deleted source still readable: %v", err)
	}
	for _, forbidden := range []string{"profile", "bucket", "object_key", "ciphertext", "supplier_usage"} {
		if bytes.Contains(raw, []byte(forbidden)) {
			t.Fatalf("physical or supplier facts leaked into stage request: %s", forbidden)
		}
	}
}

func TestProviderArtifactOldWorkerCannotAppendSealOrStartSource(t *testing.T) {
	f, o := supplierObservationFixture(t)
	if err := executionpg.NewStore(f.execution).RecordSupplierObservation(t.Context(), o); err != nil {
		t.Fatal(err)
	}
	repo := executionpg.NewArtifactStore(f.execution)
	body := bytes.Repeat([]byte{19}, contract.ChunkSize*2)
	sum := sha256.Sum256(body)
	artifact := executionapp.ManagedArtifact{Sequence: 1, Size: int64(len(body)), MIME: "image/png", SHA256: hex.EncodeToString(sum[:])}
	if err := f.execution.Exec(`UPDATE infra.task_work SET owner=NULL,lease_until=NULL,next_run_at=now() WHERE kind='operation.run' AND business_id=?`, o.Identity.OperationID.String()).Error; err != nil {
		t.Fatal(err)
	}
	outcome := make(chan error, 1)
	worker := &taskruntime.Worker{Store: taskruntime.NewStore(f.execution).ForBusiness(o.Identity.OperationID.String()), Concurrency: 1, Lease: time.Second, Poll: 10 * time.Millisecond, Handlers: map[string]taskruntime.Handler{"operation.run": func(ctx context.Context, _ taskruntime.Job) (time.Time, error) {
		record, err := repo.PrepareArtifact(ctx, o.Identity, artifact)
		if err == nil {
			err = repo.PutArtifactChunk(ctx, record.Request, 0, body[:contract.ChunkSize])
		}
		if err != nil {
			outcome <- err
			return time.Time{}, nil
		}
		if err := f.execution.Exec(`UPDATE infra.task_work SET owner='replacement-worker',epoch=epoch+1 WHERE kind='operation.run' AND business_id=?`, o.Identity.OperationID.String()).Error; err != nil {
			outcome <- err
			return time.Time{}, nil
		}
		for _, check := range []func() error{func() error { return repo.PutArtifactChunk(ctx, record.Request, 1, body[contract.ChunkSize:]) }, func() error { return repo.SealArtifact(ctx, record.Request) }, func() error { _, err := repo.StartArtifact(ctx, record.Request); return err }} {
			if err := check(); !errors.Is(err, taskruntime.ErrLeaseLost) {
				outcome <- fmt.Errorf("old artifact worker accepted or misclassified: %w", err)
				return time.Time{}, nil
			}
		}
		outcome <- nil
		return time.Time{}, nil
	}}}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	done := make(chan error, 1)
	go func() { done <- worker.Run(ctx) }()
	defer func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	select {
	case err := <-outcome:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func TestProviderArtifactMissingFrozenRouteFailsWithoutCreatingIntent(t *testing.T) {
	f := newFrozenFixture(t)
	req := artifactRequest(t, []byte("original bytes"))
	req.ProjectID = f.request.ProjectID
	req.Command.Authorities = f.request.Command.Authorities
	req.Command.SHA256 = req.Digest()
	store := mediapg.NewProviderArtifactStore(f.media)
	if _, err := store.PrepareProviderArtifact(t.Context(), req); !errors.Is(err, mediaapp.ErrStorageRouteConflict) {
		t.Fatalf("missing original route not rejected: %v", err)
	}
	var count int
	if err := f.media.Raw(`SELECT count(*) FROM media.storage_work_intent WHERE kind='operation' AND owner_id=? AND work_id=?`, req.ProjectID, req.OperationID).Row().Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("generation selected a new route")
	}
}

func errorText(err error) string {
	if err == nil {
		return "nil"
	}
	return err.Error()
}
