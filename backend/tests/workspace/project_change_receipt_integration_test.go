package workspace_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
)

func TestProjectCoverPermanentExpiredLegacyReceiptPromotion(t *testing.T) {
	ctx, db, owner := coverTestDB(t)
	actor, project := lifecycleActorProject(ctx, t, owner)
	s := workspaceapp.NewProjectLifecycle(coverStore(db), time.Now)
	read, err := s.Get(ctx, actor, project)
	if err != nil {
		t.Fatal(err)
	}
	name := read.Project.Name
	in := projectChange(project, 1, "patch")
	in.Patch.Name = &name
	legacy, _ := json.Marshal(struct {
		Contract          string
		Org, Project      uuid.UUID
		Action            string
		Revision          int64
		Name, Description *string
		Preset            *uuid.UUID
		Overseas          *bool
	}{"project.lifecycle.v1", actor.OrgID, project, "patch", 1, &name, nil, nil, nil})
	hash := sha256.Sum256(legacy)
	raw, _ := json.Marshal(read)
	lifecycleFixtureSQL(t, owner, `INSERT INTO infra.idempotency_record(id,actor_id,idem_key,request_hash,status_code,response_body,expires_at)VALUES(?,?,?,?,200,?::jsonb,clock_timestamp()-interval '30 days')`, uuid.New(), actor.ID, in.IdempotencyKey.String(), hex.EncodeToString(hash[:]), string(raw))
	var oldBytes string
	if err := db.Raw(`SELECT response_body::text FROM infra.idempotency_record WHERE actor_id=? AND idem_key=?`, actor.ID, in.IdempotencyKey.String()).Scan(&oldBytes).Error; err != nil {
		t.Fatal(err)
	}
	lifecycleFixtureSQL(t, owner, `UPDATE workspace.project SET name='later revision',revision=2 WHERE id=?`, project)
	replayed, err := s.Change(ctx, actor, in)
	if err != nil || replayed.Project.Revision != 1 || replayed.Project.Name != name {
		t.Fatal("retained expired receipt lost original result", replayed, err)
	}
	var receipt struct {
		Raw   []byte
		Hash  string
		Count int64
	}
	if err := db.Raw(`SELECT response_body::text AS raw,request_sha256 AS hash,(SELECT count(*) FROM infra.idempotency_record WHERE actor_id=? AND idem_key=?) AS count FROM workspace.project_change_command WHERE actor_id=? AND idem_key=?`, actor.ID, in.IdempotencyKey.String(), actor.ID, in.IdempotencyKey).Scan(&receipt).Error; err != nil || !bytes.Equal([]byte(oldBytes), receipt.Raw) || receipt.Hash != hex.EncodeToString(hash[:]) || receipt.Count != 1 {
		t.Fatal("legacy promotion rewrote/deleted original bytes", receipt, err)
	}
	newName := "different request"
	in.Patch.Name = &newName
	if _, err := s.Change(ctx, actor, in); !errors.Is(err, workspaceapp.ErrIdempotencyConflict) {
		t.Fatal("permanent key reused", err)
	}
	for _, mutation := range []string{`UPDATE workspace.project_change_command SET request_sha256=repeat('f',64) WHERE actor_id=? AND idem_key=?`, `DELETE FROM workspace.project_change_command WHERE actor_id=? AND idem_key=?`} {
		if err := db.Exec(mutation, actor.ID, in.IdempotencyKey).Error; err == nil {
			t.Fatal("runtime mutated permanent project command")
		}
	}
}

