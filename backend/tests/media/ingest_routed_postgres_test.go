package media_test

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"hash/crc64"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	objects "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/objectstorage"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	flow "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/workflow"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	platformdb "github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

type controlledTOSObject struct {
	body      []byte
	mime, sha string
}

// This wire implements only official TOS request/response semantics locally.
// Every SDK connection is pinned to the TLS fixture; no real cloud is contacted.
func controlledGeneratedTOS(t *testing.T) (*objectstorage.Registry, *int, config.ObjectStorageConfig) {
	t.Helper()
	var mu sync.Mutex
	entries := map[string]controlledTOSObject{}
	writes := new(int)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Host != "controlled-fixture.tos-fixture.example.invalid" {
			http.Error(w, "wrong frozen bucket", http.StatusConflict)
			return
		}
		key := r.URL.Path
		if r.Method == http.MethodPut {
			if r.Header.Get("X-Tos-Forbid-Overwrite") != "true" {
				http.Error(w, "missing conditional create", 400)
				return
			}
			if _, found := entries[key]; found {
				http.Error(w, "object exists", http.StatusConflict)
				return
			}
			b, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "body", 400)
				return
			}
			sum := sha256.Sum256(b)
			sha := hex.EncodeToString(sum[:])
			if sha != r.Header.Get("X-Tos-Meta-Sha256") {
				http.Error(w, "integrity", 400)
				return
			}
			entries[key] = controlledTOSObject{body: b, mime: r.Header.Get("Content-Type"), sha: sha}
			*writes++
			w.Header().Set("ETag", `"controlled"`)
			w.Header().Set("X-Tos-Hash-Crc64ecma", strconv.FormatUint(crc64.Checksum(b, crc64.MakeTable(crc64.ECMA)), 10))
			w.WriteHeader(200)
			return
		}
		row, found := entries[key]
		if !found {
			http.Error(w, "missing", 404)
			return
		}
		w.Header().Set("Content-Type", row.mime)
		w.Header().Set("Content-Length", strconv.Itoa(len(row.body)))
		w.Header().Set("X-Tos-Meta-Sha256", row.sha)
		w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		w.Header().Set("ETag", `"controlled"`)
		if r.Method == http.MethodGet {
			_, _ = w.Write(row.body)
		} else if r.Method != http.MethodHead {
			http.Error(w, "unsupported", http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(server.Close)
	endpoint, _ := url.Parse(server.URL)
	transport := server.Client().Transport.(*http.Transport).Clone()
	transport.TLSClientConfig = transport.TLSClientConfig.Clone()
	transport.TLSClientConfig.ServerName = endpoint.Hostname()
	transport.TLSClientConfig.MinVersion = tls.VersionTLS12
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if !strings.HasSuffix(address, ".example.invalid:443") {
			return nil, fmt.Errorf("refused non-fixture SDK destination")
		}
		return (&net.Dialer{}).DialContext(ctx, network, endpoint.Host)
	}
	previous := http.DefaultTransport
	http.DefaultTransport = transport
	t.Cleanup(func() { http.DefaultTransport = previous; transport.CloseIdleConnections() })
	t.Setenv("CONTROLLED_TOS_ACCESS", "synthetic-access")
	t.Setenv("CONTROLLED_TOS_SECRET", "synthetic-secret")
	profile := config.StorageProfile{Provider: "tos", Mode: "read_write", Endpoint: "https://tos-fixture.example.invalid", SigningEndpoint: "https://tos-fixture.example.invalid", Bucket: "controlled-fixture", Region: "cn-beijing", Credentials: config.StorageCredentialsRef{Source: "env", AccessKeyEnv: "CONTROLLED_TOS_ACCESS", SecretKeyEnv: "CONTROLLED_TOS_SECRET"}}
	newer := profile
	newer.Bucket = "unused-new-default"
	cfg := config.ObjectStorageConfig{SchemaVersion: 1, Revision: "controlled-v1", DefaultWriteProfile: "tos-new", Profiles: map[string]config.StorageProfile{"tos-frozen": profile, "tos-new": newer}, Upload: config.StorageUploadPolicy{Mode: "relay", SignedURLTTL: 10 * time.Minute, SessionTTL: 24 * time.Hour, MultipartThresholdBytes: 32 << 20, PartSizeBytes: 8 << 20, PartConcurrency: 3, StagingRetention: 48 * time.Hour}, Access: config.StorageAccessPolicy{PreviewTTL: 5 * time.Minute, MaxSignedURLTTL: 15 * time.Minute}, Limits: config.StorageLimits{MaxImageBytes: 20 << 20, MaxVideoBytes: 500 << 20, MaxAudioBytes: 100 << 20, MaxModelBytes: 64 << 20, MaxDocumentBytes: 20 << 20}}
	registry, err := objectstorage.OpenProfiles(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registry.Close)
	return registry, writes, cfg
}

