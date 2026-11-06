package runtimeclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestRequestStreamsEventsWithIdentityAndCancellation(t *testing.T) {
	canceled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/prefix/internal/agent/runs/run/events" || r.URL.Query().Get("actor_id") != "actor" || r.URL.Query().Get("project_id") != "project" {
			t.Errorf("unexpected request URL: %s", r.URL.String())
		}
		if r.Header.Get("Authorization") != "Bearer "+strings.Repeat("t", 32) || r.Header.Get("X-Agent-Audience") != "agent" || r.Header.Get("Last-Event-ID") != "cursor" {
			t.Error("missing identity or event cursor")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: first\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(canceled)
	}))
	defer server.Close()
	client, err := New(server.URL+"/prefix", strings.Repeat("t", 32))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	response, err := client.Request(ctx, "subscribeAgentEvents", map[string]string{"rid": "run"}, url.Values{"actor_id": {"actor"}, "project_id": {"project"}}, nil, "cursor")
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, len("data: first\n\n"))
	if _, err := io.ReadFull(response.Body, buf); err != nil || string(buf) != "data: first\n\n" {
		t.Fatalf("stream = %q, %v", buf, err)
	}
	cancel()
	_ = response.Body.Close()
	<-canceled
}

func TestGeneratedNestedContractJSON(t *testing.T) {
	raw := `{"actor_id":"a","org_id":"o","scope_id":"s","project_id":"p","id":"i","title":"title","execution_revision":7,"run":{"execution_available":true,"messages_page":{"items":[{"message_id":"m","run_id":"r","conversation_id":1,"sequence":"2","part_index":0,"kind":"assistant","content":"hello"}],"through":"2"},"root_run_id":"r","session_id":"i","submission_id":"sub","goal":"goal","manifest":{"nested":{"enabled":true}},"state":{"status":"running","execution_revision":7},"snapshot_cursor":"2","events":[]}}`
	var session SessionView
	if err := json.Unmarshal([]byte(raw), &session); err != nil {
		t.Fatal(err)
	}
	if session.Run == nil || len(session.Run.MessagesPage.Items) != 1 || session.Run.MessagesPage.Items[0].Content != "hello" || session.Run.Manifest["nested"].(map[string]interface{})["enabled"] != true {
		t.Fatalf("nested response lost: %#v", session.Run)
	}
	encoded, err := json.Marshal(session)
	if err != nil {
		t.Fatal(err)
	}
	var roundtrip map[string]interface{}
	if err := json.Unmarshal(encoded, &roundtrip); err != nil {
		t.Fatal(err)
	}
	if _, exists := roundtrip["canvas_id"]; exists {
		t.Fatal("absent optional canvas_id became present")
	}
	var current CurrentSubmission
	if err := json.Unmarshal([]byte(`{"active_run":null,"latest_accepted":null,"unresolved_run_ids":["r"]}`), &current); err != nil {
		t.Fatal(err)
	}
	if current.ActiveRun != nil || current.LatestAccepted != nil || current.UnresolvedRunIds == nil || len(*current.UnresolvedRunIds) != 1 {
		t.Fatalf("current submission = %#v", current)
	}
}
