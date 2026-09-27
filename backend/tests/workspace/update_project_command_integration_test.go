package workspace_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	auditapp "github.com/StephenQiu30/lanverse/backend/internal/audit/application"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
	pgworkspace "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

func createProjectForUpdate(ctx context.Context, t *testing.T, database *gorm.DB, actor identityapp.Principal) domain.Project {
	t.Helper()
	project := projectForActor(actor)
	project.Description = "原说明"
	if err := createWorkspaceProjectInStore(ctx, pgworkspace.NewStore(database), actor, project); err != nil {
		t.Fatalf("create update fixture: %v", err)
	}
	loaded, err := pgworkspace.NewStore(database).FindProject(ctx, actor, project.ID)
	if err != nil {
		t.Fatalf("load update fixture: %v", err)
	}
	return loaded
}

func projectRowForUpdate(ctx context.Context, t *testing.T, database *gorm.DB, projectID uuid.UUID) struct {
	Name                string
	Description         string
	StylePresetID       *uuid.UUID
	AllowOverseasModels bool
	AspectRatio         string
	StyleType           string
	StyleSubtype        string
	Status              string
	IsDelete            bool
	Revision            int64
} {
	t.Helper()
	var row struct {
		Name                string
		Description         string
		StylePresetID       *uuid.UUID
		AllowOverseasModels bool
		AspectRatio         string
		StyleType           string
		StyleSubtype        string
		Status              string
		IsDelete            bool
		Revision            int64
	}
	result := database.WithContext(ctx).Raw(`
		SELECT name, description, style_preset_id, allow_overseas_models,
		       aspect_ratio, style_type, style_subtype, status, is_delete, revision
		FROM workspace.project WHERE id = ?::uuid
	`, projectID.String()).Scan(&row)
	if result.Error != nil || result.RowsAffected != 1 {
		t.Fatalf("read project row: rows=%d err=%v", result.RowsAffected, result.Error)
	}
	return row
}

func projectUpdateEventCount(ctx context.Context, t *testing.T, database *gorm.DB, projectID uuid.UUID) int64 {
	t.Helper()
	var count int64
	if err := database.WithContext(ctx).Raw(`
		SELECT count(*) FROM infra.outbox WHERE partition_key = ?
	`, projectID.String()).Scan(&count).Error; err != nil {
		t.Fatalf("count project events: %v", err)
	}
	return count
}

