package canvas_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/coder/websocket/wsjson"
	"github.com/google/uuid"
	"gorm.io/gorm"

	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	pgcanvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	"github.com/StephenQiu30/glowloom-server/backend/tests/canvasfixture"
)

func TestCanvasSnapshotUsesVersionedDTO(t *testing.T) {
	f := newLiveFixture(t)
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, f.server.URL+"/api/canvases/"+f.doc.ID.String(), nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Cookie", "glowloom_session="+f.auth.Token)
	request.Header.Set("X-Creation-Scope", f.actor.ScopeID.String())
	response, err := f.server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != 200 {
		t.Fatal("snapshot", response.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["revision"] != "1" || body["schema_version"] != float64(1) {
		t.Fatalf("unversioned or unsafe revision: %+v", body)
	}
	permissions, ok := body["permissions"].(map[string]any)
	if !ok || permissions["read"] != true || permissions["write"] != true {
		t.Fatalf("missing authority: %+v", body)
	}
	if _, ok := body["read_only_reason"]; !ok {
		t.Fatal("missing nullable read_only_reason")
	}
	if _, ok := body["viewport"]; ok {
		t.Fatal("session viewport exposed as shared graph")
	}
}

func TestCanvasDecimalRevisionRejectsUnsafeAndNonCanonicalJSON(t *testing.T) {
	for _, body := range []string{`1`, `1.5`, `null`, `"01"`, `"+1"`, `"-1"`, `"1e3"`, `"9223372036854775808"`} {
		var revision application.Revision
		if err := json.Unmarshal([]byte(body), &revision); err == nil {
			t.Errorf("unsafe revision accepted: %s", body)
		}
	}
	for _, body := range []string{`"0"`, `"9007199254740993"`, `"9223372036854775807"`} {
		var revision application.Revision
		if err := json.Unmarshal([]byte(body), &revision); err != nil {
			t.Fatal(body, err)
		}
		actual, err := json.Marshal(revision)
		if err != nil || string(actual) != body {
			t.Fatal(string(actual), err)
		}
	}
}

func TestCanvasSubmissionDTORejectsLossyRequests(t *testing.T) {
	valid := `{"client_id":"ac8647b6-3994-4a42-8a9f-fd356ee10432","operation_version":1,"base_revision":"1","operations":[{"type":"create_node","node":{"id":"a252f499-cd9e-4d90-be21-6d9276ddc201","title":"素材","node_type":"image","node_action":"resource","config":{},"media_ref":{"asset_id":"89069a33-0317-4c29-b63f-d13044d087dd"},"x":1,"y":2,"parent_id":null,"z_index":0}}]}`
	input, err := application.DecodeSubmission([]byte(valid))
	if err != nil {
		t.Fatal(err)
	}
	if input.ClientID != canvasfixture.ClientID || input.BaseRevision != 1 || input.Operations[0].Node.RefType != "media_asset" {
		t.Fatal(input)
	}
	for _, body := range []string{
		strings.Replace(valid, `"base_revision":"1"`, `"base_revision":1`, 1),
		strings.Replace(valid, `"client_id":"ac8647b6-3994-4a42-8a9f-fd356ee10432",`, "", 1),
		strings.Replace(valid, `"operation_version":1`, `"operation_version":1,"unknown":true`, 1),
		strings.Replace(valid, `"node_type":"image"`, `"node_type":"image","future_field":true`, 1),
		strings.Replace(valid, `"node_type":"image"`, `"node_type":"image","ref_type":"","ref_id":null`, 1),
		strings.Replace(valid, `"media_ref":{"asset_id":"89069a33-0317-4c29-b63f-d13044d087dd"}`, `"ref_type":"media_asset","ref_id":"89069a33-0317-4c29-b63f-d13044d087dd"`, 1),
		valid + ` {"extra":true}`,
	} {
		if _, err := application.DecodeSubmission([]byte(body)); err == nil {
			t.Fatalf("lossy request accepted: %s", body)
		}
	}
}

func TestCanvasBigintRevisionAndClientIdentityPersistExactly(t *testing.T) {
	f := newLiveFixture(t)
	const base int64 = 9007199254740993
	if err := f.db.Exec(`UPDATE canvas.document SET revision=? WHERE id=?`, base, f.doc.ID).Error; err != nil {
		t.Fatal(err)
	}
	node := resourceNode("text")
	node.Config.Format = "markdown"
	node.Config.FormatVersion = 1
	key := uuid.NewString()
	input := application.CollaborationInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: base, Operations: canvasfixture.Nodes([]domain.Node{node})}
	receipt, err := application.NewCollaborationService(canvasStore(f.db)).Submit(t.Context(), f.actor, f.doc.ID, key, input)
	if err != nil || receipt.Status != "accepted" || receipt.Revision != base+1 {
		t.Fatal(receipt, err)
	}
	var row struct {
		ClientID               uuid.UUID
		BaseRevision, Revision int64
	}
	if err := f.db.Raw(`SELECT client_id,base_revision,revision FROM canvas.command_log WHERE document_id=? AND op_id=?`, f.doc.ID, key).Scan(&row).Error; err != nil || row.ClientID != canvasfixture.ClientID || row.Revision != base+1 || row.BaseRevision != base {
		t.Fatal(row, err)
	}
	doc, err := f.service.Get(t.Context(), f.actor, f.doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	dto := application.Snapshot(doc)
	body, err := json.Marshal(dto)
	if err != nil || !bytes.Contains(body, []byte(`"revision":"9007199254740994"`)) || dto.Nodes[0].Config.Format != "markdown" || dto.Nodes[0].ContentVersion != 1 {
		t.Fatal(string(body), err)
	}
	conn := f.dial(t)
	liveSend(t, conn, map[string]any{"type": "join", "protocol_version": 3, "client_id": canvasfixture.ClientID, "scope_id": f.actor.ScopeID, "csrf_token": f.auth.CSRFToken, "last_revision": "9007199254740993"})
	changes := liveRead(t, conn, "changes", base+1)
	if len(changes.Changes) != 1 || int64(changes.Changes[0].Revision) != base+1 {
		t.Fatal(changes)
	}
	liveRead(t, conn, "ready", base+1)
}

func TestCanvasSnapshotMediaFormatAndReadOnlyPermissions(t *testing.T) {
	for _, kind := range []string{"archived", "future_schema", "future_format", "future_config"} {
		t.Run(kind, func(t *testing.T) {
			f := newLiveFixture(t)
			asset := seedCanvasMedia(t, f.db, f.doc.ProjectID, "image")
			image, text := resourceNode("image"), resourceNode("text")
			image.RefID = &asset
			receipt, err := application.NewCollaborationService(canvasStore(f.db)).Submit(t.Context(), f.actor, f.doc.ID, uuid.NewString(), application.CollaborationInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 1, Operations: canvasfixture.Nodes([]domain.Node{image, text})})
			if err != nil || receipt.Status != "accepted" {
				t.Fatal(receipt, err)
			}
			before, err := f.service.Get(t.Context(), f.actor, f.doc.ID)
			if err != nil {
				t.Fatal(err)
			}
			dto := application.Snapshot(before)
			for _, node := range dto.Nodes {
				if node.ID == image.ID && (node.MediaRef == nil || node.MediaRef.AssetID != asset || node.RefID != nil || node.RefType != "") {
					t.Fatal("invalid media identity", node)
				}
				if node.ID == text.ID && (node.Config.Format != "plain" || node.Config.FormatVersion != 1) {
					t.Fatal(node)
				}
			}
			var query string
			var args []any
			switch kind {
			case "archived":
				query = `UPDATE workspace.project SET status='archived',archived_at=now() WHERE id=?`
				args = []any{f.doc.ProjectID}
			case "future_schema":
				query = `UPDATE canvas.document SET schema_version=99 WHERE id=?`
				args = []any{f.doc.ID}
			case "future_format":
				query = `UPDATE canvas.node SET config=config || '{"format":"future","format_version":99}'::jsonb WHERE id=?`
				args = []any{text.ID}
			case "future_config":
				query = `UPDATE canvas.node SET config=config || '{"future_content":{"protected":"keep me"}}'::jsonb WHERE id=?`
				args = []any{text.ID}
			}
			if err := f.db.Exec(query, args...).Error; err != nil {
				t.Fatal(err)
			}
			after, err := f.service.Get(t.Context(), f.actor, f.doc.ID)
			if err != nil {
				t.Fatal(err)
			}
			dto = application.Snapshot(after)
			if !dto.Permissions.Read || dto.Permissions.Write || len(dto.Nodes) != 2 {
				t.Fatal(dto)
			}
			if dto.ReadOnlyReason == nil {
				t.Fatal("missing read-only reason")
			}
			body, err := json.Marshal(dto)
			if err != nil {
				t.Fatal(err)
			}
			if kind == "future_config" && !bytes.Contains(body, []byte(`"protected":"keep me"`)) {
				t.Fatal("future content lost", string(body))
			}
			conn := f.dial(t)
			f.join(t, conn)
			liveSend(t, conn, map[string]any{"type": "sync"})
			heartbeat := liveRead(t, conn, "heartbeat", 2)
			if heartbeat.Permissions.Write {
				t.Fatal("read-only graph advertised as writable", heartbeat)
			}
		})
	}
}

func TestCanvasProtocolNegotiatesVersionAndExplicitErrorFields(t *testing.T) {
	f := newLiveFixture(t)
	for _, version := range []int{1, 2, 99} {
		conn := f.dial(t)
		liveSend(t, conn, map[string]any{"type": "join", "protocol_version": version, "client_id": canvasfixture.ClientID, "scope_id": f.actor.ScopeID, "csrf_token": f.auth.CSRFToken, "last_revision": "0"})
		var packet map[string]any
		if err := wsjson.Read(t.Context(), conn, &packet); err != nil {
			t.Fatal(err)
		}
		supported, ok := packet["supported_versions"].([]any)
		if packet["code"] != "unsupported_protocol_version" || packet["retryable"] != false || !ok || len(supported) != 1 || supported[0] != float64(3) {
			t.Fatal(packet)
		}
	}
	conn := f.dial(t)
	f.join(t, conn)
	key := uuid.NewString()
	node := resourceNode("text")
	body := application.CollaborationInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 1, Operations: canvasfixture.Nodes([]domain.Node{node})}
	liveSend(t, conn, map[string]any{"type": "resolve", "op_id": key, "body": body})
	packet := liveRead(t, conn, "receipt_not_found", 0)
	if packet.OpID != key || packet.Receipt != nil {
		t.Fatal(packet)
	}
	var count int64
	if err := f.db.Raw(`SELECT count(*) FROM canvas.save_receipt WHERE document_id=?`, f.doc.ID).Scan(&count).Error; err != nil || count != 0 {
		t.Fatal("resolve executed request", count, err)
	}
}

