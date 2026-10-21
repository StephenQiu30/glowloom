package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app/identityruntime"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	bibleapp "github.com/StephenQiu30/glowloom-server/backend/internal/bible/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	platformdb "github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
	"github.com/StephenQiu30/glowloom-server/backend/tests/identityfixture"
)

func TestSubjectRecoveryPublicRouter(t *testing.T) {
	dsn := os.Getenv("LV_TEST_SUBJECT_RECOVERY_DB_DSN")
	if dsn == "" {
		t.Skip("requires isolated subject database")
	}
	u, err := url.Parse(dsn)
	if err != nil || !strings.HasPrefix(u.Path, "/glowloom_subject_validation_") {
		t.Fatal("requires task-owned subject database")
	}
	owner, err := platformdb.Open(t.Context(), dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.DB.Exec(`INSERT INTO workspace.organization(id,name) SELECT ?,'Subject acceptance' WHERE NOT EXISTS(SELECT 1 FROM workspace.organization WHERE NOT is_delete)`, uuid.New()).Error; err != nil {
		t.Fatal(err)
	}
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("options", "-c role=glowloom_app")
	u.RawQuery = q.Encode()
	db, err := platformdb.Open(t.Context(), u.String(), noop.NewTracerProvider())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	cfg := identityfixture.Configure(t, config.Config{Env: "local", HTTPAddr: "127.0.0.1:18089", PublicOrigin: "http://127.0.0.1:3000"})
	router, err := app.NewBusinessRouter(zap.NewNop(), nil, noop.NewTracerProvider(), cfg, db.DB, nil, identityruntime.SessionDependencies{})
	if err != nil {
		t.Fatal(err)
	}
	account := identityfixture.Create(t, db.DB, cfg, "Subject Acceptance", "SyntheticSubject123")
	scope := identityfixture.Scope(t, db.DB, account.User.ID)
	call := func(method, path string, body any, key string) *httptest.ResponseRecorder {
		t.Helper()
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequestWithContext(t.Context(), method, path, bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://127.0.0.1:3000")
		req.Header.Set("X-CSRF-Token", account.CSRFToken)
		req.Header.Set("X-Creation-Scope", scope.String())
		req.Header.Set("Idempotency-Key", key)
		req.AddCookie(&http.Cookie{Name: "glowloom_session", Value: account.Token})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	projectResponse := call("POST", "/api/projects", map[string]string{"name": "Subject Acceptance", "aspect_ratio": "16:9", "style_type": "realistic"}, uuid.NewString())
	var project struct {
		ID uuid.UUID `json:"id"`
	}
	if projectResponse.Code != 201 || json.Unmarshal(projectResponse.Body.Bytes(), &project) != nil || project.ID == uuid.Nil {
		t.Fatalf("project: %d %s", projectResponse.Code, projectResponse.Body.String())
	}
	path := "/api/projects/" + project.ID.String() + "/bible/character"
	created := call("POST", path, map[string]any{"expected_revision": 0, "character": map[string]any{"name": "Subject Acceptance", "aliases": []string{}, "definition": map[string]any{}}}, uuid.NewString())
	var receipt bibleapp.Receipt
	if created.Code != 200 || json.Unmarshal(created.Body.Bytes(), &receipt) != nil {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	target := path + "/" + receipt.EntryID.String()
	preview := call("DELETE", target, map[string]any{"expected_revision": receipt.Revision}, uuid.NewString())
	var impact struct {
		Meta struct {
			Impact bibleapp.ImpactProof `json:"impact"`
		} `json:"meta"`
	}
	if preview.Code != 409 || json.Unmarshal(preview.Body.Bytes(), &impact) != nil || len(impact.Meta.Impact.SHA256) != 64 {
		t.Fatalf("actual impact: %d %s", preview.Code, preview.Body.String())
	}
	body := map[string]any{"expected_revision": receipt.Revision, "acknowledged_impact": impact.Meta.Impact}
	key := uuid.NewString()
	deleted := call("DELETE", target, body, key)
	if deleted.Code != 200 {
		t.Fatalf("delete: %d %s", deleted.Code, deleted.Body.String())
	}
	replay := call("DELETE", target, body, key)
	if replay.Code != 200 || !bytes.Equal(deleted.Body.Bytes(), replay.Body.Bytes()) {
		t.Fatal("delete replay changed")
	}
	if json.Unmarshal(deleted.Body.Bytes(), &receipt) != nil {
		t.Fatal("invalid receipt")
	}
	restored := call("POST", target+"/restore", map[string]any{"expected_revision": receipt.Revision}, uuid.NewString())
	if restored.Code != 200 {
		t.Fatalf("restore: %d %s", restored.Code, restored.Body.String())
	}
	active := call("GET", path, nil, "")
	trash := call("GET", path+"?deleted=true", nil, "")
	if active.Code != 200 || !strings.Contains(active.Body.String(), receipt.EntryID.String()) || trash.Code != 200 || strings.Contains(trash.Body.String(), receipt.EntryID.String()) {
		t.Fatal("persisted lists do not match restoration")
	}
	if os.Getenv("LV_SUBJECT_BROWSER_SERVER") != "1" {
		return
	}
	evidence := os.Getenv("LV_SUBJECT_BROWSER_EVIDENCE")
	if !strings.HasPrefix(evidence, "/Users/stephenqiu/.codex/visualizations/") {
		t.Fatal("explicit task evidence path required")
	}
	info, _ := json.Marshal(map[string]string{"email": account.User.Email, "project": project.ID.String(), "subject": receipt.EntryID.String()})
	if err := os.WriteFile(evidence+"/browser-account.json", info, 0600); err != nil {
		t.Fatal(err)
	}
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:18089")
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: router, ReadHeaderTimeout: 5 * time.Second}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	t.Log("isolated real HTTP/DB browser server ready")
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	deadline := time.NewTimer(25 * time.Minute)
	defer deadline.Stop()
	for {
		select {
		case <-ticker.C:
			if _, err := os.Stat(evidence + "/stop-server"); err != nil {
				continue
			}
		case <-deadline.C:
		case err := <-done:
			t.Fatal(err)
		}
		break
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		t.Error(err)
	}
	if err := <-done; err != nil && !errors.Is(err, http.ErrServerClosed) {
		t.Error(err)
	}
}
