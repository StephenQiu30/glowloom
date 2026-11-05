package script_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/script/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/domain"
	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

func scriptAdmin(t *testing.T, owner *gorm.DB, org uuid.UUID) identityapp.Principal {
	t.Helper()
	actor := identityapp.Principal{ID: uuid.New(), OrgID: org, Role: identitydomain.RoleAdmin}
	if err := owner.Exec(`INSERT INTO identity."user"(id,org_id,login_name,display_name,password_hash,role,status,must_change_password) VALUES(?,?,?,?,?,'admin','active',false)`, actor.ID, org, "script-admin-"+actor.ID.String(), "测试管理员", "test-only-not-a-credential").Error; err != nil {
		t.Fatal(err)
	}
	return scriptFixtureTeamAdmin(t, owner, actor)
}

func TestScriptSourceRecoveryPGPrivateCancelRevokedCreatorAndImmutableControl(t *testing.T) {
	db, owner := scriptTestDB(t)
	creator, pid := scriptActorProject(t, owner)
	admin := scriptAdmin(t, owner, creator.OrgID)
	storage := scriptPrivateObjects(t, db)
	loss := &lostPrivatePut{PrivateObjects: storage}
	store := scriptStore(db)
	sources := app.NewSourceService(store, loss, time.Now)
	recovery := app.NewSourceRecovery(store, sources, time.Now)
	_, input := sourceCommand()
	input.ProjectID = pid
	if _, err := sources.Write(t.Context(), creator, input); !errors.Is(err, app.ErrNeedsReconciliation) {
		t.Fatal(err)
	}
	page, err := recovery.List(t.Context(), admin, pid, 0, 100)
	if err != nil || len(page.Items) != 1 || page.CurrentActorID != admin.ID {
		t.Fatal("safe recovery list", page, err)
	}
	intent := page.Items[0]
	if err := owner.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, creator.ID).Error; err != nil {
		t.Fatal(err)
	}
	command := app.SourceControlCommand{ProjectID: pid, IntentID: intent.ID, Key: uuid.New(), RequestID: uuid.New(), Action: "cancel", ExpectedRevision: intent.Revision}
	accepted, err := recovery.Control(t.Context(), admin, command)
	if err != nil {
		t.Fatal("current admin cancel", err)
	}
	current, err := recovery.Get(t.Context(), admin, pid, intent.ID)
	if err != nil || current.Status != "pending" || !current.NeedsReconciliation || current.ActiveIO {
		t.Fatal("cancel must retain the remote-write drain fence", current, err)
	}
	plan := pendingScriptPlan(t, owner, pid)
	assertScriptFixtureObjectPresent(t, storage, plan.Objects[0])
	expireScriptFixtureSourceDrain(t, owner, plan)
	reconcile := command
	reconcile.Key = uuid.New()
	reconcile.RequestID = uuid.New()
	reconcile.Action = "reconcile"
	reconcile.ExpectedRevision = current.Revision
	if _, err := recovery.Control(t.Context(), admin, reconcile); err != nil {
		t.Fatal(err)
	}
	current, err = recovery.Get(t.Context(), admin, pid, intent.ID)
	if err != nil || current.Status != "cancelled" || current.NeedsReconciliation {
		t.Fatal("actual private cleanup", current, err)
	}
	replay, err := recovery.Control(t.Context(), admin, command)
	if err != nil || replay != accepted {
		t.Fatal("original permanent control response", replay, accepted, err)
	}
	if s, v, c := scriptCounts(t, owner, pid); s != 0 || v != 0 || c != 1 {
		t.Fatal("control published old source", s, v, c)
	}
	if blocked, err := store.HasInflightWork(t.Context(), admin, pid); err != nil || blocked {
		t.Fatal("stopped cleanup fence", blocked, err)
	}
	if _, err := sources.Write(t.Context(), creator, input); !errors.Is(err, identityapp.ErrSessionInvalid) {
		t.Fatal("borrowed admin to publish", err)
	}
	command.Key = uuid.New()
	command.ExpectedRevision = current.Revision
	if _, err := recovery.Control(t.Context(), admin, command); !errors.Is(err, app.ErrConflict) {
		t.Fatal("terminal cancellation changed", err)
	}
}

