package canvas_test

import (
	"testing"

	canvasfixture "github.com/StephenQiu30/glowloom-server/backend/tests/canvasfixture"

	"github.com/coder/websocket/wsjson"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
)

func TestCollaborationLiveOrderedChangesAndOriginalReceipt(t *testing.T) {
	f := newLiveFixture(t)
	a, b := f.dial(t), f.dial(t)
	join := map[string]any{"type": "join", "protocol_version": 3, "client_id": canvasfixture.ClientID, "last_revision": "0", "scope_id": f.actor.ScopeID, "csrf_token": f.auth.CSRFToken}
	if err := wsjson.Write(t.Context(), a, join); err != nil {
		t.Fatal(err)
	}
	liveRead(t, a, "snapshot", 1)
	liveRead(t, a, "ready", 1)
	if err := wsjson.Write(t.Context(), b, join); err != nil {
		t.Fatal(err)
	}
	liveRead(t, b, "snapshot", 1)
	liveRead(t, b, "ready", 1)
	n := resourceNode("text")
	key := uuid.NewString()
	body := application.CollaborationInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 1, Operations: []domain.Operation{{Type: "create_node", Node: &n}}}
	liveSend(t, a, map[string]any{"type": "submit", "op_id": key, "body": body})
	liveRead(t, a, "ack", 2)
	liveRead(t, b, "changes", 2)
	liveSend(t, b, map[string]any{"type": "resolve", "op_id": key, "body": body})
	liveRead(t, b, "ack", 2)
	loaded, err := f.service.Get(t.Context(), f.actor, f.doc.ID)
	if err != nil || len(loaded.Nodes) != 1 {
		t.Fatal(loaded, err)
	}
}