func TestProjectCoverPermanentZeroRowReceiptRollsBack(t *testing.T) {
	ctx, db, owner := coverTestDB(t)
	actor, project := lifecycleActorProject(ctx, t, owner)
	asset := seedCoverAsset(t, owner, project, "image")
	name := "cover_zero_" + uuid.New().String()[:8]
	lifecycleFixtureSQL(t, owner, fmt.Sprintf(`CREATE FUNCTION workspace.%s() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN IF NEW.actor_id='%s'::uuid THEN RETURN NULL;END IF;RETURN NEW;END;$$;CREATE TRIGGER %s BEFORE INSERT ON workspace.project_change_command FOR EACH ROW EXECUTE FUNCTION workspace.%s()`, name, actor.ID.String(), name, name))
	defer func() {
		lifecycleFixtureSQL(t, owner, fmt.Sprintf(`DROP TRIGGER %s ON workspace.project_change_command;DROP FUNCTION workspace.%s()`, name, name))
	}()
	if _, err := workspaceapp.NewProjectLifecycle(coverStore(db), time.Now).Change(ctx, actor, coverChange(project, 1, &asset)); err == nil {
		t.Fatal("zero row receipt accepted")
	}
	var row struct {
		Revision, Events, Receipts int64
		Cover                      *uuid.UUID
	}
	if err := db.Raw(`SELECT p.revision,p.cover_asset_id AS cover,(SELECT count(*) FROM infra.outbox WHERE partition_key=p.id::text)AS events,(SELECT count(*) FROM workspace.project_change_command WHERE project_id=p.id)AS receipts FROM workspace.project p WHERE id=?`, project).Scan(&row).Error; err != nil || row.Revision != 1 || row.Cover != nil || row.Events != 0 || row.Receipts != 0 {
		t.Fatal("zero row produced partial commit", row, err)
	}
}

func TestProjectCoverPermanentLegacyClosedPayloadRejectsWithoutPromotion(t *testing.T) {
	ctx, db, owner := coverTestDB(t)
	actor, project := lifecycleActorProject(ctx, t, owner)
	s := workspaceapp.NewProjectLifecycle(coverStore(db), time.Now)
	read, err := s.Get(ctx, actor, project)
	if err != nil {
		t.Fatal(err)
	}
	name := read.Project.Name
	in := projectChange(project, 1, "patch")
	in.Patch.Name = &name
	legacy, _ := json.Marshal(struct {
		Contract          string
		Org, Project      uuid.UUID
		Action            string
		Revision          int64
		Name, Description *string
		Preset            *uuid.UUID
		Overseas          *bool
	}{"project.lifecycle.v1", actor.OrgID, project, "patch", 1, &name, nil, nil, nil})
	hash := sha256.Sum256(legacy)
	for _, change := range []func(map[string]any){
		func(m map[string]any) { m["url"] = "private" },
		func(m map[string]any) { m["Project"].(map[string]any)["object_key"] = "private" },
		func(m map[string]any) { m["Project"].(map[string]any)["OrgID"] = uuid.NewString() },
		func(m map[string]any) { m["Project"].(map[string]any)["ID"] = uuid.NewString() },
		func(m map[string]any) { m["Project"].(map[string]any)["Name"] = "wrong result" },
		func(m map[string]any) { m["DefaultModels"] = nil },
		func(m map[string]any) { m["Project"].(map[string]any)["Status"] = "copying" },
		func(m map[string]any) { m["Project"].(map[string]any)["Revision"] = 99 },
	} {
		in.IdempotencyKey = uuid.New()
		encoded, _ := json.Marshal(read)
		var value map[string]any
		if err := json.Unmarshal(encoded, &value); err != nil {
			t.Fatal(err)
		}
		change(value)
		encoded, _ = json.Marshal(value)
		lifecycleFixtureSQL(t, owner, `INSERT INTO infra.idempotency_record(id,actor_id,idem_key,request_hash,status_code,response_body,expires_at)VALUES(?,?,?,?,200,?::jsonb,clock_timestamp()-interval '1 day')`, uuid.New(), actor.ID, in.IdempotencyKey.String(), hex.EncodeToString(hash[:]), string(encoded))
		if _, err := s.Change(ctx, actor, in); err == nil {
			t.Fatal("unproved legacy response accepted")
		}
		var count int64
		if err := db.Raw(`SELECT count(*) FROM workspace.project_change_command WHERE actor_id=? AND idem_key=?`, actor.ID, in.IdempotencyKey).Scan(&count).Error; err != nil || count != 0 {
			t.Fatal("invalid legacy payload promoted", err, count)
		}
	}
}