func TestCanvasProtocol3ResolvesOriginalPreClientIDReceiptWithoutExecuting(t *testing.T) {
	for _, canonical := range []bool{false, true} {
		t.Run(map[bool]string{false: "original hash", true: "canonical hash"}[canonical], func(t *testing.T) {
			f := newLiveFixture(t)
			node := resourceNode("text")
			key := uuid.NewString()
			input := application.CollaborationInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 1, Operations: canvasfixture.Nodes([]domain.Node{node})}
			receipt, err := application.NewCollaborationService(canvasStore(f.db)).Submit(t.Context(), f.actor, f.doc.ID, key, input)
			if err != nil || receipt.Status != "accepted" {
				t.Fatal(receipt, err)
			}
			legacy, err := json.Marshal(struct {
				OperationVersion int                `json:"operation_version"`
				BaseRevision     int64              `json:"base_revision"`
				Operations       []domain.Operation `json:"operations"`
			}{1, 1, input.Operations})
			if err != nil {
				t.Fatal(err)
			}
			hashed := legacy
			if canonical {
				var fields any
				decoder := json.NewDecoder(bytes.NewReader(legacy))
				decoder.UseNumber()
				if err := decoder.Decode(&fields); err != nil {
					t.Fatal(err)
				}
				hashed, err = json.Marshal(fields)
				if err != nil {
					t.Fatal(err)
				}
			}
			hash := sha256.Sum256(append([]byte("collaboration/"+f.doc.ID.String()+"\x00"), hashed...))
			if err := f.db.Exec(`UPDATE canvas.save_receipt SET request_hash=? WHERE actor_id=? AND idem_key=?`, hex.EncodeToString(hash[:]), f.actor.ID, key).Error; err != nil {
				t.Fatal(err)
			}
			conn := f.dial(t)
			f.join(t, conn)
			liveSend(t, conn, map[string]any{"type": "resolve", "op_id": key, "body": json.RawMessage(legacy)})
			ack := liveRead(t, conn, "ack", 2)
			if ack.Receipt == nil || ack.Receipt.Status != "accepted" || ack.Receipt.Revision != 2 {
				t.Fatal(ack)
			}
			// The same legacy request can be checked, never submitted as a new edit.
			liveSend(t, conn, map[string]any{"type": "submit", "op_id": uuid.NewString(), "body": json.RawMessage(legacy)})
			if failure := liveError(t, conn); failure.Code != "invalid_request" {
				t.Fatal(failure)
			}
			var facts struct{ Revision, Logs, Receipts int64 }
			if err := f.db.Raw(`SELECT revision,(SELECT count(*) FROM canvas.command_log WHERE document_id=?) logs,(SELECT count(*) FROM canvas.save_receipt WHERE document_id=?) receipts FROM canvas.document WHERE id=?`, f.doc.ID, f.doc.ID, f.doc.ID).Scan(&facts).Error; err != nil || facts.Revision != 2 || facts.Logs != 1 || facts.Receipts != 1 {
				t.Fatal(facts, err)
			}
		})
	}
}