type pausedScriptPut struct {
	app.PrivateObjects
	started chan struct{}
	exited  chan struct{}
}

func (p *pausedScriptPut) PutObjectIfAbsent(ctx context.Context, fact domain.ObjectFact, r io.Reader) error {
	if err := p.PrivateObjects.PutObjectIfAbsent(ctx, fact, r); err != nil {
		return err
	}
	close(p.started)
	<-ctx.Done()
	close(p.exited)
	return ctx.Err()
}
func TestScriptSourceRecoveryPGActivePrivateIOCancelJoinsBeforeCleanup(t *testing.T) {
	db, owner := scriptTestDB(t)
	creator, pid := scriptActorProject(t, owner)
	admin := scriptAdmin(t, owner, creator.OrgID)
	paused := &pausedScriptPut{PrivateObjects: scriptPrivateObjects(t, db), started: make(chan struct{}), exited: make(chan struct{})}
	store := scriptStore(db)
	sources := app.NewSourceService(store, paused, time.Now)
	recovery := app.NewSourceRecovery(store, sources, time.Now)
	_, input := sourceCommand()
	input.ProjectID = pid
	done := make(chan error, 1)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	go func() { _, err := sources.Write(ctx, creator, input); done <- err }()
	select {
	case <-paused.started:
	case <-ctx.Done():
		t.Fatal("private IO never started")
	}
	page, err := recovery.List(t.Context(), admin, pid, 0, 100)
	if err != nil || len(page.Items) != 1 {
		t.Fatal(page, err)
	}
	command := app.SourceControlCommand{ProjectID: pid, IntentID: page.Items[0].ID, Key: uuid.New(), RequestID: uuid.New(), Action: "cancel", ExpectedRevision: page.Items[0].Revision}
	if _, err := recovery.Control(ctx, admin, command); err != nil {
		t.Fatal(err)
	}
	select {
	case <-paused.exited:
	default:
		t.Fatal("cancelled before actual private IO exit")
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancel raced to publish")
		}
	case <-ctx.Done():
		t.Fatal("source routine leaked")
	}
	current, err := recovery.Get(ctx, admin, pid, page.Items[0].ID)
	if err != nil || current.Status != "pending" || !current.NeedsReconciliation || current.ActiveIO {
		t.Fatal("joined SDK still needs remote-write drain", current, err)
	}
	plan := pendingScriptPlan(t, owner, pid)
	assertScriptFixtureObjectPresent(t, paused, plan.Objects[0])
	expireScriptFixtureSourceDrain(t, owner, plan)
	reconcile := command
	reconcile.Key = uuid.New()
	reconcile.RequestID = uuid.New()
	reconcile.Action = "reconcile"
	reconcile.ExpectedRevision = current.Revision
	if _, err := recovery.Control(t.Context(), admin, reconcile); err != nil {
		t.Fatal(err)
	}
	current, err = recovery.Get(t.Context(), admin, pid, page.Items[0].ID)
	if err != nil || current.Status != "cancelled" {
		t.Fatal("joined cleanup", current, err)
	}
	if s, v, _ := scriptCounts(t, owner, pid); s != 0 || v != 0 {
		t.Fatal("parallel finalize survived cancel")
	}
}

type neverScriptPut struct{ app.PrivateObjects }

func (p neverScriptPut) PutObjectIfAbsent(context.Context, domain.ObjectFact, io.Reader) error {
	return errors.New("synthetic private put has an unknown remote outcome")
}

