package media_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"
	"gorm.io/gorm"

	auditevent "github.com/StephenQiu30/lanverse/backend/internal/audit/adapter/event"
	auditapp "github.com/StephenQiu30/lanverse/backend/internal/audit/application"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	inboxpg "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/adapter/postgres"
	inboxapp "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
	mediapg "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	toolevent "github.com/StephenQiu30/lanverse/backend/internal/mediatool/adapter/event"
	toolff "github.com/StephenQiu30/lanverse/backend/internal/mediatool/adapter/ffmpeg"
	toolhttp "github.com/StephenQiu30/lanverse/backend/internal/mediatool/adapter/http"
	toolpg "github.com/StephenQiu30/lanverse/backend/internal/mediatool/adapter/postgres"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/adapter/whisper"
	toolflow "github.com/StephenQiu30/lanverse/backend/internal/mediatool/adapter/workflow"
	toolapp "github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
	tooldomain "github.com/StephenQiu30/lanverse/backend/internal/mediatool/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstorage"
)

type speechHTTPFixture struct {
	db        *gorm.DB
	store     *toolpg.TranscriptionStore
	objects   *objectstorage.Client
	actor     identityapp.Principal
	project   uuid.UUID
	assetID   uuid.UUID
	sourceSHA string
	input     toolapp.TranscriptionCreateInput
	router    *gin.Engine
}

