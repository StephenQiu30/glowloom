package observability_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	obshttp "github.com/StephenQiu30/glowloom-server/backend/internal/observability/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/observability/adapter/memory"
	"github.com/StephenQiu30/glowloom-server/backend/internal/observability/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

func TestLogFilterRejectsAmbiguityAndBounds(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	for _, raw := range []string{"unknown=x", "limit=1&limit=2", "limit=101", "from=bad", "from=2026-11-01T00:00:00Z&to=2026-11-13T00:00:00Z", "level=warning", "service=agent", "request_id=secret", "trace_id=secret", "cursor=bad", "status=200", "q=secret", "from=2026-11-13T12:00:00Z&to=2026-11-13T11:00:00Z"} {
		if _, err := domain.ParseFilter(raw, now); err == nil {
			t.Errorf("accepted %q", raw)
		}
	}
	f, err := domain.ParseFilter("level=error&limit=2", now)
	if err != nil || f.Limit != 2 || f.To.Sub(f.From) != 24*time.Hour {
		t.Fatalf("valid defaults: %+v %v", f, err)
	}
}

func TestRequestCaptureNeverStoresSecretsAndPreservesPanicOutcome(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Now().UTC()
	buffer := memory.NewBuffer(20, 24*time.Hour, func() time.Time { return now })
	core, seen := observer.New(zap.DebugLevel)
	r := gin.New()
	r.Use(obshttp.RequestCompletion(zap.New(core), buffer))
	r.GET("/api/items/:id", func(c *gin.Context) { _ = c.Error(&privateError{}); c.Status(503) })
	r.GET("/api/panic", func(*gin.Context) { panic("TOP-SECRET-PANIC") })
	r.GET("/healthz", func(c *gin.Context) { c.Status(200) })
	r.GET("/api/platform/diagnostics/logs", func(c *gin.Context) { c.Status(200) })
	for _, path := range []string{"/api/items/TOP-SECRET-PATH?token=TOP-SECRET-QUERY", "/api/panic", "/healthz", "/api/platform/diagnostics/logs", "/TOP-SECRET-UNMATCHED"} {
		w := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, strings.NewReader("TOP-SECRET-BODY"))
		req.Header.Set("Authorization", "Bearer TOP-SECRET-HEADER")
		req.Header.Set("X-Request-Id", "TOP-SECRET-ID")
		r.ServeHTTP(w, req)
		if path == "/api/panic" && w.Code != 500 {
			t.Fatalf("panic status %d", w.Code)
		}
	}
	f, _ := domain.ParseFilter("", now.Add(time.Second))
	page, err := buffer.List(f, uuid.New())
	if err != nil || len(page.Items) != 3 {
		t.Fatalf("capture: %+v %v", page, err)
	}
	data, _ := json.Marshal(page)
	if strings.Contains(string(data), "TOP-SECRET") {
		t.Fatal("record leaked secret", string(data))
	}
	for _, entry := range seen.All() {
		data, _ := json.Marshal(entry.ContextMap())
		if strings.Contains(string(data), "TOP-SECRET") {
			t.Fatal("fallback logger leaked secret")
		}
	}
	found := false
	for _, item := range page.Items {
		if item.Route == "/api/panic" {
			found = item.Status == 500 && item.ErrorClass == "internal_error"
		}
	}
	if !found {
		t.Fatal("panic completion absent")
	}
}

type privateError struct{}

func (*privateError) Error() string { return "TOP-SECRET-ERROR" }

func TestBufferCursorBindsActorFiltersSnapshotAndRestart(t *testing.T) {
	now := time.Now().UTC()
	clock := func() time.Time { return now }
	b := memory.NewBuffer(3, time.Hour, clock)
	actor := uuid.New()
	for i := 0; i < 3; i++ {
		b.Append(domain.DiagnosticRecord{Timestamp: now, Method: "GET", Route: "/api/items/:id", Status: 200, RequestID: uuid.NewString()})
	}
	f, _ := domain.ParseFilter("limit=1", now.Add(time.Second))
	p, err := b.List(f, actor)
	if err != nil || p.NextCursor == nil {
		t.Fatal("first page", err)
	}
	f.Cursor = *p.NextCursor
	next, err := b.List(f, actor)
	if err != nil || len(next.Items) != 1 || next.Items[0].ID == p.Items[0].ID {
		t.Fatal("cursor did not advance", err)
	}
	if _, err := b.List(f, uuid.New()); err == nil {
		t.Fatal("cross actor cursor")
	}
	changed := f
	changed.Level = "error"
	if _, err := b.List(changed, actor); err == nil {
		t.Fatal("cross filter cursor")
	}
	if _, err := memory.NewBuffer(3, time.Hour, clock).List(f, actor); err == nil {
		t.Fatal("restart cursor")
	}
	b.Append(domain.DiagnosticRecord{Timestamp: now, Status: 200})
	b.Append(domain.DiagnosticRecord{Timestamp: now, Status: 200})
	b.Append(domain.DiagnosticRecord{Timestamp: now, Status: 200})
	if _, err := b.List(f, actor); err == nil {
		t.Fatal("evicted snapshot silently accepted")
	}
}