func TestUpdateProjectCommandCommitsMutableSettingsAndBothEvents(t *testing.T) {
	ctx, database := workspaceMigrationDB(t)
	orgID := insertWorkspaceOrganization(ctx, t, database)
	actor := insertWorkspaceActor(ctx, t, database, orgID)
	project := createProjectForUpdate(ctx, t, database, actor)
	presetID := insertStylePreset(ctx, t, database, orgID, project.ID.String(), false)
	input := updateProjectInput(project.ID)
	input.StylePresetID = &presetID
	updated, err := workspaceapp.NewUpdateProjectCommand(pgworkspace.NewStore(database), time.Now).Execute(ctx, actor, input)
	if err != nil || updated.ID != project.ID || updated.Name != "新名称" ||
		updated.Description != *input.Description || updated.StylePresetID == nil ||
		*updated.StylePresetID != presetID || !updated.AllowOverseasModels || updated.Revision != 2 {
		t.Fatalf("update result = %+v: %v", updated, err)
	}
	row := projectRowForUpdate(ctx, t, database, project.ID)
	if row.Name != "新名称" || row.Description != *input.Description ||
		row.StylePresetID == nil || *row.StylePresetID != presetID ||
		!row.AllowOverseasModels || row.Revision != 2 ||
		row.AspectRatio != project.AspectRatio || row.StyleType != project.StyleType ||
		row.StyleSubtype != project.StyleSubtype || row.Status != "active" || row.IsDelete {
		t.Fatalf("committed project settings = %+v", row)
	}
	if got := projectUpdateEventCount(ctx, t, database, project.ID); got != 4 {
		t.Fatalf("project events = %d, want create and update pairs", got)
	}
	var events []struct {
		Topic        string
		PartitionKey string
		Payload      []byte
	}
	if err := database.WithContext(ctx).Raw(`
		SELECT topic, partition_key, payload FROM infra.outbox
		WHERE partition_key = ? AND payload #>> '{data,request_id}' = ?
	`, project.ID.String(), input.RequestID).Scan(&events).Error; err != nil {
		t.Fatalf("read update audit: %v", err)
	}
	if len(events) != 1 || events[0].Topic != "lanverse.audit.recorded.v1" ||
		events[0].PartitionKey != project.ID.String() ||
		bytes.Contains(events[0].Payload, []byte("新名称")) ||
		bytes.Contains(events[0].Payload, []byte(*input.Description)) {
		t.Fatalf("unsafe or missing update audit: %+v", events)
	}
	audit, err := auditapp.NewRecordedActionParser().Parse(inbox.Record{
		Topic: events[0].Topic, Key: []byte(events[0].PartitionKey), Value: events[0].Payload,
	})
	if err != nil || audit.Action != "project.updated" || audit.RequestID != input.RequestID ||
		audit.ProjectID == nil || *audit.ProjectID != project.ID {
		t.Fatalf("invalid committed audit: %+v err=%v", audit, err)
	}
	var changed []struct{ Payload []byte }
	if err := database.WithContext(ctx).Raw(`
		SELECT payload FROM infra.outbox
		WHERE partition_key = ? AND topic = 'lanverse.workspace.project_changed.v1'
		  AND payload #>> '{data,change}' = 'updated'
	`, project.ID.String()).Scan(&changed).Error; err != nil || len(changed) != 1 {
		t.Fatalf("updated change event = %+v: %v", changed, err)
	}
	var body struct {
		ProjectID uuid.UUID `json:"project_id"`
		Aggregate struct {
			Revision int64 `json:"revision"`
		} `json:"aggregate"`
		Data struct {
			Change string `json:"change"`
		} `json:"data"`
	}
	if err := json.Unmarshal(changed[0].Payload, &body); err != nil ||
		body.ProjectID != project.ID || body.Aggregate.Revision != 2 || body.Data.Change != "updated" ||
		bytes.Contains(changed[0].Payload, []byte("新名称")) {
		t.Fatalf("invalid committed change event: %+v err=%v", body, err)
	}
	// A later patch may clear a preset without changing the immutable style.
	clearedPresetID := uuid.Nil
	description := "只改说明"
	cleared, err := workspaceapp.NewUpdateProjectCommand(pgworkspace.NewStore(database), time.Now).Execute(ctx, actor, workspaceapp.UpdateProjectInput{
		ProjectID: project.ID, ExpectedRevision: 2, StylePresetID: &clearedPresetID,
		Description: &description, RequestID: uuid.NewString(),
	})
	if err != nil || cleared.StylePresetID != nil || cleared.Revision != 3 {
		t.Fatalf("clear preset result = %+v: %v", cleared, err)
	}
	if row := projectRowForUpdate(ctx, t, database, project.ID); row.StylePresetID != nil || row.StyleType != project.StyleType {
		t.Fatalf("preset clear changed immutable style: %+v", row)
	}
	eventsBeforeNoop := projectUpdateEventCount(ctx, t, database, project.ID)
	sameName := "  新名称  "
	unchanged, err := workspaceapp.NewUpdateProjectCommand(pgworkspace.NewStore(database), time.Now).Execute(ctx, actor, workspaceapp.UpdateProjectInput{
		ProjectID: project.ID, ExpectedRevision: 3,
		Name: &sameName, Description: &description, StylePresetID: &clearedPresetID,
		RequestID: uuid.NewString(),
	})
	if err != nil || unchanged.Revision != 3 || unchanged.Name != "新名称" {
		t.Fatalf("no-change update result = %+v: %v", unchanged, err)
	}
	if row := projectRowForUpdate(ctx, t, database, project.ID); row.Revision != 3 {
		t.Fatalf("no-change update advanced revision: %+v", row)
	}
	if got := projectUpdateEventCount(ctx, t, database, project.ID); got != eventsBeforeNoop {
		t.Fatalf("no-change update wrote %d events, want %d", got, eventsBeforeNoop)
	}
}

