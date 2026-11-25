package canvas_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	canvas "github.com/StephenQiu30/glowloom-server/backend/internal/creation/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/creation/canvas/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/tests/canvasfixture"
)

// TestYjsOfflineUpgrade exercises data before/after an externally administered
// offline conversion. It never changes database structure or performs conversion.
// The seed phase runs against the old adapter in an isolated source copy.
func TestYjsOfflineUpgrade(t *testing.T) {
	phase := os.Getenv("GLOWLOOM_CANVAS_OFFLINE_PHASE")
	if phase == "" {
		t.Skip("opt-in isolated offline-upgrade rehearsal")
	}
	if phase != "seed" && phase != "audit-old" && phase != "verify" && phase != "verify-old" {
		t.Fatal("unsupported offline rehearsal phase")
	}
	name := os.Getenv("GLOWLOOM_TEST_CANVAS_OFFLINE_DB_NAME")
	if !strings.HasPrefix(name, "canvas_offline_") {
		t.Fatal("an explicitly named canvas_offline_ fixture database is required")
	}
	database := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	var actual string
	if err := database.Raw(`SELECT current_database()`).Scan(&actual).Error; err != nil || actual != name {
		t.Fatal("offline rehearsal database identity mismatch", err)
	}
	path := os.Getenv("GLOWLOOM_CANVAS_OFFLINE_MANIFEST")
	if path == "" {
		t.Fatal("an explicit synthetic fixture manifest path is required")
	}
	if phase == "seed" {
		manifest := seedOfflineUpgrade(t, database)
		manifest.Snapshot = captureOfflineData(t, database, false)
		body, err := json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write(body); err != nil {
			_ = file.Close()
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
		logOfflineSummary(t, manifest.Snapshot)
		return
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest offlineManifest
	if err = json.Unmarshal(body, &manifest); err != nil {
		t.Fatal(err)
	}
	canonicalOfflineMetadata(t, manifest.Snapshot.Heads)
	canonicalOfflineMetadata(t, manifest.Snapshot.Versions)
	converted := phase == "verify"
	current := captureOfflineData(t, database, converted)
	if !reflect.DeepEqual(manifest.Snapshot, current) {
		t.Fatal("offline conversion changed original heads, history, projection, receipts, logs or outbox")
	}
	logOfflineSummary(t, current)
	if phase == "audit-old" {
		return
	}
	if converted {
		verifyOfflineReferences(t, database, current)
	}
	verifyOfflineBehavior(t, database, manifest)
}

type offlineBinaryRow struct {
	Key      string
	Document uuid.UUID
	Metadata json.RawMessage
	State    []byte
}

type offlineTableDigest struct {
	Rows   int
	SHA256 string
}

type offlineSnapshot struct {
	Heads    []offlineBinaryRow
	Versions []offlineBinaryRow
	Tables   map[string]offlineTableDigest
}

type offlineDocument struct {
	Kind          string
	Actor         identity.Principal
	Peer          *identity.Principal
	ID            uuid.UUID
	NodeID        uuid.UUID
	State         canvas.YjsState
	Versions      []canvas.YjsVersion
	Views         []canvas.DocumentDTO
	Replay        canvas.YjsUpdate
	Receipt       canvas.YjsReceipt
	Pending       []canvas.YjsUpdate
	OldUpdate     *canvas.YjsUpdate
	OldCode       string
	Restore       *canvas.YjsRestore
	RestoreResult canvas.YjsRestoreResult
}

type offlineManifest struct {
	Documents []offlineDocument
	Snapshot  offlineSnapshot
}

func captureOfflineData(t *testing.T, database *gorm.DB, converted bool) offlineSnapshot {
	t.Helper()
	result := offlineSnapshot{Tables: map[string]offlineTableDigest{}}
	queries := []struct {
		old, upgraded string
		target        *[]offlineBinaryRow
	}{
		{`SELECT document_id::text AS key,document_id AS document,to_jsonb(d)-'state' AS metadata,state FROM canvas.collab_document d ORDER BY document_id`,
			`SELECT d.document_id::text AS key,d.document_id AS document,to_jsonb(d)-'state_id' AS metadata,c.state FROM canvas.collab_document d JOIN canvas.collab_checkpoint c ON c.document_id=d.document_id AND c.id=d.state_id ORDER BY d.document_id`, &result.Heads},
		{`SELECT id::text AS key,document_id AS document,to_jsonb(v)-'state' AS metadata,state FROM canvas.collab_version v ORDER BY id`,
			`SELECT v.id::text AS key,v.document_id AS document,to_jsonb(v)-'state_id' AS metadata,c.state FROM canvas.collab_version v JOIN canvas.collab_checkpoint c ON c.document_id=v.document_id AND c.id=v.state_id ORDER BY v.id`, &result.Versions},
	}
	for _, query := range queries {
		sql := query.old
		if converted {
			sql = query.upgraded
		}
		if err := database.Raw(sql).Scan(query.target).Error; err != nil {
			t.Fatal(err)
		}
		canonicalOfflineMetadata(t, *query.target)
	}
	for _, table := range []string{"canvas.document", "canvas.node", "canvas.edge", "canvas.node_input", "canvas.command_log", "canvas.collab_update", "canvas.collab_receipt", "canvas.collab_control_receipt", "infra.outbox"} {
		var rows []struct{ Body string }
		// Table identifiers are a fixed test-owned allowlist, never external input.
		if err := database.Raw(`SELECT to_jsonb(t)::text AS body FROM ` + table + ` t ORDER BY to_jsonb(t)::text`).Scan(&rows).Error; err != nil {
			t.Fatal(err)
		}
		hash := sha256.New()
		for _, row := range rows {
			_, _ = fmt.Fprintf(hash, "%d:%s", len(row.Body), row.Body)
		}
		result.Tables[table] = offlineTableDigest{Rows: len(rows), SHA256: hex.EncodeToString(hash.Sum(nil))}
	}
	return result
}

func canonicalOfflineMetadata(t *testing.T, rows []offlineBinaryRow) {
	t.Helper()
	for i := range rows {
		var value any
		decoder := json.NewDecoder(bytes.NewReader(rows[i].Metadata))
		decoder.UseNumber()
		if err := decoder.Decode(&value); err != nil {
			t.Fatal(err)
		}
		body, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		rows[i].Metadata = body
	}
}

func seedOfflineUpgrade(t *testing.T, database *gorm.DB) offlineManifest {
	t.Helper()
	store := canvasStore(database)
	service := canvas.NewYjsService(store, yjsRuntime(t))
	actor, project := seedCanvasActor(t, database)
	manifest := offlineManifest{}
	for _, scenario := range []struct {
		kind  string
		size  int
		modes []string
	}{
		{"ordinary", 50, []string{"text", "drag", "text"}},
		{"causal", 200, []string{"text", "causal", "causal"}},
		{"large", 2000, []string{"text", "drag"}},
		{"epochs", 2, []string{"text"}},
		{"lifecycle", 1, []string{"text"}},
	} {
		doc, err := canvas.NewService(store).Create(t.Context(), actor, project, uuid.NewString(), canvas.CreateInput{Name: "Offline " + scenario.kind})
		if err != nil {
			t.Fatal(err)
		}
		nodes := make([]domain.Node, scenario.size)
		for i := range nodes {
			nodes[i] = resourceNode("text")
			nodes[i].Config.Text = ptr("AB中文😀 " + strings.Repeat(uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprint(i))).String(), 8))
		}
		for start := 0; start < len(nodes); start += 100 {
			if _, err := canvasfixture.NewLegacySeeder(database, store).Submit(t.Context(), actor, doc.ID, uuid.NewString(), canvasfixture.EditInput{Operations: canvasfixture.Nodes(nodes[start:min(start+100, len(nodes))])}); err != nil {
				t.Fatal(err)
			}
		}
		revision := canvas.Revision(1 + (scenario.size+99)/100)
		if len(nodes) > 1 {
			edge := domain.Edge{ID: uuid.New(), EdgeType: "annotation", SourceNodeID: nodes[0].ID, TargetNodeID: nodes[1].ID}
			if _, err := canvasfixture.NewLegacySeeder(database, store).Submit(t.Context(), actor, doc.ID, uuid.NewString(), canvasfixture.EditInput{Operations: canvasfixture.Edges([]domain.Edge{edge})}); err != nil {
				t.Fatal(err)
			}
			revision++
		}
		initial, err := service.Initialize(t.Context(), actor, doc.ID, revision)
		if err != nil {
			t.Fatal(err)
		}
		item := offlineDocument{Kind: scenario.kind, Actor: actor, ID: doc.ID, NodeID: nodes[0].ID}
		updates := offlineFixtureUpdates(t, initial, item.NodeID, scenario.modes)
		for _, update := range updates {
			receipt, err := service.Submit(t.Context(), actor, doc.ID, update)
			if err != nil || receipt.Status != "accepted" {
				t.Fatalf("seed %s: %+v %v", scenario.kind, receipt, err)
			}
			item.Replay, item.Receipt = update, receipt
		}
		state, err := service.Load(t.Context(), actor, doc.ID)
		if err != nil {
			t.Fatal(err)
		}
		switch scenario.kind {
		case "epochs":
			versions, err := service.Versions(t.Context(), actor, doc.ID, 0)
			if err != nil {
				t.Fatal(err)
			}
			old := yjsEdit(t, state, item.NodeID, "旧 epoch", false)
			item.OldUpdate, item.OldCode = &old, "EPOCH_CHANGED"
			command := canvas.YjsRestore{ID: uuid.New(), SessionID: uuid.New(), VersionID: versions[len(versions)-1].ID, Epoch: state.Epoch, CommitSeq: state.CommitSeq}
			item.Restore = &command
			item.RestoreResult, err = service.RestoreVersion(t.Context(), actor, doc.ID, command)
			if err != nil {
				t.Fatal(err)
			}
		case "lifecycle":
			old := yjsEdit(t, state, item.NodeID, "旧生命周期", false)
			item.OldUpdate, item.OldCode = &old, "TARGET_DELETED"
			body, err := json.Marshal(state)
			if err != nil {
				t.Fatal(err)
			}
			output := offlineNodeFixture(t, "service-lifecycle-fixture.mjs", body)
			var life struct{ Deletion, Restoration canvas.YjsUpdate }
			if err := json.Unmarshal(output, &life); err != nil {
				t.Fatal(err)
			}
			for _, update := range []*canvas.YjsUpdate{&life.Deletion, &life.Restoration} {
				update.Epoch, update.SessionID, update.UpdateID = state.Epoch, canvasfixture.ClientID, uuid.New()
			}
			item.Receipt, err = service.Submit(t.Context(), actor, doc.ID, life.Deletion)
			if err != nil || item.Receipt.Status != "accepted" {
				t.Fatalf("seed deletion: %+v %v", item.Receipt, err)
			}
			item.Replay, item.Pending = life.Deletion, []canvas.YjsUpdate{life.Restoration}
		}
		finishOfflineDocument(t, service, &item)
		manifest.Documents = append(manifest.Documents, item)
	}
	first, second, _, node := newTeamLiveFixtures(t)
	team := offlineDocument{Kind: "team", Actor: first.actor, Peer: &second.actor, ID: first.doc.ID, NodeID: node}
	initial, err := service.Initialize(t.Context(), team.Actor, team.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	team.Replay = yjsEdit(t, initial, node, "转换前甲", false)
	team.Receipt, err = service.Submit(t.Context(), team.Actor, team.ID, team.Replay)
	if err != nil || team.Receipt.Status != "accepted" {
		t.Fatal(team.Receipt, err)
	}
	finishOfflineDocument(t, service, &team)
	team.Pending = append(team.Pending, yjsEdit(t, team.State, node, "转换后乙", false))
	manifest.Documents = append(manifest.Documents, team)
	return manifest
}

func finishOfflineDocument(t *testing.T, service *canvas.YjsService, item *offlineDocument) {
	t.Helper()
	var err error
	item.State, err = service.Load(t.Context(), item.Actor, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	item.Versions, err = service.Versions(t.Context(), item.Actor, item.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range item.Versions {
		view, err := service.Version(t.Context(), item.Actor, item.ID, version.ID)
		if err != nil {
			t.Fatal(err)
		}
		item.Views = append(item.Views, view)
	}
	if len(item.Pending) == 0 {
		item.Pending = []canvas.YjsUpdate{yjsEdit(t, item.State, item.NodeID, "跨转换继续", false)}
	}
}

func offlineNodeFixture(t *testing.T, fixture string, body []byte) []byte {
	t.Helper()
	command := exec.CommandContext(t.Context(), "node", "../../../canvas/tests/"+fixture)
	command.Stdin = bytes.NewReader(body)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("offline client fixture: %v %s", err, output)
	}
	return output
}

func offlineFixtureUpdates(t *testing.T, state canvas.YjsState, node uuid.UUID, modes []string) []canvas.YjsUpdate {
	t.Helper()
	body, err := json.Marshal(map[string]any{"state": state.State, "nodeId": node, "modes": modes})
	if err != nil {
		t.Fatal(err)
	}
	var updates []canvas.YjsUpdate
	if err := json.Unmarshal(offlineNodeFixture(t, "sync-performance-fixture.mjs", body), &updates); err != nil {
		t.Fatal(err)
	}
	if len(updates) != len(modes) {
		t.Fatal("offline client update count mismatch")
	}
	for i := range updates {
		updates[i].Epoch, updates[i].SessionID, updates[i].UpdateID = state.Epoch, canvasfixture.ClientID, uuid.New()
	}
	return updates
}

func verifyOfflineReferences(t *testing.T, database *gorm.DB, snapshot offlineSnapshot) {
	t.Helper()
	unique := map[string]bool{}
	for _, row := range append(append([]offlineBinaryRow{}, snapshot.Heads...), snapshot.Versions...) {
		unique[row.Document.String()+"/"+string(row.State)] = true
	}
	var counts struct{ Checkpoints, Dangling int }
	if err := database.Raw(`SELECT (SELECT count(*) FROM canvas.collab_checkpoint) checkpoints,
(SELECT count(*) FROM canvas.collab_checkpoint c WHERE NOT EXISTS(SELECT 1 FROM canvas.collab_document d WHERE d.document_id=c.document_id AND d.state_id=c.id) AND NOT EXISTS(SELECT 1 FROM canvas.collab_version v WHERE v.document_id=c.document_id AND v.state_id=c.id)) dangling`).Scan(&counts).Error; err != nil {
		t.Fatal(err)
	}
	if counts.Checkpoints != len(unique) || counts.Dangling != 0 {
		t.Fatalf("incorrect exact-state sharing: checkpoints=%d unique=%d dangling=%d", counts.Checkpoints, len(unique), counts.Dangling)
	}
	t.Logf("OFFLINE_REFERENCES checkpoints=%d exact_unique_states=%d dangling=%d", counts.Checkpoints, len(unique), counts.Dangling)
}

func verifyOfflineBehavior(t *testing.T, database *gorm.DB, manifest offlineManifest) {
	t.Helper()
	service := canvas.NewYjsService(canvasStore(database), yjsRuntime(t))
	for _, item := range manifest.Documents {
		t.Run(item.Kind, func(t *testing.T) {
			loaded, err := service.Load(t.Context(), item.Actor, item.ID)
			if err != nil || loaded.YjsBinding != item.State.YjsBinding || !bytes.Equal(loaded.State, item.State.State) || loaded.GraphHash != item.State.GraphHash || !offlineJSONEqual(loaded.Document, item.State.Document) {
				t.Fatal("converted head differs from original load", err)
			}
			versions, err := service.Versions(t.Context(), item.Actor, item.ID, 0)
			if err != nil || !offlineJSONEqual(versions, item.Versions) {
				t.Fatal("historical list changed", err)
			}
			for i, version := range versions {
				view, err := service.Version(t.Context(), item.Actor, item.ID, version.ID)
				if err != nil || !offlineJSONEqual(view, item.Views[i]) {
					t.Fatal("historical view changed", err)
				}
			}
			replay, err := service.Submit(t.Context(), item.Actor, item.ID, item.Replay)
			if err != nil || !reflect.DeepEqual(replay, item.Receipt) {
				t.Fatal("original receipt replay changed", err)
			}
			if item.Restore != nil {
				restored, err := service.RestoreVersion(t.Context(), item.Actor, item.ID, *item.Restore)
				if err != nil || restored != item.RestoreResult {
					t.Fatal("original restore receipt changed", err)
				}
			}
			if item.Peer == nil {
				receipt, err := service.Submit(t.Context(), item.Actor, item.ID, item.Pending[0])
				if err != nil || receipt.Status != "accepted" || receipt.CommitSeq != item.State.CommitSeq+1 {
					t.Fatalf("pre-conversion pending update: %+v %v", receipt, err)
				}
			} else {
				results := make(chan error, 2)
				for i, actor := range []identity.Principal{item.Actor, *item.Peer} {
					go func() {
						receipt, err := service.Submit(t.Context(), actor, item.ID, item.Pending[i])
						if err == nil && receipt.Status != "accepted" {
							err = fmt.Errorf("team pending rejected: %s", receipt.Code)
						}
						results <- err
					}()
				}
				var failures []error
				for range 2 {
					if err := <-results; err != nil {
						failures = append(failures, err)
					}
				}
				if len(failures) > 0 {
					t.Fatal(failures)
				}
				current, err := service.Load(t.Context(), *item.Peer, item.ID)
				body, marshalErr := json.Marshal(current.Document)
				if err != nil || marshalErr != nil || current.CommitSeq != item.State.CommitSeq+2 || !strings.Contains(string(body), "跨转换继续") || !strings.Contains(string(body), "转换后乙") {
					t.Fatal("two-actor concurrent edits lost content", err, marshalErr)
				}
			}
			if item.OldUpdate != nil {
				rejected, err := service.Submit(t.Context(), item.Actor, item.ID, *item.OldUpdate)
				if err != nil || rejected.Status != "rejected" || rejected.Code != item.OldCode {
					t.Fatalf("old state fence changed: %+v %v", rejected, err)
				}
			}
			current, err := service.Load(t.Context(), item.Actor, item.ID)
			if err != nil {
				t.Fatal(err)
			}
			if item.Kind != "lifecycle" {
				body, err := json.Marshal(current.Document)
				if err != nil || !strings.Contains(string(body), "跨转换继续") {
					t.Fatal("accepted pending update did not persist its text", err)
				}
			}
			if item.Kind == "lifecycle" && (len(current.Document.Nodes) != 1 || current.Document.Nodes[0].ID != item.NodeID) {
				t.Fatal("pending deletion undo changed stable identity")
			}
			// Restore a genuinely pre-conversion version using its original ID.
			command := canvas.YjsRestore{ID: uuid.New(), SessionID: uuid.New(), VersionID: item.Versions[len(item.Versions)-1].ID, Epoch: current.Epoch, CommitSeq: current.CommitSeq}
			restored, err := service.RestoreVersion(t.Context(), item.Actor, item.ID, command)
			if err != nil || restored.Epoch == current.Epoch || restored.Revision <= current.Document.Revision {
				t.Fatal("pre-conversion history cannot restore into a new epoch", err)
			}
			restoredAgain, err := service.RestoreVersion(t.Context(), item.Actor, item.ID, command)
			if err != nil || restoredAgain != restored {
				t.Fatal("new restore lost idempotency", err)
			}
			// A fresh runtime instance must reopen only the durable restored state.
			reopened, err := canvas.NewYjsService(canvasStore(database), yjsRuntime(t)).Load(t.Context(), item.Actor, item.ID)
			if err != nil || reopened.Epoch != restored.Epoch || reopened.Document.Revision != restored.Revision || len(reopened.Document.Nodes) != len(item.Views[len(item.Views)-1].Nodes) {
				t.Fatal("restarted runtime lost restored content", err)
			}
			assertOfflineRestoredGraph(t, reopened.Document, item.Views[len(item.Views)-1], current.Document)
		})
	}
}

func assertOfflineRestoredGraph(t *testing.T, current, historical, beforeRestore canvas.DocumentDTO) {
	t.Helper()
	// Restoration advances content/input versions and keeps the current document
	// identity/name. Compare every editable node field and edge independently.
	normalize := func(nodes []canvas.NodeDTO) []canvas.NodeDTO {
		result := slices.Clone(nodes)
		for i := range result {
			result[i].ContentVersion, result[i].InputRevision, result[i].LastOperationID = 0, 0, nil
		}
		slices.SortFunc(result, func(a, b canvas.NodeDTO) int { return strings.Compare(a.ID.String(), b.ID.String()) })
		return result
	}
	// The existing restore contract retains reference-label allocations accepted
	// after the historical version. Verify their exact pre-restore values rather
	// than dropping editor metadata from the comparison.
	body, err := json.Marshal(historical)
	if err != nil {
		t.Fatal(err)
	}
	var expected canvas.DocumentDTO
	if err := json.Unmarshal(body, &expected); err != nil {
		t.Fatal(err)
	}
	previous := map[uuid.UUID]canvas.NodeDTO{}
	for _, node := range beforeRestore.Nodes {
		previous[node.ID] = node
	}
	for i := range expected.Nodes {
		old := previous[expected.Nodes[i].ID]
		if old.Config.Editor == nil || len(old.Config.Editor.Metadata["referenceLabels"]) == 0 {
			continue
		}
		if expected.Nodes[i].Config.Editor == nil {
			expected.Nodes[i].Config.Editor = &domain.EditorConfig{SchemaVersion: 1}
		}
		if expected.Nodes[i].Config.Editor.Metadata == nil {
			expected.Nodes[i].Config.Editor.Metadata = map[string]json.RawMessage{}
		}
		expected.Nodes[i].Config.Editor.Metadata["referenceLabels"] = old.Config.Editor.Metadata["referenceLabels"]
	}
	if !offlineJSONEqual(normalize(current.Nodes), normalize(expected.Nodes)) || !offlineJSONEqual(current.Edges, expected.Edges) {
		t.Fatal("restored graph differs in text, config, identities, placement, labels or edges")
	}
}

func offlineJSONEqual(left, right any) bool {
	a, aErr := json.Marshal(left)
	b, bErr := json.Marshal(right)
	return aErr == nil && bErr == nil && bytes.Equal(a, b)
}

func logOfflineSummary(t *testing.T, snapshot offlineSnapshot) {
	t.Helper()
	var binaryBytes int
	for _, row := range snapshot.Heads {
		binaryBytes += len(row.State)
	}
	for _, row := range snapshot.Versions {
		binaryBytes += len(row.State)
	}
	t.Logf("OFFLINE_SNAPSHOT heads=%d versions=%d binary_bytes=%d tables=%v", len(snapshot.Heads), len(snapshot.Versions), binaryBytes, snapshot.Tables)
}