func TestCanvasJoinRequiresCanonicalBaselineAndClientID(t *testing.T) {
	f := newLiveFixture(t)
	for _, fields := range []map[string]any{
		{"last_revision": "0"},
		{"client_id": canvasfixture.ClientID},
		{"client_id": canvasfixture.ClientID, "last_revision": 0},
		{"client_id": canvasfixture.ClientID, "last_revision": "01"},
		{"client_id": canvasfixture.ClientID, "last_revision": "0", "unknown": true},
	} {
		conn := f.dial(t)
		fields["type"] = "join"
		fields["protocol_version"] = 3
		fields["scope_id"] = f.actor.ScopeID
		fields["csrf_token"] = f.auth.CSRFToken
		liveSend(t, conn, fields)
		failure := liveError(t, conn)
		if failure.Status != 422 || (failure.Code != "invalid_join" && failure.Code != "invalid_message") {
			t.Fatal(failure)
		}
	}
}

func TestCanvasCopyRejectsUnsupportedSchemaBeforeFreezing(t *testing.T) {
	f := newLiveFixture(t)
	target := uuid.New()
	if err := f.db.Exec(`INSERT INTO workspace.project(id,org_id,owner_id,owner_scope_id,name,aspect_ratio,style_type,status) VALUES(?,?,?,?,'copy target','16:9','realistic','copying')`, target, f.actor.OrgID, f.actor.ID, f.actor.ScopeID).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Exec(`UPDATE canvas.document SET schema_version=99 WHERE id=?`, f.doc.ID).Error; err != nil {
		t.Fatal(err)
	}
	binding := application.ProjectCopyBinding{JobID: uuid.New(), OrgID: f.actor.OrgID, SourceProjectID: f.doc.ProjectID, TargetProjectID: target}
	err := f.db.Transaction(func(tx *gorm.DB) error {
		_, err := pgcanvas.NewScopedProjectCopyStore(tx, nil, nil, mediawire.NewCanvasProjectAccess).Freeze(t.Context(), f.actor, binding, map[uuid.UUID]uuid.UUID{})
		return err
	})
	if !errors.Is(err, domain.ErrUnsupportedProjectCopy) {
		t.Fatal("future schema silently normalized by project copy", err)
	}
	var count int64
	if err := f.db.Raw(`SELECT count(*) FROM canvas.project_copy_snapshot WHERE job_id=?`, binding.JobID).Scan(&count).Error; err != nil || count != 0 {
		t.Fatal(count, err)
	}
}

func TestCanvasSnapshotTranslatesLegacyMetadataWithoutMutatingContent(t *testing.T) {
	text := resourceNode("text")
	doc := domain.Document{ID: uuid.New(), ProjectID: uuid.New(), Scope: json.RawMessage(`{}`), Revision: 1, Writable: true, Viewport: domain.Viewport{Zoom: 1}, Nodes: []domain.Node{text}, Edges: []domain.Edge{}}
	dto := application.Snapshot(doc)
	if dto.SchemaVersion != 1 || !dto.Permissions.Write || dto.Nodes[0].Config.Format != "plain" || dto.Nodes[0].Config.FormatVersion != 1 {
		t.Fatal(dto)
	}
	if doc.SchemaVersion != 0 || doc.Nodes[0].Config.Format != "" || doc.Viewport.Zoom != 1 {
		t.Fatal("DTO conversion rewrote historical content", doc)
	}
	doc.Nodes[0].Config.Format = "future"
	doc.Nodes[0].Config.FormatVersion = 99
	dto = application.Snapshot(doc)
	if dto.Permissions.Write || dto.ReadOnlyReason == nil || dto.Nodes[0].Config.Format != "future" {
		t.Fatal(dto)
	}
}
