package microservices_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc64"
	"image"
	"image/color"
	"image/png"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	billingpg "github.com/StephenQiu30/glowloom-server/backend/execution/billing/adapter/postgres"
	outputrpc "github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/mediaoutput"
	executionpg "github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/settlement"
	executionapp "github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
	contract "github.com/StephenQiu30/glowloom-server/backend/internal/contract/outputpublication"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
	mediarpc "github.com/StephenQiu30/glowloom-server/backend/media/adapter/grpcpublication"
	mediaobjects "github.com/StephenQiu30/glowloom-server/backend/media/adapter/objectstorage"
	mediapg "github.com/StephenQiu30/glowloom-server/backend/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/media/application"
)

// publicationCOS drives the real SDK against only a synthetic loopback TLS peer.
// Conditional writes, streamed bytes, metadata hashes and native Stat are real.
func publicationCOS(t *testing.T) (*objectstorage.Registry, mediaapp.StorageProfile, func() int) {
	t.Helper()
	type stored struct {
		body      []byte
		mime, sha string
	}
	var mu sync.Mutex
	entries := map[string]stored{}
	writes := 0
	bucket := "publication-" + uuid.NewString()[:8] + "-1250000000"
	host := bucket + ".cos.ap-beijing.myqcloud.com"
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Host != host {
			http.Error(w, "wrong synthetic bucket", http.StatusConflict)
			return
		}
		if r.Method == http.MethodPut {
			if r.Header.Get("X-Cos-Forbid-Overwrite") != "true" {
				http.Error(w, "conditional create required", 400)
				return
			}
			if _, exists := entries[r.URL.Path]; exists {
				w.WriteHeader(http.StatusConflict)
				_, _ = io.WriteString(w, `<Error><Code>FileAlreadyExists</Code></Error>`)
				return
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "source bytes", 400)
				return
			}
			sum := sha256.Sum256(body)
			sha := hex.EncodeToString(sum[:])
			if sha != r.Header.Get("X-Cos-Meta-Sha256") {
				http.Error(w, "real SHA mismatch", 400)
				return
			}
			entries[r.URL.Path] = stored{body, r.Header.Get("Content-Type"), sha}
			writes++
			w.Header().Set("ETag", `"synthetic"`)
			w.Header().Set("X-Cos-Hash-Crc64ecma", strconv.FormatUint(crc64.Checksum(body, crc64.MakeTable(crc64.ECMA)), 10))
			w.WriteHeader(200)
			return
		}
		object, exists := entries[r.URL.Path]
		if !exists {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", object.mime)
		w.Header().Set("Content-Length", strconv.Itoa(len(object.body)))
		w.Header().Set("X-Cos-Meta-Sha256", object.sha)
		w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		w.Header().Set("ETag", `"synthetic"`)
		if r.Method == http.MethodGet {
			_, _ = w.Write(object.body)
		} else if r.Method != http.MethodHead {
			http.Error(w, "unsupported method", http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(server.Close)
	target, _ := url.Parse(server.URL)
	transport := server.Client().Transport.(*http.Transport).Clone()
	transport.TLSClientConfig = transport.TLSClientConfig.Clone()
	transport.TLSClientConfig.ServerName = target.Hostname()
	transport.TLSClientConfig.MinVersion = tls.VersionTLS13
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != host+":443" {
			return nil, fmt.Errorf("refused non-fixture SDK destination")
		}
		return (&net.Dialer{}).DialContext(ctx, network, target.Host)
	}
	previous := http.DefaultTransport
	http.DefaultTransport = transport
	t.Cleanup(func() { http.DefaultTransport = previous; transport.CloseIdleConnections() })
	t.Setenv("OUTPUT_COS_ACCESS", "synthetic-access")
	t.Setenv("OUTPUT_COS_SECRET", "synthetic-secret")
	id := "output-" + uuid.NewString()
	endpoint := "https://" + host
	profile := config.StorageProfile{Provider: "cos", Mode: "read_write", Endpoint: endpoint, SigningEndpoint: endpoint, Bucket: bucket, Region: "ap-beijing", Credentials: config.StorageCredentialsRef{Source: "env", AccessKeyEnv: "OUTPUT_COS_ACCESS", SecretKeyEnv: "OUTPUT_COS_SECRET"}}
	cfg := config.ObjectStorageConfig{SchemaVersion: 1, Revision: "1", DefaultWriteProfile: id, Profiles: map[string]config.StorageProfile{id: profile}, Upload: config.StorageUploadPolicy{Mode: "relay", SignedURLTTL: 10 * time.Minute, SessionTTL: 24 * time.Hour, MultipartThresholdBytes: 32 << 20, PartSizeBytes: 8 << 20, PartConcurrency: 3, StagingRetention: 48 * time.Hour}, Access: config.StorageAccessPolicy{PreviewTTL: 5 * time.Minute, MaxSignedURLTTL: 15 * time.Minute}, Limits: config.StorageLimits{MaxImageBytes: 20 << 20, MaxVideoBytes: 500 << 20, MaxAudioBytes: 100 << 20, MaxModelBytes: 64 << 20, MaxDocumentBytes: 20 << 20}}
	registry, err := objectstorage.OpenProfiles(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registry.Close)
	return registry, mediaapp.StorageProfile{ID: id, Provider: "cos", Endpoint: endpoint, SigningEndpoint: endpoint, Bucket: bucket, Region: "ap-beijing"}, func() int { mu.Lock(); defer mu.Unlock(); return writes }
}

type lostOutputACK struct {
	mediarpc.Publisher
	once     sync.Once
	lost     atomic.Bool
	failures chan error
}

func (s *lostOutputACK) Publish(ctx context.Context, req contract.Request, body io.Reader) (contract.OperationOutputReceipt, error) {
	receipt, err := s.Publisher.Publish(ctx, req, body)
	if err != nil {
		s.failures <- err
		return receipt, err
	}
	lost := false
	s.once.Do(func() { lost = true; s.lost.Store(true) })
	if lost {
		return contract.OperationOutputReceipt{}, status.Error(codes.Unavailable, "synthetic lost archive ACK")
	}
	return receipt, nil
}

func TestOperationOutputRealArchiveLostACKAndExecutionRestart(t *testing.T) {
	f := newExecutionOutputFixture(t)
	imageData := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			imageData.Set(x, y, color.RGBA{R: uint8(x * 3), G: uint8(y * 3), B: 100, A: 255})
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, imageData); err != nil {
		t.Fatal(err)
	}
	body := encoded.Bytes()
	sum := sha256.Sum256(body)
	result, err := f.store.ReadOutputManifest(t.Context(), f.req.Publication.Output.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	result.Artifacts[0].Size = int64(len(body))
	result.Artifacts[0].SHA256 = hex.EncodeToString(sum[:])
	raw, _ := json.Marshal(result)
	if err := f.admin.Exec(`UPDATE operation.provider_result SET result=?::jsonb WHERE operation_id=?`, string(raw), result.Identity.OperationID).Error; err != nil {
		t.Fatal(err)
	}
	f.req.Publication.Output.ByteSize = int64(len(body))
	f.req.Publication.Output.SHA256 = hex.EncodeToString(sum[:])
	f.req.Publication.Output.Provider.ManifestSHA256 = outputTestHash(t, result)
	f.req.Publication.Command.SHA256 = f.req.Publication.Output.Digest()
	f.req.Command.SHA256 = f.req.Digest()
	if _, err := f.store.PrepareOutputDelivery(t.Context(), f.req); err != nil {
		t.Fatal(err)
	}
	if started, err := f.store.StartOutputDelivery(t.Context(), f.req); err != nil || !started {
		t.Fatalf("first durable transfer=%v err=%v", started, err)
	}
	proofTransport, mediaPeer, _ := ownerPeerTLS(t, "media")
	proofServer, err := outputrpc.NewProofServer(f.store, strings.Repeat("p", 40), proofTransport)
	if err != nil {
		t.Fatal(err)
	}
	proofAddress := serveFrozenRPC(t, proofServer)
	proof, err := mediarpc.NewProofClient(mediarpc.ProofTarget{Address: proofAddress, Token: strings.Repeat("p", 40), Transport: mediaPeer})
	if err != nil {
		t.Fatal(err)
	}
	registry, profile, writes := publicationCOS(t)
	routes := mediapg.NewStorageRoutes(f.media)
	if err := routes.RegisterProfiles(t.Context(), []mediaapp.StorageProfile{profile}); err != nil {
		t.Fatal(err)
	}
	if err := f.admin.Exec(`INSERT INTO media.storage_work_intent(id,kind,owner_id,work_id,profile_id,bucket,config_revision) VALUES(?,'operation',?,?,?,?,?)`, uuid.New(), f.req.Publication.Output.ProjectID, f.req.Publication.Output.OperationID, profile.ID, profile.Bucket, "synthetic output v1").Error; err != nil {
		t.Fatal(err)
	}
	publisher := mediaapp.NewOutputPublisher(mediapg.NewOutputPublicationStore(f.media, proof), mediaapp.NewGeneratedOutputArchiver(routes, mediaobjects.NewGeneratedObjects(registry, routes), mediaflow.FFProber{}, mediaflow.FFRenderer{}), mediaapp.NewProviderOutputReviewer())
	lossy := &lostOutputACK{Publisher: publisher, failures: make(chan error, 1)}
	mediaTransport, executionPeer, wrongPeer := ownerPeerTLS(t, "execution")
	server, err := mediarpc.NewListener(lossy, strings.Repeat("m", 40), 1, mediaTransport)
	if err != nil {
		t.Fatal(err)
	}
	address := serveFrozenRPC(t, server)
	client, err := outputrpc.New(outputrpc.Target{Address: address, Token: strings.Repeat("m", 40), Transport: executionPeer})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Publish(t.Context(), f.req.Publication, bytes.NewReader(body)); !errors.Is(err, executionapp.ErrInvalidProviderCall) && err == nil {
		t.Fatal("lost final ACK returned success")
	}
	if !lossy.lost.Load() || writes() != 3 {
		select {
		case cause := <-lossy.failures:
			t.Fatalf("real archive failed: %v writes=%d", cause, writes())
		default:
		}
		t.Fatalf("actual archive writes=%d lost=%v", writes(), lossy.lost.Load())
	}
	// Execution process restart: only the exact saved original ACK is queried.
	finalizer := settlement.NewOperationFinalizer(f.execution, executionpg.NewStore(f.execution), billingpg.NewStore(f.execution))
	restarted := executionpg.NewOutputDeliveryStore(f.execution, finalizer)
	delivery, err := executionapp.NewOutputDelivery(restarted, client)
	if err != nil {
		t.Fatal(err)
	}
	reopened := false
	if err := delivery.Deliver(t.Context(), f.req, func(context.Context) (io.ReadCloser, error) {
		reopened = true
		return nil, errors.New("recovery must not resend bytes")
	}); err != nil {
		t.Fatal(err)
	}
	if reopened || writes() != 3 {
		t.Fatalf("recovery resent bytes: reopened=%v writes=%d", reopened, writes())
	}
	var facts struct {
		Status                 string
		SettledMicros          int64
		Outputs, MediaReceipts int
	}
	if err := f.admin.Raw(`SELECT status,settled_micros,(SELECT count(*) FROM operation.operation_output WHERE operation_id=o.id) AS outputs,(SELECT count(*) FROM media.operation_output_receipt WHERE operation_id=o.id AND state='published') AS media_receipts FROM operation.operation o WHERE id=?`, result.Identity.OperationID).Scan(&facts).Error; err != nil {
		t.Fatal(err)
	}
	if facts.Status != "completed" || facts.SettledMicros != 1234 || facts.Outputs != 1 || facts.MediaReceipts != 1 {
		t.Fatalf("real archive adoption facts=%+v", facts)
	}
	wrong, err := outputrpc.New(outputrpc.Target{Address: address, Token: strings.Repeat("m", 40), Transport: wrongPeer})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wrong.Resolve(t.Context(), f.req.Publication); err == nil {
		t.Fatal("foreign mTLS peer recovered receipt")
	}
	// Recovery still binds the complete exact original immutable request.
	ack, err := client.Resolve(t.Context(), f.req.Publication)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.req.Publication.Matches(ack); err != nil {
		t.Fatal(err)
	}
}
