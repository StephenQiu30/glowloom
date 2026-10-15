package identity_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	inbox "github.com/StephenQiu30/glowloom-server/backend/internal/infra/inbox/application"
	scriptevent "github.com/StephenQiu30/glowloom-server/backend/internal/script/adapter/event"
	pgscript "github.com/StephenQiu30/glowloom-server/backend/internal/script/adapter/postgres"
	script "github.com/StephenQiu30/glowloom-server/backend/internal/script/application"
	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/postgres"
)

func TestSecondarySourceRecoveryUsesTeamWriteAuthority(t *testing.T) {
	tx, actor, team, scope, project, requests := projectConsumerFixture(t)
	personal := actor.ScopeID
	actor.ScopeID = scope
	var author struct{ OwnerID uuid.UUID }
	if err := tx.Raw(`SELECT owner_id FROM workspace.project WHERE id=?`, project).Scan(&author).Error; err != nil {
		t.Fatal(err)
	}
	key := uuid.New()
	plan := script.WritePlan{ActorID: author.OwnerID, OrgID: actor.OrgID, Command: script.SourceCommand{ProjectID: project, Key: key, Action: "create"}}
	id := uuid.NewSHA1(key, []byte("source-write/"+author.OwnerID.String()+"/"+project.String()))
	raw, _ := json.Marshal(plan)
	for _, q := range []string{`INSERT INTO script.request(actor_id,request_id,org_id,project_id,action,request_hash,created_at)VALUES(?,?,?,?,'create',repeat('a',64),now())`, `INSERT INTO script.command(actor_id,request_id,org_id,project_id,action,request_hash,plan,created_at)VALUES(?,?,?,?,'create',repeat('a',64),?::jsonb,now())`} {
		args := []any{author.OwnerID, key, actor.OrgID, project}
		if strings.Contains(q, "plan,") {
			args = append(args, string(raw))
		}
		if err := tx.Exec(q, args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Exec(`INSERT INTO script.command_state(actor_id,request_id,id,status,updated_at)VALUES(?,?,?,'pending',now())`, author.OwnerID, key, id).Error; err != nil {
		t.Fatal(err)
	}
	store := pgscript.NewSourceStore(tx, func(tx *gorm.DB) script.ProjectAccess {
		return pgworkspace.NewScopedProjectContentAccessStore(tx, requests(tx))
	})
	record, err := store.SourceWrite(t.Context(), actor, project, id)
	if err != nil || !record.View.CanControl {
		t.Fatalf("active non-author cannot control retained source: %v %+v", err, record.View)
	}
	other := actor
	other.ScopeID = personal
	other.PlatformRole = "platform_admin"
	if _, err := store.SourceWrite(t.Context(), other, project, id); err == nil {
		t.Fatal("valid other scope read source control")
	}
	if err := tx.Exec(`UPDATE workspace.team SET status='archived' WHERE id=?`, team).Error; err != nil {
		t.Fatal(err)
	}
	if r, err := store.SourceWrite(t.Context(), actor, project, id); err != nil || r.View.CanControl {
		t.Fatalf("archived controls: %v %+v", err, r.View)
	}
	if err := tx.Exec(`UPDATE workspace.team SET status='active' WHERE id=?`, team).Error; err != nil {
		t.Fatal(err)
	}
	control := script.SourceControlCommand{ProjectID: project, IntentID: id, Action: "cancel", ExpectedRevision: 1, Key: uuid.New(), RequestID: uuid.New()}
	if _, err := store.ControlSourceWrite(t.Context(), actor, control, time.Now()); err != nil {
		t.Fatalf("team cancellation: %v", err)
	}
	if err := tx.Exec(`UPDATE identity.user_session SET revoked_at=now() WHERE id=?`, actor.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := store.ControlSourceWrite(t.Context(), actor, control, time.Now()); !errors.Is(err, identity.ErrSessionInvalid) {
		t.Fatalf("revoked source receipt: %v", err)
	}
}

func TestSecondaryImportControlKeepsRetryActor(t *testing.T) {
	tx, actor, team, scope, project, requests := projectConsumerFixture(t)
	personal := actor.ScopeID
	actor.ScopeID = scope
	var author struct{ OwnerID uuid.UUID }
	if err := tx.Raw(`SELECT owner_id FROM workspace.project WHERE id=?`, project).Scan(&author).Error; err != nil {
		t.Fatal(err)
	}
	key, job, asset := uuid.New(), uuid.New(), uuid.New()
	if err := tx.Exec(`INSERT INTO media.media_asset(id,project_id,kind,origin,status,object_key,mime_type,byte_size,sha256,moderation_status,file_name,codec)VALUES(?,?,'document','upload','ready',?,'text/plain',1,repeat('a',64),'passed','test.txt','txt')`, asset, project, "secondary/"+asset.String()+".txt").Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`INSERT INTO script.request(actor_id,request_id,org_id,project_id,action,request_hash,created_at)VALUES(?,?,?,?,'file_import_create',repeat('a',64),now())`, author.OwnerID, key, actor.OrgID, project).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`INSERT INTO script.import_job(id,org_id,project_id,actor_id,request_id,request_hash,expected_script_revision,rights_confirmed_at,created_at)VALUES(?,?,?,?,?,repeat('a',64),0,now(),now())`, job, actor.OrgID, project, author.OwnerID, key).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`INSERT INTO script.import_state(job_id,revision,attempt,status,stage,latest_script_revision,io_state,updated_at)VALUES(?,1,1,'queued','queued',0,'idle',now())`, job).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`INSERT INTO script.import_attempt(job_id,attempt,expected_script_revision,publication_key,positions,created_at)VALUES(?,1,0,?,ARRAY[0],now())`, job, uuid.New()).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`INSERT INTO script.import_file(job_id,position,source_id,asset_id,project_id,media_revision,sha256,byte_size,mime,file_name)VALUES(?,0,?,?,?,1,repeat('a',64),1,'text/plain','test.txt')`, job, uuid.New(), asset, project).Error; err != nil {
		t.Fatal(err)
	}
	store := pgscript.NewImportStoreWithWorkerAccess(tx, func(tx *gorm.DB) script.ProjectAccess {
		return pgworkspace.NewScopedProjectContentAccessStore(tx, requests(tx))
	}, func(tx *gorm.DB) script.ProjectAccess {
		return pgworkspace.NewWorkerProjectContentAccessStore(tx, secondaryWorkers(tx))
	}, nil)
	current, err := store.Import(t.Context(), actor, project, job)
	if err != nil || !current.CanControl || current.Retryable {
		t.Fatalf("collaborator control flags: %v %+v", err, current)
	}
	other := actor
	other.ScopeID = personal
	other.PlatformRole = "platform_admin"
	if _, err := store.Import(t.Context(), other, project, job); err == nil {
		t.Fatal("other scope read import")
	}
	if err := tx.Exec(`UPDATE workspace.team SET status='archived' WHERE id=?`, team).Error; err != nil {
		t.Fatal(err)
	}
	if r, err := store.Import(t.Context(), actor, project, job); err != nil || r.CanControl || r.Retryable {
		t.Fatalf("archived import flags: %v %+v", err, r)
	}
	if err := tx.Exec(`UPDATE workspace.team SET status='active' WHERE id=?`, team).Error; err != nil {
		t.Fatal(err)
	}
	command := script.ImportControl{ProjectID: project, JobID: job, Action: "retry", ExpectedRevision: 1, Key: uuid.New(), RequestID: uuid.New()}
	if _, err := store.ControlImport(t.Context(), actor, command, time.Now()); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("retry took over durable actor: %v", err)
	}
	command.Action = "cancel"
	command.Key = uuid.New()
	if _, err := store.ControlImport(t.Context(), actor, command, time.Now()); err != nil {
		t.Fatalf("collaborator cancellation: %v", err)
	}
	if err := tx.Exec(`UPDATE identity.user_session SET revoked_at=now() WHERE id=?`, actor.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := store.ControlImport(t.Context(), actor, command, time.Now()); !errors.Is(err, identity.ErrSessionInvalid) {
		t.Fatalf("revoked import receipt: %v", err)
	}

	var event struct{ EventID uuid.UUID }
	if err := tx.Raw(`SELECT event_id FROM script.import_command WHERE actor_id=? AND request_id=?`, actor.ID, command.Key).Scan(&event).Error; err != nil {
		t.Fatal(err)
	}
	delivery := script.ImportDelivery{ImportWork: script.ImportWork{JobID: job, Attempt: 1}, EventID: event.EventID, RequestID: command.Key, ActorID: actor.ID, OrgID: actor.OrgID, ProjectID: project, Action: "cancel"}
	if ok, err := store.VerifyImportDelivery(t.Context(), delivery); err != nil || !ok {
		t.Fatalf("worker cancellation depended on old HTTP session: %v %v", ok, err)
	}
	if _, controller, err := store.ImportControlRecord(t.Context(), delivery); err != nil || controller.ID != actor.ID {
		t.Fatalf("durable controller: %v", err)
	}
	if err := tx.Exec(`UPDATE workspace.team_member SET status='removed' WHERE team_id=? AND user_id=?`, team, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if ok, err := store.VerifyImportDelivery(t.Context(), delivery); err != nil || ok {
		t.Fatalf("removed controller event must be acknowledged without dispatch: %v %v", ok, err)
	}
	if _, _, err := store.ImportControlRecord(t.Context(), delivery); err == nil {
		t.Fatal("removed controller entered import recovery")
	}

	var envelope struct{ Payload []byte }
	if err := tx.Raw(`SELECT payload FROM infra.outbox WHERE id=?`, event.EventID).Scan(&envelope).Error; err != nil {
		t.Fatal(err)
	}
	started := &secondaryImportStarter{}
	handler := scriptevent.NewImportHandler(secondaryInbox{}, store, started)
	if err := handler.Handle(t.Context(), inbox.Record{Topic: scriptevent.ImportTopic, Key: []byte(project.String()), Value: envelope.Payload}); err != nil || started.calls != 0 {
		t.Fatalf("obsolete import handler stopped consumer or dispatched: %v %d", err, started.calls)
	}
	forged := delivery
	forged.EventID = uuid.New()
	if _, err := store.VerifyImportDelivery(t.Context(), forged); !errors.Is(err, script.ErrIdempotencyConflict) {
		t.Fatalf("forged import receipt treated as obsolete: %v", err)
	}
}

type secondaryImportStarter struct{ calls int }

func (s *secondaryImportStarter) Deliver(context.Context, script.ImportDelivery) error {
	s.calls++
	return nil
}