func TestBufferConcurrentCaptureBounded(t *testing.T) {
	now := time.Now().UTC()
	b := memory.NewBuffer(30, time.Hour, func() time.Time { return now })
	actor := uuid.New()
	f, _ := domain.ParseFilter("", now.Add(time.Second))
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			for range 100 {
				b.Append(domain.DiagnosticRecord{Timestamp: now, Status: 200})
				_, _ = b.List(f, actor)
			}
		})
	}
	wg.Wait()
	p, err := b.List(f, actor)
	if err != nil || len(p.Items) != 30 || p.Coverage.Evicted != 970 {
		t.Fatalf("bounds %+v %v", p.Coverage, err)
	}
}

func TestAlreadyWrittenPanicRemainsHTTP200ButIsDiagnosticError(t *testing.T) {
	now := time.Now().UTC()
	b := memory.NewBuffer(5, time.Hour, func() time.Time { return now })
	r := gin.New()
	r.Use(obshttp.RequestCompletion(zap.NewNop(), b), httpapi.Middleware("http://localhost"))
	r.GET("/api/written-panic", func(c *gin.Context) { c.String(200, "sent"); panic("TOP-SECRET-WRITTEN-PANIC") })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), "GET", "/api/written-panic", nil))
	if w.Code != 200 || w.Body.String() != "sent" {
		t.Fatalf("changed already sent response %d %s", w.Code, w.Body.String())
	}
	f, _ := domain.ParseFilter("level=error", now.Add(time.Second))
	p, err := b.List(f, uuid.New())
	if err != nil || len(p.Items) != 1 || p.Items[0].ErrorClass != "internal_error" {
		t.Fatalf("panic hidden from diagnostics %+v %v", p, err)
	}
}

func TestOverviewUsesNearestRankQuantilesAndExpiresRetainedRecords(t *testing.T) {
	now := time.Now().UTC()
	b := memory.NewBuffer(4, time.Hour, func() time.Time { return now })
	actor := uuid.New()
	for _, ms := range []float64{100, 1000} {
		b.Append(domain.DiagnosticRecord{Timestamp: now.Add(-time.Minute), Status: 200, ElapsedMS: ms})
	}
	b.Append(domain.DiagnosticRecord{Timestamp: now.Add(-time.Minute), Status: 101, ElapsedMS: 300000})
	f, _ := domain.ParseFilter("", now)
	out, err := b.Overview(f, actor)
	if err != nil || out.P95MS == nil || *out.P95MS != 1000 || out.Requests != 3 {
		t.Fatalf("quantile %+v %v", out, err)
	}
	now = now.Add(2 * time.Hour)
	f, _ = domain.ParseFilter("", now)
	out, err = b.Overview(f, actor)
	if err != nil || out.Requests != 0 || out.P95MS != nil || out.ServerErrorRate != nil || out.Coverage.Evicted != 3 {
		t.Fatalf("expired coverage %+v %v", out, err)
	}
}