func routedGeneratedDatabase(t *testing.T) (*gorm.DB, *gorm.DB) {
	t.Helper()
	dsn := os.Getenv("GLOWLOOM_TEST_SUBJECT_DB_DSN")
	if dsn == "" {
		t.Skip("requires isolated initialized subject-alignment PostgreSQL")
	}
	u, err := url.Parse(dsn)
	if err != nil || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") || !strings.HasPrefix(u.Path, "/glowloom_subject_alignment_") {
		t.Fatal("requires task-owned loopback database")
	}
	owner, err := platformdb.Open(t.Context(), dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close() })
	q := u.Query()
	q.Set("options", "-c role=glowloom_app")
	u.RawQuery = q.Encode()
	runtime, err := platformdb.Open(t.Context(), u.String(), noop.NewTracerProvider())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	return owner.DB, runtime.DB
}

func TestRoutedIngestRealPostgresAndNativeTOSPreserveFrozenWork(t *testing.T) {
	owner, runtime := routedGeneratedDatabase(t)
	actor, project := mediaStoreProject(t, owner)
	op := seedIngestOperation(t, owner, actor, project, "image")
	registry, writes, _ := controlledGeneratedTOS(t)
	routes := pgmedia.NewStorageRoutes(owner)
	if err := routes.RegisterProfiles(t.Context(), []media.StorageProfile{{ID: "tos-frozen", Provider: "tos", Endpoint: "https://tos-fixture.example.invalid", SigningEndpoint: "https://tos-fixture.example.invalid", Region: "cn-beijing", Bucket: "controlled-fixture"}}); err != nil {
		t.Fatal(err)
	}
	intent := uuid.New()
	if err := owner.Exec(`INSERT INTO media.storage_work_intent(id,kind,owner_id,work_id,profile_id,bucket,config_revision) VALUES(?,'operation',?,?,'tos-frozen','controlled-fixture','controlled-v1')`, intent, project, op).Error; err != nil {
		t.Fatal(err)
	}
	runtimeRoutes := pgmedia.NewStorageRoutes(runtime)
	repo := pgmedia.NewScopedIngestRepository(runtime, app.NewMediaWorkerScopeAccess)
	source := newRoutedIngestFixture()
	service := media.NewRoutedIngestService(repo, source, flow.FFProber{}, flow.FFRenderer{}, objects.NewGeneratedObjects(registry, runtimeRoutes), runtimeRoutes)
	input := media.IngestInput{OperationID: op.String(), SeqNo: 1, URL: "local synthetic PNG"}
	out, err := service.Ingest(t.Context(), input)
	if err != nil {
		t.Fatal("routed candidate", err)
	}
	if out.ObjectLocation == nil || out.ObjectLocation.Ref.ProfileID != "tos-frozen" || *writes != 3 {
		t.Fatal("lost frozen original/renditions")
	}
	var accepted int64
	if err := owner.Raw(`SELECT count(*) FROM media.object_location WHERE owner_intent_id=? AND state='accepted'`, intent).Scan(&accepted).Error; err != nil || accepted != 3 {
		t.Fatal("non-atomic physical acceptance", accepted, err)
	}
	if err := repo.RecordModeration(t.Context(), op, uuid.MustParse(out.OutputID), domain.ModerationPassed, ""); err != nil {
		t.Fatal(err)
	}
	if err := repo.CompleteGeneratedWork(t.Context(), op, 1); err != nil {
		t.Fatal("complete all managed outputs", err)
	}
	replay, err := service.Ingest(t.Context(), input)
	if err != nil || replay.OutputID != out.OutputID || *writes != 3 {
		t.Fatal("retry wrote a new default", err)
	}
	file, err := registry.Get(t.Context(), objectstorage.Ref{ProfileID: out.ObjectLocation.Ref.ProfileID, Bucket: out.ObjectLocation.Ref.Bucket, Key: out.ObjectKey})
	if err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(file)
	closeErr := file.Close()
	if err != nil || closeErr != nil || int64(len(b)) != out.ByteSize {
		t.Fatal("native TOS readback", err, closeErr)
	}
	if err := owner.Exec(`UPDATE workspace.project SET is_delete=true WHERE id=?`, project).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.Ingest(t.Context(), input); err == nil {
		t.Fatal("replay bypassed live project authority")
	}
}
