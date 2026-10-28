package media_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	"github.com/StephenQiu30/glowloom-server/backend/internal/app/identityruntime"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	subject "github.com/StephenQiu30/glowloom-server/backend/internal/subject/application"
	"github.com/StephenQiu30/glowloom-server/backend/tests/identityfixture"
)

func TestSubjectPublicAPIThroughNativeTOSUploadSaveAndExportWithoutCloudCredentials(t *testing.T) {
	owner, _ := routedGeneratedDatabase(t)
	tx := owner.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	t.Cleanup(func() { _ = tx.Rollback().Error })
	_, _, storageCfg := controlledGeneratedTOS(t)
	cfg := identityfixture.Configure(t, config.Config{Env: "local", HTTPAddr: "127.0.0.1:18089", PublicOrigin: "http://localhost:1122", ObjectStorage: &storageCfg})
	account := identityfixture.Create(t, tx, cfg, "Subject Native TOS Fixture", "SyntheticSubject123")
	scope := identityfixture.Scope(t, tx, account.User.ID)
	routes := pgmedia.NewStorageRoutes(tx)
	if err := routes.RegisterProfiles(t.Context(), []media.StorageProfile{{ID: "tos-frozen", Provider: "tos", Endpoint: "https://tos-fixture.example.invalid", SigningEndpoint: "https://tos-fixture.example.invalid", Region: "cn-beijing", Bucket: "controlled-fixture"}}); err != nil {
		t.Fatal(err)
	}
	// Controlled fixture admission only: this is not a real cloud activation proof.
	policy := media.UploadPolicy{MaxImageBytes: 20 << 20, MaxVideoBytes: 500 << 20, MaxAudioBytes: 100 << 20, MaxModelBytes: 64 << 20, MaxDocumentBytes: 20 << 20}
	raw, _ := json.Marshal(policy)
	if err := tx.Exec(`INSERT INTO media.storage_config_revision(revision,config_sha256,profile_id,upload_policy,session_ttl_seconds) VALUES('controlled-api-v1',repeat('b',64),'tos-frozen',?::jsonb,86400) ON CONFLICT DO NOTHING`, string(raw)).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`INSERT INTO media.storage_route(singleton,revision,profile_id) VALUES(1,'controlled-api-v1','tos-frozen') ON CONFLICT(singleton) DO UPDATE SET revision=EXCLUDED.revision,profile_id=EXCLUDED.profile_id`).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`SET LOCAL ROLE glowloom_app`).Error; err != nil {
		t.Fatal(err)
	}
	router, err := app.NewBusinessRouter(zap.NewNop(), nil, noop.NewTracerProvider(), cfg, tx, nil, identityruntime.SessionDependencies{}, t.Context())
	if err != nil {
		t.Fatal(err)
	}
	call := func(method, path string, body []byte, content, key string) *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(t.Context(), method, path, bytes.NewReader(body))
		req.Header.Set("Origin", cfg.PublicOrigin)
		req.Header.Set("Content-Type", content)
		req.Header.Set("X-CSRF-Token", account.CSRFToken)
		req.Header.Set("X-Creation-Scope", scope.String())
		if key != "" {
			req.Header.Set("Idempotency-Key", key)
		}
		req.AddCookie(&http.Cookie{Name: "glowloom_session", Value: account.Token})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	source := newRoutedIngestFixture()
	file, err := source.Download(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	imageBytes, err := io.ReadAll(file.File)
	if err != nil {
		t.Fatal(err)
	}
	var multipartBody bytes.Buffer
	form := multipart.NewWriter(&multipartBody)
	part, err := form.CreateFormFile("file", "synthetic-subject.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(imageBytes); err != nil {
		t.Fatal(err)
	}
	if err := form.WriteField("local_review_confirmed", "true"); err != nil {
		t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	uploadKey := uuid.NewString()
	response := call("POST", "/api/subjects/sources/uploads", multipartBody.Bytes(), form.FormDataContentType(), uploadKey)
	if response.Code != 201 {
		t.Fatalf("native routed subject upload: %d %s", response.Code, response.Body.String())
	}
	var uploaded media.PersonalUploadResult
	if json.Unmarshal(response.Body.Bytes(), &uploaded) != nil || uploaded.Asset.ID == uuid.Nil {
		t.Fatal("invalid upload receipt")
	}
	replay := call("POST", "/api/subjects/sources/uploads", multipartBody.Bytes(), form.FormDataContentType(), uploadKey)
	if replay.Code != 201 || !bytes.Equal(replay.Body.Bytes(), response.Body.Bytes()) {
		t.Fatalf("upload permanent key changed receipt: %d", replay.Code)
	}
	body, _ := json.Marshal(subject.SaveInput{Name: "统一环境主体", Description: "真实HTTP、PostgreSQL和TOS原生SDK；受控TLS wire", Primary: subject.MediaSelection{AssetID: uploaded.Asset.ID}})
	saved := call("POST", "/api/subjects", body, "application/json", uuid.NewString())
	var detail subject.Detail
	if saved.Code != 200 || json.Unmarshal(saved.Body.Bytes(), &detail) != nil || detail.Primary.AssetID != uploaded.Asset.ID || detail.Primary.BindingID == uuid.Nil {
		t.Fatalf("atomic subject save %d %s", saved.Code, saved.Body.String())
	}
	reopened := call("GET", "/api/subjects/"+detail.ID.String(), nil, "application/json", "")
	var readback subject.Detail
	if reopened.Code != 200 || json.Unmarshal(reopened.Body.Bytes(), &readback) != nil || readback.VersionID != detail.VersionID || readback.Primary.BindingID != detail.Primary.BindingID {
		t.Fatal("reopen lost fixed version")
	}
	exportedBody, _ := json.Marshal(subject.ExportInput{Items: []subject.ExportItem{{ID: detail.ID, VersionID: detail.VersionID}}})
	exported := call("POST", "/api/subjects/export", exportedBody, "application/json", uuid.NewString())
	if exported.Code != 200 {
		t.Fatalf("native TOS export %d %s", exported.Code, exported.Body.String())
	}
	archive, err := zip.NewReader(bytes.NewReader(exported.Body.Bytes()), int64(exported.Body.Len()))
	if err != nil || len(archive.File) != 2 {
		t.Fatal("incomplete portable archive", err)
	}
	found := false
	for _, entry := range archive.File {
		if entry.Name == "manifest.json" {
			continue
		}
		stream, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(stream)
		closeErr := stream.Close()
		if err != nil || closeErr != nil || !bytes.Equal(b, imageBytes) {
			t.Fatal("export changed original bytes")
		}
		found = true
	}
	if !found {
		t.Fatal("no original in archive")
	}
	from := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	listed := call("GET", "/api/subjects?q="+url.QueryEscape(detail.Name)+"&from="+url.QueryEscape(from)+"&order=created_desc&page_size="+strconv.Itoa(12), nil, "application/json", "")
	if listed.Code != 200 {
		t.Fatalf("filtered global library %d %s", listed.Code, listed.Body.String())
	}
}