func TestUpdateProjectCommandRejectsStaleStateScopeAndPreset(t *testing.T) {
	ctx, database := workspaceMigrationDB(t)
	orgID := insertWorkspaceOrganization(ctx, t, database)
	otherOrgID := insertWorkspaceOrganization(ctx, t, database)
	actor := insertWorkspaceActor(ctx, t, database, orgID)
	otherActor := insertWorkspaceActor(ctx, t, database, otherOrgID)
	project := createProjectForUpdate(ctx, t, database, actor)
	command := workspaceapp.NewUpdateProjectCommand(pgworkspace.NewStore(database), time.Now)
	initialEvents := projectUpdateEventCount(ctx, t, database, project.ID)

	stale := updateProjectInput(project.ID)
	stale.ExpectedRevision = 2
	if _, err := command.Execute(ctx, actor, stale); !errors.Is(err, domain.ErrProjectRevisionConflict) {
		t.Fatalf("stale revision result = %v", err)
	}
	if _, err := command.Execute(ctx, otherActor, updateProjectInput(project.ID)); !errors.Is(err, pgworkspace.ErrProjectNotFound) {
		t.Fatalf("cross-organization update result = %v", err)
	}
	forged := otherActor
	forged.OrgID = actor.OrgID
	if _, err := command.Execute(ctx, forged, updateProjectInput(project.ID)); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatalf("forged organization update result = %v", err)
	}
	for _, tc := range []struct {
		name   string
		preset uuid.UUID
	}{
		{"missing", uuid.New()},
		{"other organization", insertStylePreset(ctx, t, database, otherOrgID, "", false)},
		{"deleted", insertStylePreset(ctx, t, database, orgID, "", true)},
		{"wrong style", insertRealisticPreset(ctx, t, database, orgID)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := updateProjectInput(project.ID)
			input.StylePresetID = &tc.preset
			if _, err := command.Execute(ctx, actor, input); err == nil {
				t.Fatal("invalid preset accepted")
			}
		})
	}
	otherProject := createProjectForUpdate(ctx, t, database, actor)
	otherProjectPreset := insertStylePreset(ctx, t, database, orgID, otherProject.ID.String(), false)
	projectScoped := updateProjectInput(project.ID)
	projectScoped.StylePresetID = &otherProjectPreset
	if _, err := command.Execute(ctx, actor, projectScoped); err == nil {
		t.Fatal("other project's preset accepted")
	}
	if got := projectUpdateEventCount(ctx, t, database, project.ID); got != initialEvents {
		t.Fatalf("rejected updates wrote %d events, want %d", got, initialEvents)
	}
	if row := projectRowForUpdate(ctx, t, database, project.ID); row.Revision != 1 || row.Name != project.Name {
		t.Fatalf("rejected update changed project: %+v", row)
	}

	if err := database.WithContext(ctx).Exec(`UPDATE identity."user" SET status = 'disabled' WHERE id = ?::uuid`, actor.ID.String()).Error; err != nil {
		t.Fatalf("disable actor: %v", err)
	}
	if _, err := command.Execute(ctx, actor, updateProjectInput(project.ID)); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatalf("revoked actor update result = %v", err)
	}
	if err := database.WithContext(ctx).Exec(`UPDATE identity."user" SET status = 'active' WHERE id = ?::uuid`, actor.ID.String()).Error; err != nil {
		t.Fatalf("restore actor: %v", err)
	}
	if err := database.WithContext(ctx).Exec(`UPDATE workspace.project SET status = 'archived' WHERE id = ?::uuid`, project.ID.String()).Error; err != nil {
		t.Fatalf("archive fixture: %v", err)
	}
	if _, err := command.Execute(ctx, actor, updateProjectInput(project.ID)); !errors.Is(err, domain.ErrProjectStateConflict) {
		t.Fatalf("archived project update result = %v", err)
	}
	if err := database.WithContext(ctx).Exec(`UPDATE workspace.project SET status = 'active', is_delete = true WHERE id = ?::uuid`, project.ID.String()).Error; err != nil {
		t.Fatalf("recycle fixture: %v", err)
	}
	if _, err := command.Execute(ctx, actor, updateProjectInput(project.ID)); err == nil {
		t.Fatal("recycled project update accepted")
	}
	if got := projectUpdateEventCount(ctx, t, database, project.ID); got != initialEvents {
		t.Fatalf("rejected states wrote %d events, want %d", got, initialEvents)
	}
}

