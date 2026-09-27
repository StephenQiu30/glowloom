package app_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"

	"github.com/StephenQiu30/lanverse/backend/internal/app"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	redisrealtime "github.com/StephenQiu30/lanverse/backend/internal/infra/realtime/adapter/redis"
	"github.com/StephenQiu30/lanverse/backend/internal/infra/realtime/adapter/sse"
	realtime "github.com/StephenQiu30/lanverse/backend/internal/infra/realtime/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/config"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/redisconn"
	pgworkspace "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
)

func TestProjectCreateAndUpdateReachSSEThroughRelay(t *testing.T) {
	dsn := os.Getenv("LV_TEST_RELAY_DB_DSN")
	brokers := os.Getenv("LV_TEST_RELAY_KAFKA_BROKERS")
	redisURL := os.Getenv("LV_TEST_RELAY_REDIS_URL")
	if dsn == "" || brokers == "" || redisURL == "" {
		t.Skip("set disposable LV_TEST_RELAY_DB_DSN, LV_TEST_RELAY_KAFKA_BROKERS, and LV_TEST_RELAY_REDIS_URL")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Second)
	defer cancel()
	dbConn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = dbConn.Close() })
	redisConn, err := redisconn.Open(redisURL)
	if err != nil {
		t.Fatalf("open test Redis: %v", err)
	}
	t.Cleanup(func() { _ = redisConn.Close() })

	orgID, actorID := uuid.New(), uuid.New()
	if err := dbConn.DB.WithContext(ctx).Exec(
		"INSERT INTO workspace.organization(id, name) VALUES (?::uuid, ?)",
		orgID.String(), "Relay project test",
	).Error; err != nil {
		t.Fatalf("insert test organization: %v", err)
	}
	t.Cleanup(func() {
		if err := dbConn.DB.Exec("DELETE FROM workspace.organization WHERE id = ?::uuid", orgID.String()).Error; err != nil {
			t.Errorf("delete test organization: %v", err)
		}
	})
	if err := dbConn.DB.WithContext(ctx).Exec(`
		INSERT INTO identity."user"
		  (id, org_id, login_name, display_name, role, password_hash, must_change_password)
		VALUES (?::uuid, ?::uuid, ?, ?, 'producer', ?, false)
	`, actorID.String(), orgID.String(), "relay-"+actorID.String(), "Relay project actor", "unused-test-hash").Error; err != nil {
		t.Fatalf("insert test actor: %v", err)
	}
	t.Cleanup(func() {
		if err := dbConn.DB.Exec("DELETE FROM identity.\"user\" WHERE id = ?::uuid", actorID.String()).Error; err != nil {
			t.Errorf("delete test actor: %v", err)
		}
	})

	actor := identityapp.Principal{ID: actorID, OrgID: orgID, Role: identitydomain.RoleProducer}
	created, err := workspaceapp.NewCreateProjectCommand(pgworkspace.NewStore(dbConn.DB), time.Now).Execute(
		ctx, actor, workspaceapp.CreateProjectInput{
			Name: "Relay 实时项目", AspectRatio: "16:9", StyleType: "realistic", RequestID: uuid.NewString(),
		},
	)
	if err != nil {
		t.Fatalf("create project command: %v", err)
	}
	projectID := created.ID.String()
	stream := "project:" + projectID + ":events"
	var outbox []struct {
		ID    uuid.UUID
		Topic string
	}
	if err := dbConn.DB.WithContext(ctx).Raw(
		"SELECT id, topic FROM infra.outbox WHERE partition_key = ?", projectID,
	).Scan(&outbox).Error; err != nil || len(outbox) != 2 {
		t.Fatalf("project Outbox events = %+v, error = %v", outbox, err)
	}
	var changeID, auditID uuid.UUID
	for _, event := range outbox {
		switch event.Topic {
		case realtime.ProjectChangedTopic:
			changeID = event.ID
		case "lanverse.audit.recorded.v1":
			auditID = event.ID
		default:
			t.Fatalf("unexpected project Outbox topic %q", event.Topic)
		}
	}
	if changeID == uuid.Nil || auditID == uuid.Nil {
		t.Fatalf("project Outbox missing change or audit event: %+v", outbox)
	}
	t.Cleanup(func() {
		if err := dbConn.DB.Exec(
			"DELETE FROM infra.processed_event WHERE event_id IN (SELECT id FROM infra.outbox WHERE partition_key = ?)",
			projectID,
		).Error; err != nil {
			t.Errorf("delete project consumer markers: %v", err)
		}
		if err := dbConn.DB.Exec("DELETE FROM infra.outbox WHERE partition_key = ?", projectID).Error; err != nil {
			t.Errorf("delete project Outbox events: %v", err)
		}
		if err := dbConn.DB.Exec("DELETE FROM billing.budget WHERE project_id = ?::uuid", projectID).Error; err != nil {
			t.Errorf("delete test budget: %v", err)
		}
		if err := dbConn.DB.Exec("DELETE FROM workspace.project WHERE id = ?::uuid", projectID).Error; err != nil {
			t.Errorf("delete test project: %v", err)
		}
		if err := redisConn.Client.Del(context.Background(), stream).Err(); err != nil {
			t.Errorf("delete project replay stream: %v", err)
		}
	})

	streamHandler, err := sse.NewHandler(redisConn.Client, func(_ *http.Request, id string) bool {
		return id == projectID
	}, zap.NewNop(), sse.Options{})
	if err != nil {
		t.Fatalf("create test-authorized SSE handler: %v", err)
	}
	streamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		streamHandler.ServeProject(w, r, projectID)
	}))
	t.Cleanup(streamServer.Close)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, streamServer.URL+"/events", nil)
	if err != nil {
		t.Fatalf("create SSE request: %v", err)
	}
	response, err := streamServer.Client().Do(request)
	if err != nil {
		t.Fatalf("connect project SSE: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK || !strings.HasPrefix(response.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("project SSE response = %d, %q", response.StatusCode, response.Header.Get("Content-Type"))
	}
	reader := bufio.NewReader(response.Body)
	if line, err := reader.ReadString('\n'); err != nil || line != ": connected\n" {
		t.Fatalf("project SSE connection frame = %q, error = %v", line, err)
	}
	if line, err := reader.ReadString('\n'); err != nil || line != "\n" {
		t.Fatalf("project SSE connection frame ending = %q, error = %v", line, err)
	}

	runCtx, stopRelay := context.WithCancel(context.Background())
	relayDone := make(chan error, 1)
	healthAddr := localHealthAddress(t)
	go func() {
		relayDone <- app.RunRelay(runCtx, config.Config{
			DBDSN: dsn, KafkaBrokers: brokers, RedisURL: redisURL,
			RelayHealthAddr: healthAddr,
		}, zap.NewNop())
	}()
	t.Cleanup(func() {
		stopRelay()
		select {
		case err := <-relayDone:
			if err != nil {
				t.Errorf("stop project relay: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Error("project relay did not stop after cancellation")
		}
		assertRoleHealthStopped(t, healthAddr)
	})
	assertRoleHealth(ctx, t, healthAddr)

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		var published, realtimeProcessed, auditProcessed, audited bool
		if err := dbConn.DB.WithContext(ctx).Raw(`
			SELECT count(*) = 2 FROM infra.outbox
			WHERE id IN (?::uuid, ?::uuid) AND published_at IS NOT NULL
		`, changeID.String(), auditID.String()).Scan(&published).Error; err != nil {
			t.Fatalf("read Outbox delivery: %v", err)
		}
		if err := dbConn.DB.WithContext(ctx).Raw(`
			SELECT EXISTS (SELECT 1 FROM infra.processed_event
			WHERE consumer = 'realtime' AND event_id = ?::uuid)
		`, changeID.String()).Scan(&realtimeProcessed).Error; err != nil {
			t.Fatalf("read realtime consumer marker: %v", err)
		}
		if err := dbConn.DB.WithContext(ctx).Raw(`
			SELECT EXISTS (SELECT 1 FROM infra.processed_event
			WHERE consumer = 'audit' AND event_id = ?::uuid)
		`, auditID.String()).Scan(&auditProcessed).Error; err != nil {
			t.Fatalf("read audit consumer marker: %v", err)
		}
		if err := dbConn.DB.WithContext(ctx).Raw(`
			SELECT EXISTS (SELECT 1 FROM audit.audit_log
			WHERE id = ?::uuid AND project_id = ?::uuid AND action = 'project.created')
		`, auditID.String(), projectID).Scan(&audited).Error; err != nil {
			t.Fatalf("read project audit row: %v", err)
		}
		if published && realtimeProcessed && auditProcessed && audited {
			break
		}
		select {
		case err := <-relayDone:
			t.Fatalf("relay exited before project projection: %v", err)
		case <-ctx.Done():
			t.Fatalf("wait for project relay: %v (published %t, realtime %t, audit %t, audited %t)",
				ctx.Err(), published, realtimeProcessed, auditProcessed, audited)
		case <-ticker.C:
		}
	}
	entries, err := redisConn.Client.XRange(ctx, stream, "-", "+").Result()
	if err != nil || len(entries) != 1 || entries[0].Values["id"] != changeID.String() ||
		entries[0].Values["event"] != "project.updated" {
		t.Fatalf("project Redis projection = %+v, error = %v", entries, err)
	}
	streamData, ok := entries[0].Values["data"].(string)
	if !ok {
		t.Fatalf("project Redis data = %T", entries[0].Values["data"])
	}
	assertRelayProjectData(t, []byte(streamData), projectID, 1, "created")
	frame := nextNamedSSEFrame(t, reader)
	if !strings.Contains(frame, "id: "+changeID.String()+"\n") ||
		!strings.Contains(frame, "event: project.updated\n") {
		t.Fatalf("project SSE event = %q", frame)
	}
	var sseData string
	for _, line := range strings.Split(frame, "\n") {
		if strings.HasPrefix(line, "data: ") {
			sseData = strings.TrimPrefix(line, "data: ")
		}
	}
	assertRelayProjectData(t, []byte(sseData), projectID, 1, "created")

	newName := "修改后的私密项目名称"
	newDescription := "修改说明不得进入事件"
	updated, err := workspaceapp.NewUpdateProjectCommand(pgworkspace.NewStore(dbConn.DB), time.Now).Execute(
		ctx, actor, workspaceapp.UpdateProjectInput{
			ProjectID: created.ID, ExpectedRevision: created.Revision,
			Name: &newName, Description: &newDescription, RequestID: uuid.NewString(),
		},
	)
	if err != nil || updated.ID != created.ID || updated.Revision != 2 ||
		updated.Name != newName || updated.Description != newDescription {
		t.Fatalf("update project command = %+v, error = %v", updated, err)
	}
	var updateEvents []struct {
		ID      uuid.UUID
		Topic   string
		Payload []byte
	}
	if err := dbConn.DB.WithContext(ctx).Raw(`
		SELECT id, topic, payload FROM infra.outbox
		WHERE partition_key = ? AND id NOT IN (?::uuid, ?::uuid)
	`, projectID, changeID.String(), auditID.String()).Scan(&updateEvents).Error; err != nil || len(updateEvents) != 2 {
		t.Fatalf("project update Outbox events = %+v, error = %v", updateEvents, err)
	}
	var updateChangeID, updateAuditID uuid.UUID
	for _, event := range updateEvents {
		if bytes.Contains(event.Payload, []byte(newName)) || bytes.Contains(event.Payload, []byte(newDescription)) {
			t.Fatal("project update Outbox leaked a free-form setting")
		}
		switch event.Topic {
		case realtime.ProjectChangedTopic:
			updateChangeID = event.ID
		case "lanverse.audit.recorded.v1":
			updateAuditID = event.ID
		default:
			t.Fatalf("unexpected project update Outbox topic %q", event.Topic)
		}
	}
	if updateChangeID == uuid.Nil || updateAuditID == uuid.Nil {
		t.Fatalf("project update Outbox missing change or audit event: %+v", updateEvents)
	}
	for {
		var published, realtimeProcessed, auditProcessed, audited bool
		if err := dbConn.DB.WithContext(ctx).Raw(`
			SELECT count(*) = 2 FROM infra.outbox
			WHERE id IN (?::uuid, ?::uuid) AND published_at IS NOT NULL
		`, updateChangeID.String(), updateAuditID.String()).Scan(&published).Error; err != nil {
			t.Fatalf("read update Outbox delivery: %v", err)
		}
		if err := dbConn.DB.WithContext(ctx).Raw(`
			SELECT EXISTS (SELECT 1 FROM infra.processed_event
			WHERE consumer = 'realtime' AND event_id = ?::uuid)
		`, updateChangeID.String()).Scan(&realtimeProcessed).Error; err != nil {
			t.Fatalf("read update realtime marker: %v", err)
		}
		if err := dbConn.DB.WithContext(ctx).Raw(`
			SELECT EXISTS (SELECT 1 FROM infra.processed_event
			WHERE consumer = 'audit' AND event_id = ?::uuid)
		`, updateAuditID.String()).Scan(&auditProcessed).Error; err != nil {
			t.Fatalf("read update audit marker: %v", err)
		}
		if err := dbConn.DB.WithContext(ctx).Raw(`
			SELECT EXISTS (SELECT 1 FROM audit.audit_log
			WHERE id = ?::uuid AND project_id = ?::uuid AND action = 'project.updated')
		`, updateAuditID.String(), projectID).Scan(&audited).Error; err != nil {
			t.Fatalf("read project update audit row: %v", err)
		}
		if published && realtimeProcessed && auditProcessed && audited {
			break
		}
		select {
		case err := <-relayDone:
			t.Fatalf("relay exited before project update projection: %v", err)
		case <-ctx.Done():
			t.Fatalf("wait for project update relay: %v (published %t, realtime %t, audit %t, audited %t)",
				ctx.Err(), published, realtimeProcessed, auditProcessed, audited)
		case <-ticker.C:
		}
	}
	entries, err = redisConn.Client.XRange(ctx, stream, "-", "+").Result()
	if err != nil || len(entries) != 2 || entries[1].Values["id"] != updateChangeID.String() ||
		entries[1].Values["event"] != "project.updated" {
		t.Fatalf("updated project Redis projection = %+v, error = %v", entries, err)
	}
	streamData, ok = entries[1].Values["data"].(string)
	if !ok {
		t.Fatalf("updated project Redis data = %T", entries[1].Values["data"])
	}
	assertRelayProjectData(t, []byte(streamData), projectID, 2, "updated")
	replayed, resync, err := redisrealtime.NewSink(redisConn.Client).Replay(ctx, projectID, changeID.String())
	if err != nil || resync || len(replayed) != 1 || replayed[0].ID != updateChangeID.String() ||
		replayed[0].Event != "project.updated" {
		t.Fatalf("project update replay = %+v, resync = %t, error = %v", replayed, resync, err)
	}
	assertRelayProjectData(t, replayed[0].Data, projectID, 2, "updated")
	frame = nextNamedSSEFrame(t, reader)
	if !strings.Contains(frame, "id: "+updateChangeID.String()+"\n") ||
		!strings.Contains(frame, "event: project.updated\n") {
		t.Fatalf("project update SSE event = %q", frame)
	}
	sseData = ""
	for _, line := range strings.Split(frame, "\n") {
		if strings.HasPrefix(line, "data: ") {
			sseData = strings.TrimPrefix(line, "data: ")
		}
	}
	assertRelayProjectData(t, []byte(sseData), projectID, 2, "updated")
}

func assertRelayProjectData(t *testing.T, raw []byte, projectID string, revision int64, change string) {
	t.Helper()
	var data map[string]json.RawMessage
	if err := json.Unmarshal(raw, &data); err != nil || len(data) != 3 {
		t.Fatalf("project event data = %q, error = %v; want three safe fields", raw, err)
	}
	var got struct {
		ProjectID string `json:"project_id"`
		Revision  int64  `json:"revision"`
		Change    string `json:"change"`
	}
	if err := json.Unmarshal(raw, &got); err != nil || got.ProjectID != projectID ||
		got.Revision != revision || got.Change != change {
		t.Fatalf("project event data = %+v, error = %v", got, err)
	}
}
