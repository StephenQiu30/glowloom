package microservices_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	creationpg "github.com/StephenQiu30/glowloom-server/backend/creation/prompt/adapter/postgres"
	creationapp "github.com/StephenQiu30/glowloom-server/backend/creation/prompt/application"
	promptdomain "github.com/StephenQiu30/glowloom-server/backend/creation/prompt/domain"
	executionpg "github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/postgres"
	executionapp "github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

type frozenFixture struct {
	admin, creation, execution, identity, media *gorm.DB
	request                                     creationapp.FreezeInputRequest
	draft                                       promptdomain.Draft
	store                                       *creationpg.FrozenInputStore
}

func frozenDraft(text string) promptdomain.Draft {
	return promptdomain.Draft{SchemaVersion: 1, Document: promptdomain.Document{Type: "doc", Content: []promptdomain.Paragraph{{Type: "paragraph", Content: []promptdomain.Inline{{Type: "text", Text: text}}}}}, References: []promptdomain.Reference{}, Params: map[string]any{}, LabelState: promptdomain.LabelState{HighWater: map[string]int64{"image": 0, "video": 0, "audio": 0, "subject": 0}, Allocations: map[string]promptdomain.LabelAllocation{}}}
}
func newFrozenFixture(t *testing.T, aspects ...string) *frozenFixture {
	t.Helper()
	aspect := "16:9"
	if len(aspects) > 1 {
		t.Fatal("one project aspect ratio is required")
	}
	if len(aspects) == 1 {
		aspect = aspects[0]
	}
	if aspect != "9:16" && aspect != "16:9" && aspect != "1:1" {
		t.Fatal("unsupported fixture project aspect ratio")
	}
	f := &frozenFixture{admin: fenceDatabase(t, ""), creation: authorityDatabase(t, "creation"), execution: authorityDatabase(t, "execution"), identity: authorityDatabase(t, "identity"), media: authorityDatabase(t, "media"), draft: frozenDraft("original frozen prompt")}
	org, actor, session, scope, project, draftID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	statements := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO workspace.organization(id,name) VALUES (?,'handoff fixture')`, []any{org}},
		{`INSERT INTO identity."user"(id,org_id,login_name,display_name,role,password_hash,must_change_password) VALUES (?,?,?,'handoff fixture','producer','fixture',false)`, []any{actor, org, actor.String()}},
		{`INSERT INTO workspace.creation_scope(id,kind,owner_user_id,physical_scope_id) VALUES (?,'personal',?,?)`, []any{scope, actor, org}},
		{`INSERT INTO workspace.project(id,org_id,owner_id,owner_scope_id,name,aspect_ratio,style_type) VALUES (?,?,?,?,'handoff fixture',?,'realistic')`, []any{project, org, actor, scope, aspect}},
	}
	for _, statement := range statements {
		if err := f.admin.Exec(statement.sql, statement.args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	dh, ih := f.draft.Hashes()
	raw, _ := json.Marshal(f.draft)
	if err := f.admin.Exec(`INSERT INTO workspace.prompt_draft(id,project_id,actor_id,revision,draft,draft_hash,input_hash) VALUES(?,?,?,1,?::jsonb,?,?)`, draftID, project, actor, string(raw), dh, ih).Error; err != nil {
		t.Fatal(err)
	}
	authorities := []writefence.Authority{
		{Issuer: writefence.Identity, Kind: writefence.Account, ID: actor},
		{Issuer: writefence.Identity, Kind: writefence.Session, ID: session},
		{Issuer: writefence.Creation, Kind: writefence.Scope, ID: scope},
		{Issuer: writefence.Creation, Kind: writefence.Project, ID: project},
	}
	for i, authority := range authorities {
		owner := f.creation
		if authority.Issuer == writefence.Identity {
			owner = f.identity
		}
		coordination := completedAuthority(t, pgfence.NewAuthorityStore(owner), authority, false)
		authorities[i] = coordination.Control.Authority
		for _, target := range []*gorm.DB{f.identity, f.creation, f.media, f.execution} {
			if err := pgfence.New(target).Install(t.Context(), coordination.Control); err != nil {
				t.Fatal(err)
			}
		}
	}
	f.request = creationapp.FreezeInputRequest{HandoffID: uuid.New(), RequestID: uuid.New(), Command: writefence.Command{ID: uuid.New(), Action: "execution.frozen_input.accept", SHA256: strings.Repeat("a", 64), Authorities: authorities}, ActorID: actor, SessionID: session, ScopeID: scope, ProjectID: project, Source: promptdomain.InputSource{Kind: "prompt_draft", DraftID: draftID, Revision: 1}, SourceGeneration: 1, ExpectedScopeRevision: 1, ExpectedProjectRevision: 1, Pins: []promptdomain.FrozenPin{}}
	f.bind(t)
	f.store = creationpg.NewFrozenInputStore(f.creation, nil)
	return f
}
func (f *frozenFixture) bind(t *testing.T) {
	t.Helper()
	f.request.SourceCommand = writefence.Command{ID: uuid.NewSHA1(f.request.HandoffID, []byte("creation.frozen_input.freeze")), Action: "creation.frozen_input.freeze", Authorities: append([]writefence.Authority{}, f.request.Command.Authorities...)}
	hash, err := f.request.SourceCommandSHA256()
	if err != nil {
		t.Fatal(err)
	}
	f.request.SourceCommand.SHA256 = hash
	for _, db := range []*gorm.DB{f.identity, f.creation} {
		if err := registerAuthority(t, db, pgfence.NewAuthorityStore(db), f.request.Command, writefence.WriterExecution); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pgfence.New(f.creation).Admit(t.Context(), f.request.SourceCommand); err != nil {
		t.Fatal(err)
	}
	if _, err := pgfence.New(f.execution).Admit(t.Context(), f.request.Command); err != nil {
		t.Fatal(err)
	}
}
func (f *frozenFixture) freeze(t *testing.T) creationapp.FrozenInputEnvelope {
	t.Helper()
	out, err := f.store.FreezeInput(t.Context(), f.request)
	if err != nil {
		t.Fatal("freeze", err)
	}
	return out
}
func (f *frozenFixture) cancelCommand(t *testing.T) writefence.Command {
	t.Helper()
	command := f.request.Command
	command.ID = uuid.New()
	command.Action = "creation.frozen_input.cancel"
	envelope := f.freeze(t)
	command.SHA256, _ = creationapp.FrozenInputCancellationSHA256(envelope.Key(), command)
	for _, db := range []*gorm.DB{f.creation, f.execution} {
		if _, err := pgfence.New(db).Admit(t.Context(), command); err != nil {
			t.Fatal(err)
		}
	}
	return command
}
func executionKey(e creationapp.FrozenInputEnvelope) executionapp.FrozenInputKey {
	k := e.Key()
	return executionapp.FrozenInputKey{HandoffID: k.HandoffID, CommandID: k.CommandID, CommandSHA256: k.CommandSHA256, SnapshotSHA256: k.SnapshotSHA256}
}

type frozenSource func(context.Context, executionapp.FrozenInputKey) (executionapp.FrozenInput, error)

func (s frozenSource) ReadFrozenInput(ctx context.Context, key executionapp.FrozenInputKey) (executionapp.FrozenInput, error) {
	return s(ctx, key)
}
func localFrozenSource(store *creationpg.FrozenInputStore) frozenSource {
	return func(ctx context.Context, key executionapp.FrozenInputKey) (executionapp.FrozenInput, error) {
		envelope, err := store.ReadFrozenInput(ctx, creationapp.FrozenInputKey{HandoffID: key.HandoffID, CommandID: key.CommandID, CommandSHA256: key.CommandSHA256, SnapshotSHA256: key.SnapshotSHA256})
		if err != nil {
			return executionapp.FrozenInput{}, err
		}
		in := envelope.Input
		raw, _, err := in.Snapshot()
		return executionapp.FrozenInput{Key: key, RequestID: in.RequestID, Command: in.Command, ActorID: in.ActorID, SessionID: in.SessionID, ScopeID: in.ScopeID, ProjectID: in.ProjectID, OrgID: in.OrgID, AspectRatio: in.AspectRatio, SourceGeneration: in.SourceGeneration, DraftRevision: in.DraftRevision, Snapshot: raw, AdoptionBlocked: envelope.AdoptionBlocked, CancelCommand: envelope.CancelCommand}, err
	}
}
func creationProof(p executionapp.FrozenInputCancelProof) creationapp.FrozenInputCancelProof {
	return creationapp.FrozenInputCancelProof{Key: creationapp.FrozenInputKey{HandoffID: p.Key.HandoffID, CommandID: p.Key.CommandID, CommandSHA256: p.Key.CommandSHA256, SnapshotSHA256: p.Key.SnapshotSHA256}, CancelCommandID: p.CancelCommandID, Outcome: p.Outcome, Original: p.Original}
}

func TestFrozenInputPostgresSnapshotReplayAndSourceMutation(t *testing.T) {
	f := newFrozenFixture(t)
	envelope := f.freeze(t)
	var physical struct{ OrgID uuid.UUID }
	if err := f.creation.Raw(`SELECT org_id FROM workspace.project WHERE id=?`, f.request.ProjectID).Scan(&physical).Error; err != nil {
		t.Fatal(err)
	}
	if envelope.Input.OrgID != physical.OrgID || physical.OrgID == f.request.ScopeID {
		t.Fatal("snapshot substituted scope ID for the locked physical organization")
	}
	if envelope.Input.AspectRatio != "16:9" {
		t.Fatal("snapshot did not capture the locked project's aspect ratio")
	}
	original, _ := json.Marshal(envelope.Input)
	replay, err := creationpg.NewFrozenInputStore(f.creation, nil).FreezeInput(t.Context(), f.request)
	if err != nil || !reflect.DeepEqual(envelope, replay) {
		t.Fatalf("replay=%+v err=%v", replay, err)
	}
	changed := frozenDraft("edited later")
	dh, ih := changed.Hashes()
	raw, _ := json.Marshal(changed)
	if err := f.creation.Exec(`UPDATE workspace.prompt_draft SET revision=2,draft=?::jsonb,draft_hash=?,input_hash=? WHERE id=?`, string(raw), dh, ih, f.request.Source.DraftID).Error; err != nil {
		t.Fatal(err)
	}
	reloaded, err := f.store.ReadFrozenInput(t.Context(), envelope.Key())
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _ := json.Marshal(reloaded.Input)
	if string(snapshot) != string(original) {
		t.Fatal("editor mutation changed frozen snapshot")
	}
	if _, err := f.store.FreezeInput(t.Context(), f.request); err != nil {
		t.Fatal("replay improperly rechecks mutable draft", err)
	}
	conflicting := f.request
	conflicting.RequestID = uuid.New()
	if _, err := f.store.FreezeInput(t.Context(), conflicting); !errors.Is(err, promptdomain.ErrFrozenInputConflict) {
		t.Fatalf("same command changed content err=%v", err)
	}
	f.request.HandoffID = uuid.New()
	f.request.RequestID = uuid.New()
	f.request.Command.ID = uuid.New()
	f.request.SourceGeneration = 2
	f.bind(t)
	if _, err := f.store.FreezeInput(t.Context(), f.request); !errors.Is(err, promptdomain.ErrInputStale) {
		t.Fatalf("stale saved version err=%v", err)
	}
	var count int64
	for _, table := range []string{"workspace.frozen_input_handoff", "infra.task_work"} {
		predicate := "handoff_id=?"
		id := f.request.HandoffID
		if table == "infra.task_work" {
			predicate = "service_owner='creation' AND business_id=?"
		}
		if err := f.admin.Table(table).Where(predicate, id).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("failed source left %s effects=%d err=%v", table, count, err)
		}
	}
}

func TestFrozenInputPostgresRequiresOriginalExecutionTargetAdmission(t *testing.T) {
	for _, target := range []writefence.Writer{"", writefence.WriterCreation} {
		t.Run(string(target), func(t *testing.T) {
			f := newFrozenFixture(t)
			req := f.request
			req.HandoffID = uuid.New()
			req.RequestID = uuid.New()
			req.Command.ID = uuid.New()
			req.SourceCommand.ID = uuid.NewSHA1(req.HandoffID, []byte("creation.frozen_input.freeze"))
			req.SourceCommand.SHA256, _ = req.SourceCommandSHA256()
			if err := registerAuthority(t, f.identity, pgfence.NewAuthorityStore(f.identity), req.Command, writefence.WriterExecution); err != nil {
				t.Fatal(err)
			}
			if target != "" {
				if err := registerAuthority(t, f.creation, pgfence.NewAuthorityStore(f.creation), req.Command, target); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := pgfence.New(f.creation).Admit(t.Context(), req.SourceCommand); err != nil {
				t.Fatal(err)
			}
			_, err := f.store.FreezeInput(t.Context(), req)
			expected := writefence.ErrFenced
			if target != "" {
				expected = promptdomain.ErrFrozenInputConflict
			}
			if !errors.Is(err, expected) {
				t.Fatalf("source-local admission substituted for original target: target=%q err=%v", target, err)
			}
			var count int64
			if err := f.creation.Table("workspace.frozen_input_handoff").Where("handoff_id=?", req.HandoffID).Count(&count).Error; err != nil || count != 0 {
				t.Fatalf("mismatch left snapshot=%d err=%v", count, err)
			}
			saved, err := pgfence.New(f.creation).Load(t.Context(), req.SourceCommand.ID)
			if err != nil || saved.State != writefence.Admitted {
				t.Fatalf("mismatch completed separate source identity %+v %v", saved, err)
			}
		})
	}
	f := newFrozenFixture(t)
	f.request.SourceCommand.ID = f.request.Command.ID
	f.request.SourceCommand.SHA256, _ = f.request.SourceCommandSHA256()
	if _, err := f.store.FreezeInput(t.Context(), f.request); !errors.Is(err, promptdomain.ErrFrozenInputInvalid) {
		t.Fatalf("same source and consumer identity accepted: %v", err)
	}
}

func TestFrozenInputPostgresSourcePolicyAndEpochs(t *testing.T) {
	for _, test := range []struct {
		name, sql string
		args      func(*frozenFixture) []any
	}{
		{"scope owner", `UPDATE workspace.creation_scope SET owner_user_id=(SELECT id FROM identity."user" WHERE login_name=?) WHERE id=?`, func(f *frozenFixture) []any {
			return []any{"handoff-foreign-" + f.request.ActorID.String(), f.request.ScopeID}
		}},
		{"archived project", `UPDATE workspace.project SET status='archived',archived_at=now() WHERE id=?`, func(f *frozenFixture) []any { return []any{f.request.ProjectID} }},
		{"deleted project", `UPDATE workspace.project SET is_delete=true WHERE id=?`, func(f *frozenFixture) []any { return []any{f.request.ProjectID} }},
		{"project revision", `UPDATE workspace.project SET revision=2 WHERE id=?`, func(f *frozenFixture) []any { return []any{f.request.ProjectID} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newFrozenFixture(t)
			if test.name == "scope owner" {
				if err := f.admin.Exec(`INSERT INTO identity."user"(id,org_id,login_name,display_name,role,password_hash,must_change_password) SELECT ?,org_id,?,'foreign handoff','producer','fixture',false FROM identity."user" WHERE id=?`, uuid.New(), "handoff-foreign-"+f.request.ActorID.String(), f.request.ActorID).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := f.admin.Exec(test.sql, test.args(f)...).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := f.store.FreezeInput(t.Context(), f.request); err == nil {
				t.Fatal("invalid current policy admitted")
			}
		})
	}
	t.Run("missing prior admission", func(t *testing.T) {
		f := newFrozenFixture(t)
		f.request.HandoffID = uuid.New()
		f.request.SourceCommand.ID = uuid.NewSHA1(f.request.HandoffID, []byte("creation.frozen_input.freeze"))
		hash, _ := f.request.SourceCommandSHA256()
		f.request.SourceCommand.SHA256 = hash
		if _, err := f.store.FreezeInput(t.Context(), f.request); !errors.Is(err, writefence.ErrNotFound) {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("blocked authority", func(t *testing.T) {
		f := newFrozenFixture(t)
		a := f.request.Command.Authorities[3]
		a.Epoch++
		if err := pgfence.New(f.creation).Install(t.Context(), writefence.Control{Authority: a, Blocked: true, CoordinationID: uuid.New()}); err != nil {
			t.Fatal(err)
		}
		if _, err := f.store.FreezeInput(t.Context(), f.request); !errors.Is(err, writefence.ErrFenced) {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("source generation is separate", func(t *testing.T) {
		f := newFrozenFixture(t)
		f.request.SourceGeneration = 2
		f.request.HandoffID = uuid.New()
		f.request.Command.ID = uuid.New()
		f.bind(t)
		if _, err := f.store.FreezeInput(t.Context(), f.request); !errors.Is(err, promptdomain.ErrFrozenInputConflict) {
			t.Fatalf("err=%v", err)
		}
	})
}

func TestFrozenInputPostgresActualOwnersAndNoOuterTransaction(t *testing.T) {
	f := newFrozenFixture(t)
	for owner, db := range map[string]*gorm.DB{"identity": f.identity, "creation": f.creation, "media": f.media, "execution": f.execution} {
		t.Run(owner, func(t *testing.T) {
			if owner != "creation" {
				if _, err := creationpg.NewFrozenInputStore(db, nil).FreezeInput(t.Context(), f.request); !errors.Is(err, promptdomain.ErrFrozenInputOwner) {
					t.Fatalf("source wrong role err=%v", err)
				}
			}
			if owner != "execution" {
				if _, err := executionpg.NewFrozenInputStore(db, localFrozenSource(f.store)).AcceptFrozenInput(t.Context(), executionapp.FrozenInputKey{}); !errors.Is(err, executionapp.ErrFrozenInputOwner) {
					t.Fatalf("target wrong role err=%v", err)
				}
			}
			forbidden := []string{"workspace.frozen_input_handoff", "workspace.prompt_draft", "canvas.node_input"}
			if owner == "creation" {
				forbidden = []string{"operation.frozen_input_acceptance", "operation.frozen_input_cancellation", "operation.operation", "identity.session", "media.reference_pin"}
			}
			for _, table := range forbidden {
				if owner == "execution" && strings.HasPrefix(table, "operation.") {
					continue
				}
				if owner == "creation" && strings.HasPrefix(table, "workspace.") {
					continue
				}
				if err := db.Exec("SELECT * FROM " + table + " LIMIT 0").Error; err == nil {
					t.Fatal("cross-owner SQL allowed", table)
				}
			}
		})
	}
	sourceTx := f.creation.Begin()
	defer func() { _ = sourceTx.Rollback().Error }()
	if _, err := creationpg.NewFrozenInputStore(sourceTx, nil).FreezeInput(t.Context(), f.request); !errors.Is(err, writefence.ErrTransaction) {
		t.Fatalf("source outer Tx err=%v", err)
	}
	targetTx := f.execution.Begin()
	defer func() { _ = targetTx.Rollback().Error }()
	if _, err := executionpg.NewFrozenInputStore(targetTx, localFrozenSource(f.store)).AcceptFrozenInput(t.Context(), executionapp.FrozenInputKey{}); !errors.Is(err, writefence.ErrTransaction) {
		t.Fatalf("target outer Tx err=%v", err)
	}
	envelope := f.freeze(t)
	if err := f.creation.Exec(`UPDATE workspace.frozen_input_handoff SET snapshot='{}' WHERE handoff_id=?`, envelope.Input.HandoffID).Error; err == nil {
		t.Fatal("source immutable snapshot update allowed")
	}
	if err := f.creation.Exec(`DELETE FROM workspace.frozen_input_handoff WHERE handoff_id=?`, envelope.Input.HandoffID).Error; err == nil {
		t.Fatal("source responsibility deletion allowed")
	}
}

func TestFrozenInputPostgresAcceptLostACKAndCancellation(t *testing.T) {
	for _, received := range []bool{false, true} {
		t.Run(fmt.Sprintf("received=%t", received), func(t *testing.T) {
			f := newFrozenFixture(t)
			envelope := f.freeze(t)
			key := executionKey(envelope)
			source := localFrozenSource(f.store)
			store := executionpg.NewFrozenInputStore(f.execution, source)
			if received {
				receipt, err := store.AcceptFrozenInput(t.Context(), key)
				if err != nil {
					t.Fatal(err)
				}
				offline := frozenSource(func(context.Context, executionapp.FrozenInputKey) (executionapp.FrozenInput, error) {
					return executionapp.FrozenInput{}, errors.New("source must not be called for original ACK recovery")
				})
				replay, err := executionpg.NewFrozenInputStore(f.execution, offline).AcceptFrozenInput(t.Context(), key)
				if err != nil || replay != receipt {
					t.Fatalf("lost ACK did not recover original receipt %+v %v", replay, err)
				}
				changed := key
				changed.SnapshotSHA256 = strings.Repeat("e", 64)
				if _, err := store.AcceptFrozenInput(t.Context(), changed); !errors.Is(err, executionapp.ErrFrozenInputConflict) {
					t.Fatalf("changed hash accepted: %v", err)
				}
				// Receiving input does not create a quote, submission, reservation or provider task.
				var count int64
				if err := f.execution.Raw(`SELECT count(*) FROM operation.operation WHERE id=?`, receipt.OperationID).Scan(&count).Error; err != nil || count != 0 {
					t.Fatalf("receipt fabricated real execution count=%d err=%v", count, err)
				}
			}
			cancel := f.cancelCommand(t)
			blocked, err := f.store.BlockFrozenInput(t.Context(), envelope.Key(), cancel)
			if err != nil || !blocked.AdoptionBlocked || blocked.CancelProof != nil {
				t.Fatalf("source did not persist prohibition: %+v %v", blocked, err)
			}
			proof, err := store.CancelFrozenInput(t.Context(), key, cancel)
			if err != nil {
				t.Fatal(err)
			}
			expected := "not_received_frozen"
			if received {
				expected = "cancelled_input"
			}
			if proof.Outcome != expected {
				t.Fatalf("outcome=%s want=%s", proof.Outcome, expected)
			}
			completed, err := f.store.CompleteFrozenInputCancellation(t.Context(), envelope.Key(), creationProof(proof))
			if err != nil || completed.CancelProof == nil {
				t.Fatalf("complete=%+v err=%v", completed, err)
			}
			if _, err := store.AcceptFrozenInput(t.Context(), key); !errors.Is(err, executionapp.ErrFrozenInputCancelled) {
				t.Fatalf("late accept after durable cancel err=%v", err)
			}
			if _, err := creationpg.NewFrozenInputStore(f.creation, nil).BlockFrozenInput(t.Context(), envelope.Key(), cancel); err != nil {
				t.Fatal(err)
			}
			replay, err := store.CancelFrozenInput(t.Context(), key, cancel)
			if err != nil || !reflect.DeepEqual(replay, proof) {
				t.Fatalf("cancel ACK recovery err=%v", err)
			}
			changed := cancel
			changed.ID = uuid.New()
			if _, err := f.store.BlockFrozenInput(t.Context(), envelope.Key(), changed); !errors.Is(err, promptdomain.ErrFrozenInputConflict) {
				t.Fatalf("cancel original identity replaced err=%v", err)
			}
			sourceRecord, err := pgfence.New(f.creation).Load(t.Context(), f.request.SourceCommand.ID)
			if err != nil || sourceRecord.State != writefence.Committed {
				t.Fatalf("source freeze was incorrectly reused as cancellation %+v %v", sourceRecord, err)
			}
		})
	}
}

func rejectFrozenCreate(t *testing.T, db *gorm.DB) {
	t.Helper()
	name := "frozen_task_failure_" + uuid.NewString()
	if err := db.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "task_work" || tx.Statement.Table == "infra.task_work" {
			_ = tx.AddError(errors.New("injected frozen task failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Callback().Create().Remove(name); err != nil {
			t.Error(err)
		}
	})
}
func TestFrozenInputPostgresCompleteFailureRollsBackBusinessTaskAndOutbox(t *testing.T) {
	for _, owner := range []string{"creation", "execution"} {
		t.Run(owner, func(t *testing.T) {
			f := newFrozenFixture(t)
			db := f.creation
			command := f.request.SourceCommand
			var key executionapp.FrozenInputKey
			if owner == "execution" {
				key = executionKey(f.freeze(t))
				db = f.execution
				command = f.request.Command
			}
			callback := "frozen_complete_fault_" + uuid.NewString()
			if err := db.Callback().Raw().Before("gorm:raw").Register(callback, func(tx *gorm.DB) {
				sql := tx.Statement.SQL.String()
				if strings.Contains(sql, "UPDATE infra.writer_command") && strings.Contains(sql, "SET state = 'committed'") {
					_ = tx.AddError(errors.New("injected final frozen receipt failure"))
				}
			}); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := db.Callback().Raw().Remove(callback); err != nil {
					t.Error(err)
				}
			}()
			var err error
			if owner == "creation" {
				_, err = f.store.FreezeInput(t.Context(), f.request)
			} else {
				_, err = executionpg.NewFrozenInputStore(db, localFrozenSource(f.store)).AcceptFrozenInput(t.Context(), key)
			}
			if err == nil || !strings.Contains(err.Error(), "injected final frozen receipt failure") {
				t.Fatalf("final failure not propagated %v", err)
			}
			saved, err := pgfence.New(db).Load(t.Context(), command.ID)
			if err != nil || saved.State != writefence.Admitted || len(saved.Receipt) != 0 {
				t.Fatalf("failed final receipt completed command %+v %v", saved, err)
			}
			table := "workspace.frozen_input_handoff"
			if owner == "execution" {
				table = "operation.frozen_input_acceptance"
			}
			var count int64
			if err := db.Table(table).Where("handoff_id=?", f.request.HandoffID).Count(&count).Error; err != nil || count != 0 {
				t.Fatalf("final failure retained business=%d err=%v", count, err)
			}
			if err := db.Raw(`SELECT count(*) FROM infra.task_work WHERE business_id=?`, f.request.HandoffID.String()).Scan(&count).Error; err != nil || count != 0 {
				t.Fatalf("final failure retained task=%d err=%v", count, err)
			}
			if err := db.Raw(`SELECT count(*) FROM infra.outbox WHERE partition_key=?`, f.request.ProjectID.String()).Scan(&count).Error; err != nil || count != 0 {
				t.Fatalf("final failure retained outbox=%d err=%v", count, err)
			}
		})
	}
}

func TestFrozenInputPostgresTaskAndFinalReceiptRollback(t *testing.T) {
	for _, owner := range []string{"creation", "execution"} {
		t.Run(owner, func(t *testing.T) {
			f := newFrozenFixture(t)
			db := f.creation
			var key executionapp.FrozenInputKey
			if owner == "execution" {
				envelope := f.freeze(t)
				key = executionKey(envelope)
				db = f.execution
			}
			rejectFrozenCreate(t, db)
			var err error
			if owner == "creation" {
				_, err = f.store.FreezeInput(t.Context(), f.request)
			} else {
				_, err = executionpg.NewFrozenInputStore(db, localFrozenSource(f.store)).AcceptFrozenInput(t.Context(), key)
			}
			if err == nil || !strings.Contains(err.Error(), "injected frozen task failure") {
				t.Fatalf("task failure not propagated: %v", err)
			}
			commandID := f.request.Command.ID
			if owner == "creation" {
				commandID = f.request.SourceCommand.ID
			}
			record, err := pgfence.New(db).Load(t.Context(), commandID)
			if err != nil || record.State != writefence.Admitted || len(record.Receipt) != 0 {
				t.Fatalf("failed effect completed command %+v %v", record, err)
			}
			table := "workspace.frozen_input_handoff"
			if owner == "execution" {
				table = "operation.frozen_input_acceptance"
			}
			var count int64
			if err := db.Table(table).Where("handoff_id=?", f.request.HandoffID).Count(&count).Error; err != nil || count != 0 {
				t.Fatalf("business survived task rollback count=%d err=%v", count, err)
			}
			if err := db.Raw(`SELECT count(*) FROM infra.outbox WHERE partition_key=?`, f.request.ProjectID.String()).Scan(&count).Error; err != nil || count != 0 {
				t.Fatalf("outbox survived rollback count=%d err=%v", count, err)
			}
		})
	}
}

func TestFrozenInputPostgresCancelBeforeLateDeliveryAndWorkerFence(t *testing.T) {
	f := newFrozenFixture(t)
	envelope := f.freeze(t)
	key := executionKey(envelope)
	// A real source read finishes, then cancellation arrives before target commit.
	source := localFrozenSource(f.store)
	read := make(chan struct{})
	resume := make(chan struct{})
	paused := frozenSource(func(ctx context.Context, k executionapp.FrozenInputKey) (executionapp.FrozenInput, error) {
		in, err := source(ctx, k)
		close(read)
		select {
		case <-resume:
			return in, err
		case <-ctx.Done():
			return in, ctx.Err()
		}
	})
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := executionpg.NewFrozenInputStore(f.execution, paused).AcceptFrozenInput(ctx, key)
		done <- err
	}()
	<-read
	command := f.cancelCommand(t)
	if _, err := f.store.BlockFrozenInput(ctx, envelope.Key(), command); err != nil {
		t.Fatal(err)
	}
	proof, err := executionpg.NewFrozenInputStore(f.execution, source).CancelFrozenInput(ctx, key, command)
	if err != nil {
		t.Fatal(err)
	}
	close(resume)
	if err := <-done; !errors.Is(err, executionapp.ErrFrozenInputCancelled) {
		t.Fatalf("stale fetched input adopted after cancel: %v", err)
	}
	jobs := taskruntime.NewStore(f.creation).ForBusiness(key.HandoffID.String())
	job, err := jobs.Claim(ctx, []string{"creation.frozen_input.cancel"}, "old-handoff-worker", time.Minute)
	if err != nil || job == nil {
		t.Fatalf("claim=%+v err=%v", job, err)
	}
	if err := f.creation.Exec(`UPDATE infra.task_work SET owner='new-handoff-worker',epoch=epoch+1 WHERE kind=? AND business_id=?`, job.Kind, job.BusinessID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.CompleteFrozenInputCancellationForTask(ctx, envelope.Key(), creationProof(proof), *job); !errors.Is(err, taskruntime.ErrLeaseLost) {
		t.Fatalf("stale worker completion err=%v", err)
	}
	saved, err := f.store.ReadFrozenInput(ctx, envelope.Key())
	if err != nil || saved.CancelProof != nil {
		t.Fatalf("stale worker left source completion %+v %v", saved, err)
	}
	record, err := pgfence.New(f.creation).Load(ctx, command.ID)
	if err != nil || record.State != writefence.Admitted {
		t.Fatalf("stale worker completed writer %+v %v", record, err)
	}
	job.Owner = "new-handoff-worker"
	job.Epoch++
	if _, err := f.store.CompleteFrozenInputCancellationForTask(ctx, envelope.Key(), creationProof(proof), *job); err != nil {
		t.Fatal("new worker could not recover exact responsibility", err)
	}
}