func TestUpdateProjectCommandRollsBackWhenAuditInsertFails(t *testing.T) {
	ctx, database := workspaceMigrationDB(t)
	orgID := insertWorkspaceOrganization(ctx, t, database)
	actor := insertWorkspaceActor(ctx, t, database, orgID)
	project := createProjectForUpdate(ctx, t, database, actor)
	input := updateProjectInput(project.ID)
	input.StylePresetID = nil
	initialEvents := projectUpdateEventCount(ctx, t, database, project.ID)
	identifier := strings.ReplaceAll(uuid.NewString(), "-", "")
	function := "reject_update_audit_" + identifier
	trigger := "reject_update_audit_before_insert_" + identifier
	functionSQL := fmt.Sprintf(`
		CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
		  IF NEW.topic = 'lanverse.audit.recorded.v1'
		     AND NEW.payload #>> '{data,request_id}' = '%s'
		  THEN RAISE EXCEPTION 'reject project update audit'; END IF;
		  RETURN NEW;
		END $$
	`, function, input.RequestID)
	if err := database.WithContext(ctx).Exec(functionSQL).Error; err != nil {
		t.Fatalf("create scoped audit failure function: %v", err)
	}
	t.Cleanup(func() { _ = database.Exec("DROP FUNCTION IF EXISTS " + function + "()").Error })
	triggerSQL := fmt.Sprintf(`
		CREATE TRIGGER %s BEFORE INSERT ON infra.outbox
		FOR EACH ROW EXECUTE FUNCTION %s()
	`, trigger, function)
	if err := database.WithContext(ctx).Exec(triggerSQL).Error; err != nil {
		t.Fatalf("create scoped audit failure trigger: %v", err)
	}
	t.Cleanup(func() { _ = database.Exec("DROP TRIGGER IF EXISTS " + trigger + " ON infra.outbox").Error })

	if _, err := workspaceapp.NewUpdateProjectCommand(pgworkspace.NewStore(database), time.Now).Execute(ctx, actor, input); err == nil {
		t.Fatal("audit insert failure committed update")
	}
	if row := projectRowForUpdate(ctx, t, database, project.ID); row.Revision != 1 ||
		row.Name != project.Name || row.Description != project.Description || row.AllowOverseasModels {
		t.Fatalf("audit failure did not roll back project: %+v", row)
	}
	if got := projectUpdateEventCount(ctx, t, database, project.ID); got != initialEvents {
		t.Fatalf("audit failure left %d events, want %d", got, initialEvents)
	}
}