func speechFixture(t *testing.T) *speechHTTPFixture {
	t.Helper()
	sample := os.Getenv("LV_TEST_WHISPER_SAMPLE")
	if sample == "" {
		if os.Getenv("LV_TEST_WHISPER_REQUIRED") == "1" {
			t.Fatal("actual official speech fixture is required")
		}
		t.Skip("set actual official LV_TEST_WHISPER_SAMPLE and isolated storage/PG")
	}
	return speechFixtureFrom(t, sample, "audio/wave", "audio")
}
func speechFixtureFrom(t *testing.T, sample, mime, kind string) *speechHTTPFixture {
	t.Helper()
	data, err := os.ReadFile(sample)
	if err != nil {
		t.Fatal("read task-owned official sample", err)
	}
	file, err := os.CreateTemp(t.TempDir(), "official-speech-*.wav")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(data); err != nil {
		t.Fatal(err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(data)
	sha := hex.EncodeToString(hash[:])
	download := &mediaapp.Downloaded{File: file, Size: int64(len(data)), MIMEType: mime, SHA256: sha}
	defer func() { _ = download.Close() }()
	db := mediaStoreDB(t)
	objects := glbTestObjects(t)
	actor, project := mediaStoreProject(t, db)
	upload := mediaapp.NewUploadService(mediapg.NewStore(db), mediaflow.FFUploadProber{}, mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, mediaflow.NewUploadObjects(objects), time.Now)
	uploaded, err := upload.Upload(t.Context(), mediaapp.UploadInput{Actor: actor, Request: mediaapp.UploadRequest{ProjectID: project, Key: uuid.New(), RequestID: uuid.New(), FileName: filepath.Base(sample)}, File: download, LocalReviewConfirmed: true})
	if err != nil {
		t.Fatal("actual reviewed speech upload", err)
	}
	f := &speechHTTPFixture{db: db, store: transcriptionStore(t, db), objects: objects, actor: actor, project: project, assetID: uploaded.Asset.ID, sourceSHA: sha}
	f.input = transcriptionNode(t, db, actor, project, uploaded.Asset.ID, kind)
	f.router = gin.New()
	f.router.Use(httpapi.Middleware("http://localhost:3000"))
	g := f.router.Group("/api")
	g.Use(func(c *gin.Context) { c.Set("principal", actor); c.Next() })
	toolhttp.NewTranscriptionHandler(f.store).Register(g)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var keys []string
		if err := db.WithContext(ctx).Raw(`SELECT object_key FROM media.media_asset WHERE project_id=? UNION ALL SELECT r.object_key FROM media.rendition r JOIN media.media_asset a ON a.id=r.media_asset_id WHERE a.project_id=?`, project, project).Scan(&keys).Error; err != nil {
			t.Error(err)
			return
		}
		var jobs []struct {
			ID      uuid.UUID
			Attempt int
		}
		if err := db.WithContext(ctx).Raw(`SELECT id,attempt FROM mediatool.transcription_job WHERE project_id=?`, project).Scan(&jobs).Error; err != nil {
			t.Error(err)
			return
		}
		for _, job := range jobs {
			for attempt := 1; attempt <= job.Attempt; attempt++ {
				keys = append(keys, path.Join("projects", project.String(), "transcriptions", job.ID.String(), "attempt-"+strconv.Itoa(attempt)+".json"))
			}
		}
		for _, key := range keys {
			if err := objects.Remove(ctx, key); err != nil {
				t.Error("remove exact task-owned speech object")
			}
		}
	})
	return f
}
func (f *speechHTTPFixture) request(t *testing.T, method, url string, key uuid.UUID, body any) *httptest.ResponseRecorder {
	t.Helper()
	var raw []byte
	if body != nil {
		var err error
		raw, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	request := httptest.NewRequestWithContext(t.Context(), method, url, bytes.NewReader(raw))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://localhost:3000")
	if key != uuid.Nil {
		request.Header.Set("Idempotency-Key", key.String())
	}
	response := httptest.NewRecorder()
	f.router.ServeHTTP(response, request)
	return response
}
func (f *speechHTTPFixture) create(t *testing.T) tooldomain.TranscriptionJob {
	t.Helper()
	response := f.request(t, http.MethodPost, "/api/projects/"+f.project.String()+"/media-transcriptions", uuid.New(), f.input)
	var job tooldomain.TranscriptionJob
	if response.Code != 202 || json.Unmarshal(response.Body.Bytes(), &job) != nil {
		t.Fatalf("actual source admission: %d %s", response.Code, response.Body.String())
	}
	return job
}
func (f *speechHTTPFixture) worker(t *testing.T, client toolapp.Transcriber) *toolapp.TranscriptionWorker {
	t.Helper()
	prepare, err := toolff.NewAudioPreprocessor(mediaflow.FFProber{})
	if err != nil {
		t.Fatal(err)
	}
	return toolapp.NewTranscriptionWorker(f.store, toolflow.NewObjects(f.objects), prepare, client)
}

func TestTranscriptionActualHTTPWhisperPrivateDraftSourceAndSRT(t *testing.T) {
	endpoint := os.Getenv("LV_TEST_WHISPER_BASE_URL")
	if endpoint == "" {
		if os.Getenv("LV_TEST_WHISPER_REQUIRED") == "1" {
			t.Fatal("actual native endpoint required")
		}
		t.Skip("set task-owned actual native Whisper endpoint")
	}
	f := speechFixture(t)
	client, err := whisper.NewClient(endpoint, &http.Client{Timeout: toolapp.TranscriptionInferenceTimeout})
	if err != nil {
		t.Fatal(err)
	}
	key := uuid.New()
	url := "/api/projects/" + f.project.String() + "/media-transcriptions"
	accepted := f.request(t, "POST", url, key, f.input)
	var job tooldomain.TranscriptionJob
	if accepted.Code != 202 || json.Unmarshal(accepted.Body.Bytes(), &job) != nil {
		t.Fatalf("admission %d %s", accepted.Code, accepted.Body.String())
	}
	replay := f.request(t, "POST", url, key, f.input)
	if replay.Code != 202 || replay.Body.String() != accepted.Body.String() {
		t.Fatal("permanent admission receipt changed")
	}
	changed := f.input
	changed.Language = "zh"
	if response := f.request(t, "POST", url, key, changed); response.Code != 409 {
		t.Fatal("changed idempotency body admitted")
	}
	stale := f.input
	stale.Revision++
	if response := f.request(t, "POST", url, uuid.New(), stale); response.Code != 409 {
		t.Fatalf("stale source admitted %d", response.Code)
	}
	worker := f.worker(t, client)
	activities := toolflow.NewTranscriptionActivities(worker, f.store)
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterActivityWithOptions(activities.TranscribeSpeech, activity.RegisterOptions{Name: toolflow.TranscribeActivity})
	env.RegisterActivityWithOptions(activities.FailTranscriptionWorkflow, activity.RegisterOptions{Name: toolflow.TranscriptionFailureActivity})
	env.ExecuteWorkflow(toolflow.TranscriptionWorkflow, toolapp.TranscriptionWorkID{JobID: job.ID, Attempt: 1})
	if err := env.GetWorkflowError(); err != nil {
		t.Fatal("actual native activity", err)
	}
	var final tooldomain.TranscriptionJob
	if err := env.GetWorkflowResult(&final); err != nil || final.Status != tooldomain.TranscriptionSucceeded || final.Progress != 100 || final.ResultSHA256 == nil {
		t.Fatalf("actual completion %+v %v", final, err)
	}
	base := "/api/media-transcriptions/" + job.ID.String()
	scope := "?project_id=" + f.project.String()
	response := f.request(t, "GET", base+"/result"+scope, uuid.Nil, nil)
	var result toolapp.TranscriptionResult
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &result) != nil || result.SourceAssetID != f.assetID || result.SourceAssetRevision != 1 || result.SourceSHA256 != f.sourceSHA || result.Revision != final.Revision || result.Draft.Language != "english" || !strings.Contains(strings.ToLower(result.SRT), "ask not") {
		t.Fatalf("actual frozen subtitle result %d %s", response.Code, response.Body.String())
	}
	raw, err := json.Marshal(result.Draft)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != result.SHA256 || result.SHA256 != *final.ResultSHA256 {
		t.Fatal("draft hash does not bind recognized cues")
	}
	attachment := f.request(t, "GET", base+"/subtitles"+scope, uuid.Nil, nil)
	if attachment.Code != 200 || attachment.Body.String() != result.SRT || !strings.HasPrefix(attachment.Header().Get("Content-Disposition"), "attachment;") {
		t.Fatal("actual SRT attachment")
	}
	restored := f.request(t, "GET", url+"?canvas_id="+f.input.CanvasID.String()+"&node_id="+f.input.NodeID.String(), uuid.Nil, nil)
	var page toolapp.TranscriptionPage
	if restored.Code != 200 || json.Unmarshal(restored.Body.Bytes(), &page) != nil || len(page.Items) != 1 || page.Items[0].Status != tooldomain.TranscriptionSucceeded {
		t.Fatal("durable source-filtered restore")
	}
	completed, err := worker.Execute(t.Context(), toolapp.TranscriptionWorkID{JobID: job.ID, Attempt: 1, ExecutionID: uuid.New()})
	if err != nil || completed.Revision != final.Revision {
		t.Fatal("completed job re-submitted", err)
	}
	speechAuditEvents(t, f.db, f.project)
	if err := f.db.Exec(`UPDATE media.media_asset SET moderation_status='pending' WHERE id=?`, f.assetID).Error; err != nil {
		t.Fatal(err)
	}
	if changed := f.request(t, "GET", base+"/result"+scope, uuid.Nil, nil); changed.Code != 409 {
		t.Fatal("changed source still adoptable")
	}
}

