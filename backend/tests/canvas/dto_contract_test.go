package canvas_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

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

func TestCanvasSnapshotMediaFormatAndReadOnlyPermissions(t *testing.T) {
	for _, kind := range []string{"archived", "future_schema", "future_format", "future_config"} {
		t.Run(kind, func(t *testing.T) {
			f := newLiveFixture(t)
			asset := seedCanvasMedia(t, f.db, f.doc.ProjectID, "image")
			image, text := resourceNode("image"), resourceNode("text")
			image.RefID = &asset
			receipt, err := canvasfixture.NewLegacySeeder(f.db, canvasStore(f.db)).Submit(t.Context(), f.actor, f.doc.ID, uuid.NewString(), canvasfixture.EditInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 1, Operations: canvasfixture.Nodes([]domain.Node{image, text})})
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
			liveSend(t, conn, map[string]any{"type": "join", "protocol_version": 4, "client_id": uuid.New(), "scope_id": f.actor.ScopeID, "csrf_token": f.auth.CSRFToken})
			packet := readYjsUntil(t, conn, func(p yjsPacket) bool { return p.Type == "yjs_migration_required" })
			if packet.State.Document.Permissions.Write {
				t.Fatal("protected graph advertised writable")
			}
		})
	}
}
