package analytics_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	analyticspg "github.com/StephenQiu30/glowloom-server/backend/internal/analytics/adapter/postgres"
	analytical "github.com/StephenQiu30/glowloom-server/backend/internal/analytics/application"
	analyticsdomain "github.com/StephenQiu30/glowloom-server/backend/internal/analytics/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/app/insights"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/observability/adapter/memory"
	obsapp "github.com/StephenQiu30/glowloom-server/backend/internal/observability/application"
	obsdomain "github.com/StephenQiu30/glowloom-server/backend/internal/observability/domain"
)

func analyticsTransaction(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("GLOWLOOM_TEST_ANALYTICS_DSN")
	if dsn == "" {
		t.Skip("isolated analysis PostgreSQL absent; not verified")
	}
	database, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("connect isolated PostgreSQL failed")
	}
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	var name string
	if err := database.Raw("SELECT current_database()").Scan(&name).Error; err != nil || !strings.HasPrefix(name, "glowloom_logs_") {
		t.Fatal("test requires dedicated glowloom_logs_ database")
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 30*time.Second)
	t.Cleanup(cancel)
	tx := database.WithContext(ctx).Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	t.Cleanup(func() {
		if err := tx.Rollback().Error; err != nil {
			t.Error("fixture rollback failed")
		}
	})
	if err := tx.Exec("SET LOCAL lock_timeout='1s'").Error; err != nil {
		t.Fatal(err)
	}
	return tx
}
func seedActor(t *testing.T, tx *gorm.DB, now time.Time, org uuid.UUID) identity.Principal {
	t.Helper()
	a := identity.Principal{ID: uuid.New(), SessionID: uuid.New(), OrgID: org, ScopeID: uuid.New(), CredentialEpoch: 1, PlatformEpoch: 1, PlatformRole: "creator"}
	run(t, tx, `INSERT INTO workspace.organization(id,name)VALUES(?,'Synthetic analysis fixture')ON CONFLICT DO NOTHING`, org)
	run(t, tx, `INSERT INTO identity."user"(id,org_id,login_name,display_name,role,password_hash,email_key,email_verified_at)VALUES(?,?,?,'Synthetic analysis fixture','producer','synthetic',?,?)`, a.ID, org, "analytics-"+a.ID.String(), a.ID.String()+"@fixture.invalid", now.Add(-time.Hour))
	digest := sha256.Sum256(a.SessionID[:])
	run(t, tx, `INSERT INTO identity.user_session(id,account_id,token_hash,credential_revision,created_at,last_active_at,absolute_expires_at,persistent)VALUES(?,?,?,1,?,?,?,false)`, a.SessionID, a.ID, digest[:], now.Add(-time.Hour), now.Add(-time.Minute), now.Add(time.Hour))
	run(t, tx, `INSERT INTO workspace.creation_scope(id,kind,owner_user_id,physical_scope_id)VALUES(?,'personal',?,?)`, a.ScopeID, a.ID, org)
	return a
}
func run(t *testing.T, tx *gorm.DB, query string, args ...any) {
	t.Helper()
	if err := tx.Exec(query, args...).Error; err != nil {
		t.Fatal(err)
	}
}
func seedProject(t *testing.T, tx *gorm.DB, a identity.Principal, scope uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.New()
	run(t, tx, `INSERT INTO workspace.project(id,org_id,owner_id,owner_scope_id,name,aspect_ratio,style_type)VALUES(?,?,?,?,'Synthetic project','16:9','realistic')`, id, a.OrgID, a.ID, scope)
	return id
}
func seedOperation(t *testing.T, tx *gorm.DB, now time.Time, a identity.Principal, project uuid.UUID, status string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	run(t, tx, `INSERT INTO operation.operation(id,project_id,target_type,capability,mode,input_hash,origin,status,model_profile_version_id,region,confirmed_by,confirmed_at,quote_micros,quote_expires_at,create_time)VALUES(?,?,'free','image.generate','standard','synthetic','canvas',?,?,'domestic',?,?,9007199254740993,?,?)`, id, project, status, uuid.New(), a.ID, now.Add(-time.Hour), now.Add(time.Hour), now.Add(-2*time.Hour))
	return id
}
func TestPostgresCreatorOwnershipPlatformRevocationAndSafeHTTP(t *testing.T) {
	tx := analyticsTransaction(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	clock := func() time.Time { return now }
	a := seedActor(t, tx, now, uuid.New())
	peer := seedActor(t, tx, now, a.OrgID)
	foreign := seedActor(t, tx, now, uuid.New())
	ownProject := seedProject(t, tx, a, a.ScopeID)
	peerProject := seedProject(t, tx, peer, peer.ScopeID)
	foreignProject := seedProject(t, tx, foreign, foreign.ScopeID)
	own := seedOperation(t, tx, now, a, ownProject, "completed")
	seedOperation(t, tx, now, peer, peerProject, "completed")
	seedOperation(t, tx, now, foreign, foreignProject, "failed")
	seedOperation(t, tx, now, peer, ownProject, "completed")
	// Confirming user alone is insufficient after access to another personal scope is lost.
	seedOperation(t, tx, now, a, peerProject, "completed")
	run(t, tx, `UPDATE workspace.project SET independent_canvas=true WHERE id=?`, ownProject)
	provider, model, version := uuid.New(), uuid.New(), uuid.New()
	run(t, tx, `INSERT INTO catalog.capability(id,key,output_type,modes,input_roles)VALUES(?,'image.generate','image',ARRAY['standard'],ARRAY['prompt'])ON CONFLICT DO NOTHING`, uuid.New())
	run(t, tx, `INSERT INTO catalog.provider(id,key,name,adapter_key,region)VALUES(?,?,'Synthetic provider','openrouter','domestic')`, provider, "analytics-"+provider.String())
	run(t, tx, `INSERT INTO catalog.model_profile(id,model_key,provider_id,capability,display_name)VALUES(?, ?,?,'image.generate','Synthetic model')`, model, "analytics-"+model.String(), provider)
	run(t, tx, `INSERT INTO catalog.model_profile_version(id,model_profile_id,version_no,provider_model_id,modes,limits,param_schema,supports_query,supports_cancel,supports_callback,expected_max_ms)VALUES(?,?,1,'synthetic',ARRAY['standard'],'{}','[]',false,false,false,1000)`, version, model)
	run(t, tx, `UPDATE operation.operation SET model_profile_version_id=? WHERE id=?`, version, own)
	// Canonical schema provides provider_call_default; no fixture DDL is needed.
	for _, call := range []struct {
		attempt  int
		action   string
		dispatch bool
	}{{1, "submit", true}, {1, "submit", true}, {1, "query", true}, {2, "submit", true}, {2, "query", true}, {3, "submit", false}} {
		var sent *time.Time
		if call.dispatch {
			v := now.Add(-time.Minute)
			sent = &v
		}
		run(t, tx, `INSERT INTO operation.provider_call(id,project_id,operation_id,attempt,action,provider_key,request_summary,outcome,region,dispatch_started_at)VALUES(?,?,?,?,?,'synthetic','{}','ok','domestic',?)`, uuid.New(), ownProject, own, call.attempt, call.action, sent)
	}
	for i := 0; i < 3; i++ {
		run(t, tx, `INSERT INTO operation.operation_output(id,project_id,operation_id,seq_no,kind,json_payload)VALUES(?,?,?,?,'json','{}')`, uuid.New(), ownProject, own, i)
	}
	store := analyticspg.NewStore(tx, func(tx *gorm.DB) analyticspg.Access { return pgidentity.NewStore(tx) })
	query := analytical.NewQuery(store, clock)
	f, _ := analyticsdomain.ParseFilter("", now)
	result, err := query.Creation(t.Context(), a, f)
	if err != nil || result.AcceptedOperations != 1 || result.Attempts != 2 || result.Outputs != 3 || result.RevenueMicros != nil || result.CostMicros != nil {
		t.Fatalf("personal projection %+v %v", result, err)
	}
	if len(result.Models) != 1 || result.Models[0].ModelName == nil || *result.Models[0].ModelName != "Synthetic model" {
		t.Fatal("safe model display missing")
	}
	if result.Timezone != "Asia/Shanghai" {
		t.Fatal("timezone missing")
	}
	// Drafts/quotes cannot increase accepted counts, even if malformed fixtures carry a confirmer.
	seedOperation(t, tx, now, a, ownProject, "quoted")
	seedOperation(t, tx, now, a, ownProject, "draft")
	result, err = query.Creation(t.Context(), a, f)
	if err != nil || result.AcceptedOperations != 1 {
		t.Fatal("quote counted", err)
	}
	platformFilter := f
	platformFilter.Scope = "platform"
	if _, err := query.Creation(t.Context(), a, platformFilter); !errors.Is(err, identity.ErrForbidden) {
		t.Fatal("creator platform analysis", err)
	}
	// Team members see their own accepted work only, and lose it after membership removal.
	team, scope := uuid.New(), uuid.New()
	run(t, tx, `INSERT INTO workspace.team(id,name,created_by)VALUES(?,'Synthetic team',?)`, team, a.ID)
	run(t, tx, `INSERT INTO workspace.team_member(team_id,user_id,role,status,joined_at,source)VALUES(?,?,'member','active',?,'created')`, team, a.ID, now)
	run(t, tx, `INSERT INTO workspace.creation_scope(id,kind,team_id,physical_scope_id)VALUES(?,'team',?,?)`, scope, team, a.OrgID)
	teamProject := seedProject(t, tx, a, scope)
	seedOperation(t, tx, now, a, teamProject, "cancelled")
	seedOperation(t, tx, now, peer, teamProject, "completed")
	result, err = query.Creation(t.Context(), a, f)
	if err != nil || result.AcceptedOperations != 2 {
		t.Fatal("team actor ownership", err)
	}
	run(t, tx, `UPDATE workspace.team_member SET status='removed' WHERE team_id=? AND user_id=?`, team, a.ID)
	result, err = query.Creation(t.Context(), a, f)
	if err != nil || result.AcceptedOperations != 1 {
		t.Fatal("removed member facts exposed", err)
	}
	// The API routes use actual transaction-bound DB authority even with a forged/stale request principal.
	b := memory.NewBuffer(10, time.Hour, clock)
	b.Append(obsdomain.DiagnosticRecord{Timestamp: now.Add(-time.Minute), Method: "GET", Route: "/api/items/:id", Status: 500, RequestID: uuid.NewString()})
	principal := a
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api", func(c *gin.Context) { c.Set("principal", principal) })
	insights.Register(group, tx, b, clock)
	get := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), "GET", path, nil))
		return w
	}
	if w := get("/api/platform/diagnostics/logs"); w.Code != 403 {
		t.Fatalf("creator diagnostics %d", w.Code)
	}
	run(t, tx, `UPDATE identity."user" SET platform_role='platform_admin',platform_epoch=2 WHERE id=?`, a.ID)
	if w := get("/api/platform/diagnostics/logs"); w.Code != 403 {
		t.Fatal("stale platform epoch passed")
	}
	a.PlatformRole = "platform_admin"
	a.PlatformEpoch = 2
	principal = a
	for _, path := range []string{"/api/platform/diagnostics/logs", "/api/platform/diagnostics/overview", "/api/analytics/creation?scope=platform", "/api/analytics/business"} {
		if w := get(path); w.Code != 200 {
			t.Fatalf("%s status %d %s", path, w.Code, w.Body.String())
		}
	}
	business := get("/api/analytics/business")
	if !strings.Contains(business.Body.String(), `"status":"unavailable"`) || !strings.Contains(business.Body.String(), `"cost_micros":null`) {
		t.Fatal("finance capability fabricated")
	}
	for _, path := range []string{"/api/analytics/creation?scope=personal&scope=platform", "/api/analytics/creation?origin=canvas%27%20OR%201%3D1--", "/api/analytics/business?scope=platform", "/api/platform/diagnostics/logs?q=TOP-SECRET-QUERY", "/api/platform/diagnostics/logs?limit=1&limit=2", "/api/platform/diagnostics/logs/not-a-uuid"} {
		w := get(path)
		if w.Code != 422 || strings.Contains(w.Body.String(), "TOP-SECRET") {
			t.Fatalf("unsafe query %d %s", w.Code, w.Body.String())
		}
	}
	obs := obsapp.NewQuery(insights.NewPlatformAccess(tx, clock), b, clock)
	lf, _ := obsdomain.ParseFilter("", now)
	page, err := obs.List(t.Context(), a, lf)
	if err != nil || len(page.Items) != 1 {
		t.Fatal("platform log read", err)
	}
	detailID := page.Items[0].ID
	if w := get("/api/platform/diagnostics/logs/" + detailID.String()); w.Code != 200 {
		t.Fatal("safe detail denied")
	}
	run(t, tx, `UPDATE identity."user" SET platform_role='creator',platform_epoch=3 WHERE id=?`, a.ID)
	if w := get("/api/platform/diagnostics/logs?cursor=" + page.FirstCursor); w.Code != 403 {
		t.Fatalf("demoted admin replayed first page: %d", w.Code)
	}
	if _, err := obs.Detail(t.Context(), a, detailID); !errors.Is(err, identity.ErrForbidden) {
		t.Fatal("demoted admin log detail", err)
	}
	run(t, tx, `UPDATE identity.user_session SET revoked_at=? WHERE id=?`, now, a.SessionID)
	if _, err := query.Creation(t.Context(), a, f); !errors.Is(err, identity.ErrSessionInvalid) {
		t.Fatal("revoked analysis", err)
	}
	if _, err := obs.Detail(t.Context(), a, detailID); !errors.Is(err, identity.ErrSessionInvalid) {
		t.Fatal("revoked log detail", err)
	}
	if w := get("/api/analytics/business"); w.Code != 401 {
		t.Fatalf("revoked capability read %d", w.Code)
	}
	raw, _ := json.Marshal(result)
	if strings.Contains(string(raw), "9007199254740993") {
		t.Fatal("quoted price appeared as actual money")
	}
}