func speechAuditEvents(t *testing.T, db *gorm.DB, project uuid.UUID) {
	t.Helper()
	var rows []struct {
		Topic, PartitionKey string
		Payload             []byte
	}
	if err := db.Raw(`SELECT topic,partition_key,payload FROM infra.outbox WHERE topic='lanverse.audit.recorded.v1' AND payload->>'project_id'=? AND payload->'data'->'object'->>'type'='media_transcription' ORDER BY create_time,id`, project.String()).Scan(&rows).Error; err != nil || len(rows) == 0 {
		t.Fatal("produced speech audits missing", err)
	}
	consumer := auditevent.NewHandler(inboxpg.NewStore(db), auditapp.NewRecordedActionParser())
	for _, row := range rows {
		record := inboxapp.Record{Topic: row.Topic, Key: []byte(row.PartitionKey), Value: row.Payload}
		for range 2 {
			if err := consumer.Handle(t.Context(), record); err != nil {
				t.Fatalf("actual produced speech audit violates parser: %v", err)
			}
		}
	}
}

func TestTranscriptionSoftCancelActualHTTPWaitsForNativeResponse(t *testing.T) {
	f := speechFixture(t)
	job := f.create(t)
	started, finish := make(chan struct{}), make(chan struct{})
	var detached atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(started)
		select {
		case <-finish:
		case <-r.Context().Done():
			detached.Store(true)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"language":"english","duration":11,"segments":[{"start":0,"end":11,"text":"Actual HTTP terminal response"}]}`)
	}))
	defer server.Close()
	client, err := whisper.NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	worker := f.worker(t, client)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	id := toolapp.TranscriptionWorkID{JobID: job.ID, Attempt: 1, ExecutionID: uuid.New()}
	type speechOutcome struct {
		job tooldomain.TranscriptionJob
		err error
	}
	done := make(chan speechOutcome, 1)
	exited := make(chan struct{})
	go func() { actual, err := worker.Execute(ctx, id); done <- speechOutcome{actual, err} }()
	released := false
	defer func() {
		if !released {
			close(finish)
			<-exited
		}
		_ = f.store.Release(t.Context(), id)
	}()
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("native inference did not start")
	}
	running, err := f.store.Get(t.Context(), f.actor, f.project, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := f.store.Control(t.Context(), f.actor, f.project, job.ID, uuid.New(), running.Revision, "cancel")
	if err != nil || accepted.Status != tooldomain.TranscriptionCancelRequested {
		t.Fatal(err)
	}
	cancel() // The owned native call must remain attached until its actual terminal response.
	select {
	case outcome := <-done:
		t.Fatal("caller cancellation fabricated inference completion", outcome.err)
	case <-time.After(50 * time.Millisecond):
	}
	if detached.Load() {
		t.Fatal("soft cancellation detached native HTTP")
	}
	close(finish)
	released = true
	received := <-done
	if received.err != nil || received.job.Status != tooldomain.TranscriptionCancelled || received.job.ResultSHA256 != nil {
		t.Fatalf("terminal cancellation %+v %v", received.job, received.err)
	}
}

func TestTranscriptionUnknownConnectionCannotRePOST(t *testing.T) {
	f := speechFixture(t)
	job := f.create(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		h, ok := w.(http.Hijacker)
		if !ok {
			t.Error("test native connection cannot hijack")
			return
		}
		conn, _, err := h.Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		_ = conn.Close()
	}))
	defer server.Close()
	client, err := whisper.NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	worker := f.worker(t, client)
	id := toolapp.TranscriptionWorkID{JobID: job.ID, Attempt: 1, ExecutionID: uuid.New()}
	if _, err := worker.Execute(t.Context(), id); !errors.Is(err, toolapp.ErrInferenceUncertain) {
		t.Fatal("disconnected request completion forged", err)
	}
	if err := f.store.Finish(t.Context(), id, false, "inference_unknown"); err != nil {
		t.Fatal(err)
	}
	if err := f.store.Release(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	if err := f.store.FailWorkflow(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	next := id
	next.ExecutionID = uuid.New()
	if _, err := worker.Execute(t.Context(), next); !errors.Is(err, toolapp.ErrInferenceUncertain) {
		t.Fatal("uncertain inference re-entered", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("duplicate native inference calls=%d", calls.Load())
	}
	current, err := f.store.Get(t.Context(), f.actor, f.project, job.ID)
	if err != nil || current.Stage != "awaiting_reconciliation" {
		t.Fatal("lost native response was reported as normal running")
	}
	if _, err := f.store.Control(t.Context(), f.actor, f.project, job.ID, uuid.New(), current.Revision, "retry"); !errors.Is(err, toolapp.ErrConflict) {
		t.Fatal("unknown native retried")
	}
}

// This controlled scheduler verifies receipt/inbox dispatch, not a live Temporal run.
type transcriptionDispatchCounter struct{ calls int }

func (s *transcriptionDispatchCounter) Deliver(context.Context, toolapp.TranscriptionDelivery) error {
	s.calls++
	return nil
}
func TestTranscriptionProducedCommandReceiptAndInboxRejectForgery(t *testing.T) {
	db, store, actor, project, input := transcriptionPGFixture(t)
	job, err := store.Create(t.Context(), actor, project, uuid.New(), input)
	if err != nil {
		t.Fatal(err)
	}
	var row struct {
		Topic, PartitionKey string
		Payload             []byte
	}
	if err := db.Raw(`SELECT topic,partition_key,payload FROM infra.outbox WHERE topic=? AND payload->'data'->>'job_id'=?`, toolevent.TranscriptionTopic, job.ID.String()).Scan(&row).Error; err != nil || len(row.Payload) == 0 {
		t.Fatal("actual committed speech delivery missing", err)
	}
	starter := &transcriptionDispatchCounter{}
	handler := toolevent.NewTranscriptionHandler(inboxpg.NewStore(db), store, starter)
	record := inboxapp.Record{Topic: row.Topic, Key: []byte(row.PartitionKey), Value: row.Payload}
	for range 2 {
		if err := handler.Handle(t.Context(), record); err != nil {
			t.Fatal(err)
		}
	}
	if starter.calls != 1 {
		t.Fatal("duplicate scheduling not deduplicated")
	}
	var forged map[string]any
	if err := json.Unmarshal(row.Payload, &forged); err != nil {
		t.Fatal(err)
	}
	newID := uuid.NewString()
	forged["event_id"] = newID
	payload := forged["data"].(map[string]any)
	payload["event_id"] = newID
	payload["request_id"] = uuid.NewString()
	record.Value, err = json.Marshal(forged)
	if err != nil {
		t.Fatal(err)
	}
	if err := handler.Handle(t.Context(), record); !errors.Is(err, toolapp.ErrInvalidTranscription) {
		t.Fatal("uncommitted speech delivery admitted", err)
	}
	if starter.calls != 1 {
		t.Fatal("forgery reached scheduler")
	}
}

func TestTranscriptionSilentVideoFailsBeforeNativeDispatch(t *testing.T) {
	sample := filepath.Join(t.TempDir(), "silent.mp4")
	if output, err := exec.CommandContext(t.Context(), "ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "color=c=blue:s=64x64:r=25:d=0.5", "-c:v", "libx264", "-pix_fmt", "yuv420p", sample).CombinedOutput(); err != nil {
		t.Fatalf("actual silent video: %v %s", err, output)
	}
	f := speechFixtureFrom(t, sample, "video/mp4", "video")
	job := f.create(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()
	client, err := whisper.NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	activities := toolflow.NewTranscriptionActivities(f.worker(t, client), f.store)
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterActivityWithOptions(activities.TranscribeSpeech, activity.RegisterOptions{Name: toolflow.TranscribeActivity})
	env.RegisterActivityWithOptions(activities.FailTranscriptionWorkflow, activity.RegisterOptions{Name: toolflow.TranscriptionFailureActivity})
	env.ExecuteWorkflow(toolflow.TranscriptionWorkflow, toolapp.TranscriptionWorkID{JobID: job.ID, Attempt: 1})
	if env.GetWorkflowError() == nil || calls.Load() != 0 {
		t.Fatal("silent source sent fabricated speech inference")
	}
	failed, err := f.store.Get(t.Context(), f.actor, f.project, job.ID)
	if err != nil || failed.Status != tooldomain.TranscriptionFailed || failed.FailureCode == nil || *failed.FailureCode != "no_audio_stream" {
		t.Fatalf("actual failure facts %+v %v", failed, err)
	}
}
func TestTranscriptionInsufficientActivityBudgetNeverSubmitsNative(t *testing.T) {
	f := speechFixture(t)
	job := f.create(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()
	client, err := whisper.NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	id := toolapp.TranscriptionWorkID{JobID: job.ID, Attempt: 1, ExecutionID: uuid.New()}
	if _, err := f.worker(t, client).Execute(ctx, id); !errors.Is(err, toolapp.ErrUnavailable) {
		t.Fatal("activity budget was not enforced before dispatch", err)
	}
	if calls.Load() != 0 {
		t.Fatal("inference submitted without finalization budget")
	}
	if err := f.store.Finish(t.Context(), id, true, "dependency_unavailable"); err != nil {
		t.Fatal(err)
	}
	if err := f.store.Release(t.Context(), id); err != nil {
		t.Fatal(err)
	}
}

func TestTranscriptionUnconfiguredActivityFailsOldQueuedWithoutSubmission(t *testing.T) {
	db, store, actor, project, input := transcriptionPGFixture(t)
	job, err := store.Create(t.Context(), actor, project, uuid.New(), input)
	if err != nil {
		t.Fatal(err)
	}
	// Dependencies are absent before claim/dispatch. Production workflow cleanup
	// must terminate this historical queued request rather than leave it orphaned.
	activities := toolflow.NewTranscriptionActivities(toolapp.NewTranscriptionWorker(store, nil, nil, nil), store)
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterActivityWithOptions(activities.TranscribeSpeech, activity.RegisterOptions{Name: toolflow.TranscribeActivity})
	env.RegisterActivityWithOptions(activities.FailTranscriptionWorkflow, activity.RegisterOptions{Name: toolflow.TranscriptionFailureActivity})
	env.ExecuteWorkflow(toolflow.TranscriptionWorkflow, toolapp.TranscriptionWorkID{JobID: job.ID, Attempt: 1})
	if env.GetWorkflowError() == nil {
		t.Fatal("absent recognizer fabricated success")
	}
	failed, err := store.Get(t.Context(), actor, project, job.ID)
	if err != nil || failed.Status != tooldomain.TranscriptionFailed || failed.Stage != "failed" {
		t.Fatalf("historical unavailable work orphaned %+v %v; workflow error: %v", failed, err, env.GetWorkflowError())
	}
	var state struct {
		InferenceState string
		ActiveWorker   *uuid.UUID
	}
	if err := db.Raw(`SELECT inference_state,active_worker FROM mediatool.transcription_job WHERE id=?`, job.ID).Scan(&state).Error; err != nil || state.InferenceState != "none" || state.ActiveWorker != nil {
		t.Fatal("unconfigured Activity invented dispatch or retained owner", err)
	}
}