func TestBufferCursorRejectsChangedRangeTamperingAndExpiredContinuation(t *testing.T) {
	now := time.Now().UTC()
	b := memory.NewBuffer(5, time.Hour, func() time.Time { return now })
	actor := uuid.New()
	for range 3 {
		b.Append(domain.DiagnosticRecord{Timestamp: now.Add(-time.Minute), Status: 200})
	}
	f, _ := domain.ParseFilter("limit=1", now)
	p, err := b.List(f, actor)
	if err != nil || p.NextCursor == nil {
		t.Fatal("initial cursor", err)
	}
	f.Cursor = *p.NextCursor
	changed := f
	changed.From = changed.From.Add(time.Minute)
	changed.FromSet = true
	if _, err := b.List(changed, actor); err == nil {
		t.Fatal("cursor range changed")
	}
	changed = f
	changed.Cursor = "A" + changed.Cursor[1:]
	if _, err := b.List(changed, actor); err == nil {
		t.Fatal("unsigned cursor accepted")
	}
	now = now.Add(11 * time.Minute)
	if _, err := b.List(f, actor); err == nil {
		t.Fatal("expired cursor accepted")
	}
}

type brokenRecorder struct{}

func (brokenRecorder) Append(domain.DiagnosticRecord) { panic("TOP-SECRET-CAPTURE-FAILURE") }
func TestCaptureFailureDoesNotChangeBusinessResponse(t *testing.T) {
	r := gin.New()
	r.Use(obshttp.RequestCompletion(zap.NewNop(), brokenRecorder{}))
	r.GET("/api/success", func(c *gin.Context) { c.Status(204) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), "GET", "/api/success", nil))
	if w.Code != 204 {
		t.Fatalf("capture changed response %d", w.Code)
	}
}

func TestRouterCompletionPreservesActiveTraceForSuccessPanicAndFiltering(t *testing.T) {
	for _, scenario := range []struct {
		name, path string
		status     int
		panic      bool
		traced     bool
	}{
		{"generated_server_trace", "/api/trace-success", 200, false, true},
		{"public_panic_recovery", "/api/trace-panic", 500, true, true},
		{"excluded_route", "/api/media-export-launch", 200, false, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			exporter := tracetest.NewInMemoryExporter()
			provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
			t.Cleanup(func() { _ = provider.Shutdown(context.WithoutCancel(t.Context())) })
			buffer := memory.NewBuffer(10, time.Hour, time.Now)
			core, seen := observer.New(zap.DebugLevel)
			router := app.NewRouter(zap.New(core), nil, provider, buffer)
			router.Use(httpapi.Middleware("http://localhost"))
			var activeTrace string
			router.GET(scenario.path, func(c *gin.Context) {
				span := trace.SpanFromContext(c.Request.Context()).SpanContext()
				if span.IsValid() {
					activeTrace = span.TraceID().String()
				}
				if scenario.panic {
					panic("TOP-SECRET-ROUTER-PANIC")
				}
				c.Status(200)
			})
			request := httptest.NewRequestWithContext(t.Context(), "GET", scenario.path+"?token=TOP-SECRET-QUERY", nil)
			request.Header.Set("traceparent", "TOP-SECRET-INVALID-TRACEPARENT")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != scenario.status {
				t.Fatalf("HTTP status %d, want %d", response.Code, scenario.status)
			}
			filter, err := domain.ParseFilter("", time.Now().UTC())
			if err != nil {
				t.Fatal(err)
			}
			actor := uuid.New()
			page, err := buffer.List(filter, actor)
			if err != nil || len(page.Items) != 1 {
				t.Fatalf("completion records %d, error %v", len(page.Items), err)
			}
			record := page.Items[0]
			if record.Status != scenario.status || record.Panic != scenario.panic {
				t.Fatalf("completion status=%d panic=%v", record.Status, record.Panic)
			}
			logs := seen.FilterMessage("http request").All()
			if len(logs) != 1 {
				t.Fatalf("completion logs %d", len(logs))
			}
			loggedTrace, _ := logs[0].ContextMap()["trace_id"].(string)
			if scenario.traced {
				if activeTrace == "" {
					t.Fatal("handler received no active server trace")
				}
				if record.TraceID != activeTrace || loggedTrace != activeTrace {
					t.Fatalf("trace association lost: handler=%q record=%q logger=%q", activeTrace, record.TraceID, loggedTrace)
				}
				spans := exporter.GetSpans()
				if len(spans) != 1 || spans[0].SpanContext.TraceID().String() != activeTrace {
					t.Fatal("record does not match exported server span")
				}
				filter.TraceID = activeTrace
				filtered, err := buffer.List(filter, actor)
				if err != nil || len(filtered.Items) != 1 || filtered.Items[0].ID != record.ID {
					t.Fatalf("trace filter missed completion: %v", err)
				}
				filter.TraceID = "11111111111111111111111111111111"
				filtered, err = buffer.List(filter, actor)
				if err != nil || len(filtered.Items) != 0 {
					t.Fatal("trace filter widened result", err)
				}
			} else if activeTrace != "" || record.TraceID != "" || loggedTrace != "" || len(exporter.GetSpans()) != 0 {
				t.Fatal("excluded route fabricated trace association")
			}
			safeRecord, _ := json.Marshal(record)
			safeLog, _ := json.Marshal(logs[0].ContextMap())
			if strings.Contains(string(safeRecord), "TOP-SECRET") || strings.Contains(string(safeLog), "TOP-SECRET") || strings.Contains(response.Body.String(), "TOP-SECRET") {
				t.Fatal("trace completion exposed private request or panic contents")
			}
		})
	}
}