func TestUpdateProjectStoreRejectsTamperedEventsBeforeWriting(t *testing.T) {
	ctx, database := workspaceMigrationDB(t)
	orgID := insertWorkspaceOrganization(ctx, t, database)
	actor := insertWorkspaceActor(ctx, t, database, orgID)
	project := createProjectForUpdate(ctx, t, database, actor)
	initialEvents := projectUpdateEventCount(ctx, t, database, project.ID)
	for _, tc := range []struct {
		name   string
		change func(*testing.T, []identityapp.OutboxEvent)
	}{
		{"audit free text", func(t *testing.T, events []identityapp.OutboxEvent) {
			t.Helper()
			var body map[string]any
			if err := json.Unmarshal(events[1].Payload, &body); err != nil {
				t.Fatal(err)
			}
			body["data"].(map[string]any)["after"].(map[string]any)["name"] = "project name must not leave the database"
			payload, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			events[1].Payload = payload
		}},
		{"wrong change revision", func(t *testing.T, events []identityapp.OutboxEvent) {
			t.Helper()
			var body map[string]any
			if err := json.Unmarshal(events[0].Payload, &body); err != nil {
				t.Fatal(err)
			}
			body["aggregate"].(map[string]any)["revision"] = float64(99)
			payload, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			events[0].Payload = payload
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			capture := &updateProjectCommandStore{project: project}
			input := updateProjectInput(project.ID)
			input.StylePresetID = nil
			if _, err := workspaceapp.NewUpdateProjectCommand(capture, time.Now).Execute(ctx, actor, input); err != nil {
				t.Fatalf("build valid project update: %v", err)
			}
			tc.change(t, capture.events)
			_, err := pgworkspace.NewStore(database).UpdateProjectWithEvents(ctx, actor, capture.before, capture.after, capture.events)
			if !errors.Is(err, workspaceapp.ErrInvalidUpdateProject) {
				t.Fatalf("tampered update result = %v, want invalid command", err)
			}
			if row := projectRowForUpdate(ctx, t, database, project.ID); row.Revision != 1 || row.Name != project.Name {
				t.Fatalf("tampered event changed project: %+v", row)
			}
			if got := projectUpdateEventCount(ctx, t, database, project.ID); got != initialEvents {
				t.Fatalf("tampered event wrote %d Outbox rows, want %d", got, initialEvents)
			}
		})
	}
}

func TestUpdateProjectCommandConcurrentSameRevisionHasOneWinner(t *testing.T) {
	ctx, database := workspaceMigrationDB(t)
	orgID := insertWorkspaceOrganization(ctx, t, database)
	actor := insertWorkspaceActor(ctx, t, database, orgID)
	project := createProjectForUpdate(ctx, t, database, actor)
	command := workspaceapp.NewUpdateProjectCommand(pgworkspace.NewStore(database), time.Now)
	start := make(chan struct{})
	results := make(chan error, 2)
	var wait sync.WaitGroup
	for _, name := range []string{"并发一", "并发二"} {
		wait.Add(1)
		go func(name string) {
			defer wait.Done()
			<-start
			input := workspaceapp.UpdateProjectInput{
				ProjectID: project.ID, ExpectedRevision: 1,
				Name: &name, RequestID: uuid.NewString(),
			}
			_, err := command.Execute(ctx, actor, input)
			results <- err
		}(name)
	}
	close(start)
	wait.Wait()
	close(results)
	wins, conflicts := 0, 0
	for err := range results {
		switch {
		case err == nil:
			wins++
		case errors.Is(err, domain.ErrProjectRevisionConflict):
			conflicts++
		default:
			t.Fatalf("concurrent update returned unexpected error: %v", err)
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatalf("concurrent result: wins=%d conflicts=%d", wins, conflicts)
	}
	if row := projectRowForUpdate(ctx, t, database, project.ID); row.Revision != 2 ||
		(row.Name != "并发一" && row.Name != "并发二") {
		t.Fatalf("concurrent update left wrong revision or name: %+v", row)
	}
	if got := projectUpdateEventCount(ctx, t, database, project.ID); got != 4 {
		t.Fatalf("concurrent updates emitted %d events, want one update pair", got)
	}
}