func pendingScriptPlan(t *testing.T, owner *gorm.DB, pid uuid.UUID) app.WritePlan {
	t.Helper()
	var row struct{ Plan string }
	if err := owner.Raw(`SELECT plan::text FROM script.command WHERE project_id=?`, pid).Scan(&row).Error; err != nil {
		t.Fatal(err)
	}
	var plan app.WritePlan
	if err := json.Unmarshal([]byte(row.Plan), &plan); err != nil {
		t.Fatal("decode synthetic script plan")
	}
	// Semantic command JSON intentionally excludes physical IDs. The fixture
	// reads the exact command's own object ledger, as the production store does.
	var rows []struct {
		ObjectKey        string
		ObjectLocationID uuid.UUID
	}
	if err := owner.Raw(`SELECT object_key,object_location_id FROM script.object_intent WHERE actor_id=? AND request_id=?`, plan.ActorID, plan.Command.Key).Scan(&rows).Error; err != nil {
		t.Fatal(err)
	}
	byKey := make(map[string]uuid.UUID, len(rows))
	for _, row := range rows {
		byKey[row.ObjectKey] = row.ObjectLocationID
	}
	for i := range plan.Objects {
		plan.Objects[i].LocationID = byKey[plan.Objects[i].Key]
		if plan.Objects[i].LocationID == uuid.Nil {
			t.Fatal("command object lacks its admitted physical location")
		}
	}
	for i := range plan.NewSources {
		plan.NewSources[i].Original.LocationID = byKey[plan.NewSources[i].Original.Key]
		plan.NewSources[i].Rich.LocationID = byKey[plan.NewSources[i].Rich.Key]
	}
	plan.Version.Text.LocationID = byKey[plan.Version.Text.Key]
	plan.Version.Rich.LocationID = byKey[plan.Version.Rich.Key]
	return plan
}
func cleanScriptPlan(t *testing.T, db *gorm.DB, _ app.PrivateObjects, plan app.WritePlan) {
	t.Helper()
	t.Cleanup(func() {
		for _, f := range plan.Objects {
			if err := removeScriptFixtureBytes(t, db, f); err != nil {
				t.Error("clean exact synthetic script object")
			}
		}
	})
}
func TestScriptSourceRecoveryPGUnknownIOObjectsAndCurrentControlPermissionFence(t *testing.T) {
	for _, scenario := range []string{"unknown_put_absent", "process_lost", "foreign_digest", "disabled_controller", "spoof_admin_role"} {
		t.Run(scenario, func(t *testing.T) {
			db, owner := scriptTestDB(t)
			creator, pid := scriptActorProject(t, owner)
			admin := scriptAdmin(t, owner, creator.OrgID)
			privateObjects := scriptPrivateObjects(t, db)
			var objects app.PrivateObjects = &lostPrivatePut{PrivateObjects: privateObjects}
			if scenario == "unknown_put_absent" {
				objects = neverScriptPut{PrivateObjects: privateObjects}
			}
			store := scriptStore(db)
			sources := app.NewSourceService(store, objects, time.Now)
			recovery := app.NewSourceRecovery(store, sources, time.Now)
			_, input := sourceCommand()
			input.ProjectID = pid
			if _, err := sources.Write(t.Context(), creator, input); !errors.Is(err, app.ErrNeedsReconciliation) {
				t.Fatal(err)
			}
			plan := pendingScriptPlan(t, owner, pid)
			cleanScriptPlan(t, db, privateObjects, plan)
			if scenario == "process_lost" {
				if err := owner.Exec(`UPDATE script.command_state SET io_state='running',io_owner_id=? WHERE actor_id=? AND request_id=?`, uuid.New(), creator.ID, input.Key).Error; err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "foreign_digest" {
				f := plan.Objects[0]
				if err := removeScriptFixtureBytes(t, db, f); err != nil {
					t.Fatal(err)
				}
				bad := []byte("foreign bytes")
				if err := putScriptFixtureBytes(t, db, f, bytes.NewReader(bad), int64(len(bad)), f.MIME, domain.ContentSHA(bad)); err != nil {
					t.Fatal(err)
				}
			}
			page, err := recovery.List(t.Context(), admin, pid, 0, 100)
			if err != nil {
				t.Fatal(err)
			}
			before := page.Items[0]
			controller := admin
			if scenario == "disabled_controller" {
				if err := owner.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, admin.ID).Error; err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "spoof_admin_role" {
				controller = scriptFixtureNonmember(t, owner, creator)
				controller.Role = identitydomain.RoleAdmin
			}
			command := app.SourceControlCommand{ProjectID: pid, IntentID: before.ID, Key: uuid.New(), RequestID: uuid.New(), Action: "cancel", ExpectedRevision: before.Revision}
			_, err = recovery.Control(t.Context(), controller, command)
			if scenario == "disabled_controller" || scenario == "spoof_admin_role" {
				expected := identityapp.ErrSessionInvalid
				if scenario == "spoof_admin_role" {
					expected = workspaceapp.ErrScopeNotFound
				}
				if !errors.Is(err, expected) {
					t.Fatal("non-current authority accepted", err)
				}
				var controls int64
				if err := owner.Raw(`SELECT count(*) FROM script.source_control WHERE project_id=?`, pid).Scan(&controls).Error; err != nil || controls != 0 {
					t.Fatal("forbidden control wrote", controls, err)
				}
			} else if err != nil {
				t.Fatal("durable acceptance", err)
			}
			current, err := recovery.Get(t.Context(), creator, pid, before.ID)
			if err != nil || current.Status != "pending" {
				t.Fatal("unknown physical proof released", current, err)
			}
			if scenario != "disabled_controller" && scenario != "spoof_admin_role" && !current.NeedsReconciliation {
				t.Fatal("unknown fence not visible")
			}
			if blocked, err := store.HasInflightWork(t.Context(), creator, pid); err != nil || !blocked {
				t.Fatal("unknown lifecycle fence", blocked, err)
			}
			if s, v, _ := scriptCounts(t, owner, pid); s != 0 || v != 0 {
				t.Fatal("control published source")
			}
			if scenario == "unknown_put_absent" {
				// An absent uncertain remote Put is not a cessation proof. Once the original
				// frozen bytes actually appear, the same cancellation is explicitly reconciled.
				raw, err := input.Sources[0].Document.CanonicalJSON()
				if err != nil {
					t.Fatal(err)
				}
				f := plan.NewSources[0].Original
				if err := putScriptFixtureBytes(t, db, f, bytes.NewReader(raw), f.ByteSize, f.MIME, f.SHA256); err != nil {
					t.Fatal(err)
				}
				expireScriptFixtureSourceDrain(t, owner, plan)
				recon := command
				recon.Key = uuid.New()
				recon.Action = "reconcile"
				recon.ExpectedRevision = current.Revision
				if _, err := recovery.Control(t.Context(), admin, recon); err != nil {
					t.Fatal(err)
				}
				final, err := recovery.Get(t.Context(), creator, pid, before.ID)
				if err != nil || final.Status != "cancelled" || final.NeedsReconciliation {
					t.Fatal("exact delayed object recovery", final, err)
				}
			}
		})
	}
}

func scriptNullTrigger(t *testing.T, owner *gorm.DB, table, condition string) {
	t.Helper()
	name := "script_null_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := owner.Exec(`CREATE FUNCTION script.` + name + `() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NULL; END $$`).Error; err != nil {
		t.Fatal(err)
	}
	if err := owner.Exec(`CREATE TRIGGER ` + name + ` BEFORE INSERT OR UPDATE ON ` + table + ` FOR EACH ROW WHEN (` + condition + `) EXECUTE FUNCTION script.` + name + `()`).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Exec(`DROP TRIGGER ` + name + ` ON ` + table).Error; err != nil {
			t.Error(err)
		}
		if err := owner.Exec(`DROP FUNCTION script.` + name + `()`).Error; err != nil {
			t.Error(err)
		}
	})
}
func TestScriptSourceRecoveryPGZeroRowFinalizationRollsBackAndCleanupRecovers(t *testing.T) {
	for _, stage := range []string{"project_head", "permanent_result", "source_audit"} {
		t.Run(stage, func(t *testing.T) {
			db, owner := scriptTestDB(t)
			creator, pid := scriptActorProject(t, owner)
			admin := scriptAdmin(t, owner, creator.OrgID)
			privateObjects := scriptPrivateObjects(t, db)
			store := scriptStore(db)
			sources := app.NewSourceService(store, privateObjects, time.Now)
			table, condition := "script.project_state", "NEW.project_id='"+pid.String()+"'::uuid"
			if stage == "permanent_result" {
				table = "script.command_result"
				condition = "NEW.actor_id='" + creator.ID.String() + "'::uuid"
			}
			if stage == "source_audit" {
				table = "infra.outbox"
				condition = "NEW.partition_key='" + pid.String() + "' AND NEW.payload#>>'{data,action}'='script.source_create'"
			}
			scriptNullTrigger(t, owner, table, condition)
			_, input := sourceCommand()
			input.ProjectID = pid
			if _, err := sources.Write(t.Context(), creator, input); !errors.Is(err, app.ErrConflict) {
				t.Fatal("zero-row commit accepted", err)
			}
			plan := pendingScriptPlan(t, owner, pid)
			cleanScriptPlan(t, db, privateObjects, plan)
			if s, v, c := scriptCounts(t, owner, pid); s != 0 || v != 0 || c != 1 {
				t.Fatal("partial formal facts", s, v, c)
			}
			var revision int64
			if err := owner.Raw(`SELECT revision FROM workspace.project WHERE id=?`, pid).Scan(&revision).Error; err != nil || revision != 1 {
				t.Fatal("zero row project touched", revision, err)
			}
			recovery := app.NewSourceRecovery(store, sources, time.Now)
			page, err := recovery.List(t.Context(), admin, pid, 0, 100)
			if err != nil {
				t.Fatal(err)
			}
			command := app.SourceControlCommand{ProjectID: pid, IntentID: page.Items[0].ID, Key: uuid.New(), RequestID: uuid.New(), Action: "cancel", ExpectedRevision: page.Items[0].Revision}
			if _, err := recovery.Control(t.Context(), admin, command); err != nil {
				t.Fatal(err)
			}
			current, err := recovery.Get(t.Context(), admin, pid, command.IntentID)
			if err != nil || current.Status != "pending" || !current.NeedsReconciliation {
				t.Fatal("unexpired rollback outputs lost their drain fence", current, err)
			}
			assertScriptFixtureObjectPresent(t, privateObjects, plan.Objects[0])
			expireScriptFixtureSourceDrain(t, owner, plan)
			reconcile := command
			reconcile.Key = uuid.New()
			reconcile.RequestID = uuid.New()
			reconcile.Action = "reconcile"
			reconcile.ExpectedRevision = current.Revision
			if _, err := recovery.Control(t.Context(), admin, reconcile); err != nil {
				t.Fatal(err)
			}
			current, err = recovery.Get(t.Context(), admin, pid, command.IntentID)
			if err != nil || current.Status != "cancelled" {
				t.Fatal("zero row unpublished cleanup", current, err)
			}
		})
	}
}

