package identity_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	handler "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/http"
	pg "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/postgres"
	app "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

func TestNotificationsHTTPAuthorityOriginCSRFAndContract(t *testing.T) {
	tx := existingIdentityTransaction(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	auth, admin := verifiedTestActor(t, tx, "notification-http-admin@example.invalid", now)
	if err := tx.Exec(`UPDATE identity."user" SET platform_role='platform_admin' WHERE id=?`, admin.ID).Error; err != nil {
		t.Fatal(err)
	}
	login, err := auth.Login(t.Context(), "notification-http-admin@example.invalid", "long password for tests", false)
	if err != nil {
		t.Fatal(err)
	}
	origin := "http://localhost:3000"
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(httpapi.Middleware(origin))
	api := router.Group("/api")
	h := handler.NewNotificationHandler(app.NewNotifications(pg.NewNotificationStore(tx), func() time.Time { return now }))
	h.RegisterPublic(api)
	security := handler.NewEmailAuthHandler(auth, origin, false)
	h.Register(api.Group("", security.RequireSession()))
	call := func(method, path, body string, cookie bool, requestOrigin, key, csrf string) *httptest.ResponseRecorder {
		r := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", requestOrigin)
		r.Header.Set("Idempotency-Key", key)
		r.Header.Set("X-CSRF-Token", csrf)
		if cookie {
			r.AddCookie(&http.Cookie{Name: "glowloom_session", Value: login.Token})
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	if w := call("GET", "/api/notifications/summary", "", false, "", "", ""); w.Code != 401 {
		t.Fatal("anonymous inbox", w.Code, w.Body.String())
	}
	if w := call("GET", "/api/announcements/public", "", false, "", "", ""); w.Code != 200 {
		t.Fatal("public announcements", w.Code, w.Body.String())
	}
	body, _ := json.Marshal(app.SaveAnnouncementInput{CommandID: uuid.New(), Content: notificationContent(now)})
	for _, tc := range []struct {
		origin, key, csrf string
		status            int
	}{{"", uuid.NewString(), login.CSRFToken, 403}, {origin, "", login.CSRFToken, 422}, {origin, uuid.NewString(), "", 403}} {
		if w := call("POST", "/api/platform/notifications", string(body), true, tc.origin, tc.key, tc.csrf); w.Code != tc.status {
			t.Fatal("write security", w.Code, w.Body.String())
		}
	}
	w := call("POST", "/api/platform/notifications", string(body), true, origin, uuid.NewString(), login.CSRFToken)
	if w.Code != 201 {
		t.Fatal("create", w.Code, w.Body.String())
	}
	var created struct {
		ID      uuid.UUID `json:"id"`
		Version string    `json:"version"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &created); err != nil || created.Version != "1" {
		t.Fatal("string revision", created, err)
	}
	validityPath := "/api/notifications/display/" + created.ID.String() + "/1"
	if w = call("GET", validityPath, "", false, "", "", ""); w.Code != 401 {
		t.Fatal("anonymous display validation", w.Code, w.Body.String())
	}
	w = call("GET", validityPath, "", true, "", "", "")
	var validity app.NotificationDisplayValidity
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &validity) != nil || validity.Valid {
		t.Fatal("draft display validation", w.Code, w.Body.String())
	}
	if w = call("GET", "/api/notifications/display/"+created.ID.String()+"/01", "", true, "", "", ""); w.Code != 422 {
		t.Fatal("invalid display revision", w.Code, w.Body.String())
	}
	if w = call("GET", "/api/platform/notifications?unknown=true", "", true, "", "", ""); w.Code != 422 {
		t.Fatal("unknown query accepted", w.Code, w.Body.String())
	}
	unknown := strings.TrimSuffix(string(body), "}") + `,"recipient_id":"` + uuid.NewString() + `"}`
	if w = call("POST", "/api/platform/notifications", unknown, true, origin, uuid.NewString(), login.CSRFToken); w.Code != 422 {
		t.Fatal("unknown write field accepted", w.Code, w.Body.String())
	}
	if err = tx.Exec(`UPDATE identity."user" SET platform_role='creator',platform_epoch=platform_epoch+1 WHERE id=?`, admin.ID).Error; err != nil {
		t.Fatal(err)
	}
	if w = call("GET", "/api/platform/notifications", "", true, "", "", ""); w.Code != 403 {
		t.Fatal("stale administrator accepted", w.Code, w.Body.String())
	}
}
