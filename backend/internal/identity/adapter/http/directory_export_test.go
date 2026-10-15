package http

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

func TestDirectoryExportHTTPStrictFiltersAndMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	service := application.NewDirectoryExport(nil, nil, nil, time.Now)
	stats := application.NewDirectoryStatistics(nil, time.Now)
	NewDirectoryExportHandler(service, stats).Register(router.Group("/api"))
	NewInvitationHandler(nil).Register(router.Group("/api"))
	team := uuid.NewString()
	for _, path := range []string{
		"/api/platform/user-statistics?status=pending_deletion",
		"/api/platform/user-statistics?status=active&status=deleted",
		"/api/platform/user-statistics?cursor=opaque",
		"/api/platform/user-statistics?from=invalid",
		"/api/platform/user-statistics?in_team=1",
		"/api/platform/user-statistics?role=team_admin",
		"/api/teams/" + team + "/member-statistics?platform_role=platform_admin",
		"/api/teams/" + team + "/member-statistics?role=team_admin&role=member",
		"/api/teams/" + team + "/member-statistics?status=deleted",
		"/api/platform/teams/" + team + "/invitations?limit=1&limit=2",
	} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), "GET", path, nil))
		if w.Code != 422 || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s: %d", path, w.Code)
		}
	}
	for _, body := range []string{`{"filter":{"status":"pending_deletion"}}`, `{"filter":{"password_hash":"hidden"}}`, `{"filter":{"role":"team_admin"}}`} {
		r := httptest.NewRequestWithContext(t.Context(), "POST", "/api/platform/user-exports", strings.NewReader(body))
		r.Header.Set("Idempotency-Key", uuid.NewString())
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != 422 {
			t.Fatalf("%s: %d", body, w.Code)
		}
	}
	now := time.Now()
	object := uuid.New()
	view := exportView(application.DirectoryExportJob{ID: uuid.New(), Status: "completed", ObjectID: &object, KeyID: "private", Snapshot: []byte("secret"), ExpiresAt: now})
	raw, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{object.String(), "private", "secret", "snapshot", "object_id", "actor_id"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("private export field %q escaped DTO", secret)
		}
	}
	if exportView(application.DirectoryExportJob{Status: "pending", ExpiresAt: now}).ExpiresAt != nil {
		t.Fatal("pending job exposed a completed-download expiry")
	}
}