func TestScriptSourceRecoveryPGCleanupZeroRowKeepsProofForExplicitReconcile(t *testing.T) {
	db, owner := scriptTestDB(t)
	creator, pid := scriptActorProject(t, owner)
	admin := scriptAdmin(t, owner, creator.OrgID)
	privateObjects := scriptPrivateObjects(t, db)
	store := scriptStore(db)
	sources := app.NewSourceService(store, &lostPrivatePut{PrivateObjects: privateObjects}, time.Now)
	recovery := app.NewSourceRecovery(store, sources, time.Now)
	_, input := sourceCommand()
	input.ProjectID = pid
	if _, err := sources.Write(t.Context(), creator, input); !errors.Is(err, app.ErrNeedsReconciliation) {
		t.Fatal(err)
	}
	plan := pendingScriptPlan(t, owner, pid)
	cleanScriptPlan(t, db, privateObjects, plan)
	// Inject only cleanup bookkeeping loss, after actual deletion. The pre-delete
	// digest proof must survive so a later absence can be safely reconciled.
	name := "script_remove_null_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := owner.Exec(`CREATE FUNCTION script.` + name + `() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.removed THEN RETURN NULL; END IF; RETURN NEW; END $$`).Error; err != nil {
		t.Fatal(err)
	}
	// Fail only the receipt for the exact real object written before the lost
	// response. Earlier confirmed-absent objects may now correctly finish their
	// own owner bookkeeping without consuming this post-delete fault.
	fault := plan.Objects[0]
	if err := owner.Exec(`CREATE TRIGGER ` + name + ` BEFORE UPDATE ON script.object_state FOR EACH ROW WHEN (NEW.object_location_id='` + fault.LocationID.String() + `'::uuid AND NEW.object_key='` + strings.ReplaceAll(fault.Key, "'", "''") + `') EXECUTE FUNCTION script.` + name + `()`).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Exec(`DROP TRIGGER IF EXISTS ` + name + ` ON script.object_state`).Error; err != nil {
			t.Error(err)
		}
		if err := owner.Exec(`DROP FUNCTION IF EXISTS script.` + name + `()`).Error; err != nil {
			t.Error(err)
		}
	})
	page, err := recovery.List(t.Context(), admin, pid, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	command := app.SourceControlCommand{ProjectID: pid, IntentID: page.Items[0].ID, Key: uuid.New(), RequestID: uuid.New(), Action: "cancel", ExpectedRevision: page.Items[0].Revision}
	if _, err := recovery.Control(t.Context(), admin, command); err != nil {
		t.Fatal(err)
	}
	current, err := recovery.Get(t.Context(), admin, pid, command.IntentID)
	if err != nil || current.Status != "pending" || !current.NeedsReconciliation {
		t.Fatal("cleanup zero row cleared fence", current, err)
	}
	assertScriptFixtureObjectPresent(t, privateObjects, plan.Objects[0])
	expireScriptFixtureSourceDrain(t, owner, plan)
	probe := command
	probe.Key = uuid.New()
	probe.RequestID = uuid.New()
	probe.Action = "reconcile"
	probe.ExpectedRevision = current.Revision
	if _, err := recovery.Control(t.Context(), admin, probe); err != nil {
		t.Fatal(err)
	}
	reader, absenceErr := privateObjects.GetObject(t.Context(), plan.Objects[0])
	if absenceErr == nil {
		// The native COS SDK opens lazily; consume the response to observe its
		// actual missing-object result before checking the lost receipt.
		_, absenceErr = io.ReadAll(reader)
		absenceErr = errors.Join(absenceErr, reader.Close())
	}
	if !errors.Is(absenceErr, app.ErrObjectMissing) {
		t.Fatal("bookkeeping fault did not follow real deletion", absenceErr)
	}
	current, err = recovery.Get(t.Context(), admin, pid, command.IntentID)
	if err != nil || current.Status != "pending" || !current.NeedsReconciliation {
		t.Fatal("lost deletion receipt cleared cancellation fence", current, err)
	}
	if err := owner.Exec(`DROP TRIGGER ` + name + ` ON script.object_state`).Error; err != nil {
		t.Fatal(err)
	}
	command.Key = uuid.New()
	command.Action = "reconcile"
	command.ExpectedRevision = current.Revision
	if _, err := recovery.Control(t.Context(), admin, command); err != nil {
		t.Fatal(err)
	}
	current, err = recovery.Get(t.Context(), admin, pid, command.IntentID)
	if err != nil || current.Status != "cancelled" || current.NeedsReconciliation {
		t.Fatal("original absent object proof recovery", current, err)
	}
}
