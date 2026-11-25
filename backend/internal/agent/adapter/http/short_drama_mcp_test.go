package http

import (
	"bytes"
	"context"
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	app "github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
)

func TestShortDramaMCPToolsetGuards(t *testing.T) {
	gin.SetMode(gin.TestMode)
	token := strings.Repeat("d", 32)
	tools := ShortDramaMCPTools(ShortDramaExecutors{
		"script_upsert_event_graph": func(_ context.Context, _ domain.Scope, _ uuid.UUID, raw json.RawMessage) (domain.CallResult, error) {
			return domain.CallResult{Status: domain.CallStatusCompleted, Result: raw}, nil
		},
		"media_submit_video_i2v": func(_ context.Context, _ domain.Scope, _ uuid.UUID, raw json.RawMessage) (domain.CallResult, error) {
			return domain.CallResult{Status: domain.CallStatusCompleted, Result: raw}, nil
		},
	})

	router := gin.New()
	NewBusinessHandler(nil, app.Capabilities{Tools: tools}, token).Register(router)

	callMCP := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(stdhttp.MethodPost, "/internal/agent-mcp", bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Agent-Audience", "agent-business")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("X-Glowloom-Actor-Id", uuid.NewString())
		req.Header.Set("X-Glowloom-Org-Id", uuid.NewString())
		req.Header.Set("X-Glowloom-Scope-Id", uuid.NewString())
		req.Header.Set("X-Glowloom-Project-Id", uuid.NewString())
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	// 1. tools/list exposes all 5 short-drama groups (20 atomic tools).
	listRec := callMCP(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if listRec.Code != stdhttp.StatusOK ||
		!strings.Contains(listRec.Body.String(), "script_upsert_event_graph") ||
		!strings.Contains(listRec.Body.String(), "canvas_split_grid_2x2") ||
		!strings.Contains(listRec.Body.String(), "media_submit_lipsync") ||
		!strings.Contains(listRec.Body.String(), "timeline_export_render") {
		t.Fatalf("missing expected short-drama MCP tools: %s", listRec.Body.String())
	}

	// 2. Write tool rejects missing expected_version optimistic lock.
	noLockRec := callMCP(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"script_upsert_event_graph","arguments":{"events":[]}}}`)
	if !strings.Contains(noLockRec.Body.String(), "optimistic_lock_required") {
		t.Fatalf("expected optimistic_lock_required error, got: %s", noLockRec.Body.String())
	}

	// 3. Billable I2V tool rejects unapproved call without decision_id or manual_canvas_redraw.
	unapprovedRec := callMCP(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"media_submit_video_i2v","arguments":{"shot_id":"s1","anchor_asset_id":"a1"}}}`)
	if !strings.Contains(unapprovedRec.Body.String(), "approval_required") {
		t.Fatalf("expected approval_required error, got: %s", unapprovedRec.Body.String())
	}

	// 4. Billable I2V tool succeeds with valid decision_id or manual_canvas_redraw=true.
	approvedRec := callMCP(`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"media_submit_video_i2v","arguments":{"shot_id":"s1","anchor_asset_id":"a1","decision_id":"dec-1"}}}`)
	if strings.Contains(approvedRec.Body.String(), `"isError":true`) {
		t.Fatalf("expected approved billable tool call to succeed, got: %s", approvedRec.Body.String())
	}
	redrawRec := callMCP(`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"media_submit_video_i2v","arguments":{"shot_id":"s1","anchor_asset_id":"a1","manual_canvas_redraw":true}}}`)
	if strings.Contains(redrawRec.Body.String(), `"isError":true`) {
		t.Fatalf("expected manual canvas redraw call to succeed, got: %s", redrawRec.Body.String())
	}
}