func TestBufferFirstCursorAfterWrapAndExpiry(t *testing.T) {
	now := time.Now().UTC()
	b := memory.NewBuffer(4, time.Hour, func() time.Time { return now })
	actor := uuid.New()
	for i := range 6 {
		b.Append(domain.DiagnosticRecord{Timestamp: now.Add(time.Duration(i-6) * time.Minute), ElapsedMS: float64(i)})
	}
	read := func(token string) domain.Page {
		t.Helper()
		f, err := domain.ParseFilter("limit=1", now)
		if err != nil {
			t.Fatal(err)
		}
		f.Cursor = token
		page, err := b.List(f, actor)
		if err != nil {
			t.Fatal(err)
		}
		return page
	}
	// Expire the oldest retained record after wrap-around.
	now = now.Add(56*time.Minute + time.Second)
	first := read("")
	if len(first.Items) != 1 || first.Items[0].ElapsedMS != 5 || first.NextCursor == nil || first.Coverage.Evicted != 3 {
		t.Fatalf("wrapped first page: %+v", first)
	}
	wire, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	var receipt struct {
		First string `json:"first_cursor"`
	}
	if err := json.Unmarshal(wire, &receipt); err != nil || receipt.First == "" {
		t.Fatal("missing signed first_cursor")
	}
	changed, _ := domain.ParseFilter("limit=2", now)
	changed.Cursor = receipt.First
	if _, err := b.List(changed, actor); err == nil {
		t.Fatal("first cursor accepted changed page size")
	}
	second := read(*first.NextCursor)
	now = now.Add(time.Millisecond)
	b.Append(domain.DiagnosticRecord{Timestamp: now.Add(-time.Millisecond), ElapsedMS: 99})
	back := read(receipt.First)
	if !back.AsOf.Equal(first.AsOf) || back.NextCursor == nil {
		t.Fatal("invalid first-page receipt")
	}
	again := read(*back.NextCursor)
	if back.Items[0].ID != first.Items[0].ID || again.Items[0].ID != second.Items[0].ID || !first.AsOf.Equal(back.AsOf) || !first.AsOf.Equal(again.AsOf) {
		t.Fatal("snapshot changed")
	}
	b.Append(domain.DiagnosticRecord{Timestamp: now})
	f, _ := domain.ParseFilter("limit=1", now)
	f.Cursor = receipt.First
	if _, err := b.List(f, actor); err == nil {
		t.Fatal("evicted first cursor accepted")
	}
}

func TestFirstCursorPreservesSingleExplicitDateAtRangeLimit(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	b := memory.NewBuffer(4, time.Hour, func() time.Time { return now })
	actor := uuid.New()
	b.Append(domain.DiagnosticRecord{Timestamp: now.Add(-time.Minute)})
	raw := "from=" + now.Add(-7*24*time.Hour).Format(time.RFC3339Nano)
	f, err := domain.ParseFilter(raw, now)
	if err != nil {
		t.Fatal(err)
	}
	first, err := b.List(f, actor)
	if err != nil || len(first.Items) != 1 {
		t.Fatal("missing initial row", err)
	}
	now = now.Add(time.Minute)
	f, err = domain.ParseFilter(raw+"&cursor="+first.FirstCursor, now)
	if err != nil {
		t.Fatal("moving implicit upper bound invalidated signed range", err)
	}
	back, err := b.List(f, actor)
	if err != nil || len(back.Items) != 1 || back.Items[0].ID != first.Items[0].ID || !back.To.Equal(first.To) {
		t.Fatal("signed range not restored", err)
	}
}