func TestProjectCoverPermanentHTTPKeyMismatchIsConflictAndMigrationRetention(t *testing.T) {
	ctx, db, owner := coverTestDB(t)
	actor, project := lifecycleActorProject(ctx, t, owner)
	path := "/api/projects/" + project.String()
	key := uuid.NewString()
	first := projectHTTPRequest(ctx, coverRouter(db, actor), "PATCH", path, key, []byte(`{"expected_revision":1,"name":"first permanent result"}`))
	if first.Code != 200 {
		t.Fatal(first.Code, first.Body.String())
	}
	conflict := projectHTTPRequest(ctx, coverRouter(db, actor), "PATCH", path, key, []byte(`{"expected_revision":1,"name":"other input"}`))
	if conflict.Code != 409 {
		t.Fatal("permanent HTTP key reuse must conflict", conflict.Code, conflict.Body.String())
	}
	down, err := os.ReadFile("../../db/migrations/202610020052_workspace_project_cover.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	tx := owner.Begin()
	defer rollbackLifecycleTest(t, tx)
	if err := tx.Exec(string(down)).Error; err == nil {
		t.Fatal("migration erased retained permanent receipts")
	}
	if err := tx.Rollback().Error; err != nil {
		t.Fatal(err)
	}
	var exists bool
	if err := db.Raw(`SELECT EXISTS(SELECT 1 FROM workspace.project_change_command WHERE actor_id=? AND idem_key=?)`, actor.ID, uuid.MustParse(key)).Scan(&exists).Error; err != nil || !exists {
		t.Fatal("rollback guard did not retain receipt", err)
	}
}

func TestProjectCoverPermanentActualOldProductionFingerprintGolden(t *testing.T) {
	ctx, _, owner := coverTestDB(t)
	raw, err := os.ReadFile("testdata/project_change_legacy_golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var golden struct {
		InputBytes    string `json:"input_bytes"`
		ResponseBytes string `json:"response_bytes"`
		Fingerprint   string `json:"fingerprint_sha256"`
	}
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatal(err)
	}
	var input workspaceapp.ProjectChangeInput
	var result workspaceapp.ProjectSnapshot
	if err := json.Unmarshal([]byte(golden.InputBytes), &input); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(golden.ResponseBytes), &result); err != nil {
		t.Fatal(err)
	}
	actor := identityapp.Principal{ID: uuid.MustParse("00000000-0000-4000-8000-000000000006"), OrgID: result.Project.OrgID, Role: "producer"}
	tx := owner.Begin()
	defer rollbackLifecycleTest(t, tx)
	lifecycleFixtureSQL(t, tx, `INSERT INTO workspace.organization(id,name)VALUES(?,'fixed old golden')`, actor.OrgID)
	lifecycleFixtureSQL(t, tx, `INSERT INTO identity."user"(id,org_id,login_name,display_name,password_hash,role,must_change_password)VALUES(?,?,'cover-golden@example.test','Cover Golden','synthetic-never-authenticates','producer',false)`, actor.ID, actor.OrgID)
	lifecycleFixtureSQL(t, tx, `INSERT INTO workspace.project(id,org_id,name,aspect_ratio,style_type,resolution,revision)VALUES(?,?,'later content','16:9','realistic','1080p',9)`, input.Patch.ProjectID, actor.OrgID)
	lifecycleFixtureSQL(t, tx, `INSERT INTO infra.idempotency_record(id,actor_id,idem_key,request_hash,status_code,response_body,expires_at)VALUES(?,?,?,?,200,?::jsonb,clock_timestamp()-interval '1 year')`, uuid.New(), actor.ID, input.IdempotencyKey.String(), golden.Fingerprint, golden.ResponseBytes)
	lifecycleFixtureSQL(t, tx, `SET LOCAL ROLE lanverse_app`)
	replayed, err := workspaceapp.NewProjectLifecycle(coverStore(tx), time.Now).Change(ctx, actor, input)
	encoded, _ := json.Marshal(replayed)
	if err != nil || !bytes.Equal(encoded, []byte(golden.ResponseBytes)) {
		t.Fatal("fixed29ce actual fingerprint/response incompatible", err, string(encoded))
	}
	var hash string
	if err := tx.Raw(`SELECT request_sha256 FROM workspace.project_change_command WHERE actor_id=? AND idem_key=?`, actor.ID, input.IdempotencyKey).Scan(&hash).Error; err != nil || hash != golden.Fingerprint {
		t.Fatal("old production hash changed", hash, err)
	}
}
