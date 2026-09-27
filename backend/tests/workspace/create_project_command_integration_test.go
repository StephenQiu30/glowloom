package workspace_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	auditapp "github.com/StephenQiu30/lanverse/backend/internal/audit/application"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
	pgworkspace "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
)

func projectCreateIntegrationInput(presetID uuid.UUID) workspaceapp.CreateProjectInput {
	return workspaceapp.CreateProjectInput{
		Name: "逆光", Description: "项目说明仅留在项目表",
		AspectRatio: "16:9", StyleType: "stylized", StyleSubtype: "guofeng_xianxia",
		StylePresetID: presetID, RequestID: uuid.NewString(),
	}
}

func countProjectsNamed(ctx context.Context, t *testing.T, database *gorm.DB, orgID uuid.UUID, name string) int64 {
	t.Helper()
	var count int64
	if err := database.WithContext(ctx).Raw(`
		SELECT count(*) FROM workspace.project WHERE org_id = ?::uuid AND name = ?
	`, orgID.String(), name).Scan(&count).Error; err != nil {
		t.Fatalf("count named projects: %v", err)
	}
	return count
}

func TestCreateProjectCommandCommitsProjectBudgetAndSafeEvents(t *testing.T) {
	ctx, database := workspaceMigrationDB(t)
	orgID := insertWorkspaceOrganization(ctx, t, database)
	actor := insertWorkspaceActor(ctx, t, database, orgID)
	presetID := insertStylePreset(ctx, t, database, orgID, "", false)
	command := workspaceapp.NewCreateProjectCommand(pgworkspace.NewStore(database), time.Now)
	input := projectCreateIntegrationInput(presetID)

	created, err := command.Execute(ctx, actor, input)
	if err != nil || created.ID == uuid.Nil || created.OrgID != actor.OrgID ||
		created.Name != input.Name || created.Resolution != "1080p" ||
		created.AllowOverseasModels || created.Status != "active" || created.Revision != 1 ||
		created.CreateTime.IsZero() || created.UpdateTime.IsZero() {
		t.Fatalf("create project result: id=%s revision=%d err=%v", created.ID, created.Revision, err)
	}
	var row struct {
		OrgID               uuid.UUID
		StylePresetID       uuid.UUID
		AspectRatio         string
		StyleType           string
		StyleSubtype        string
		Resolution          string
		Status              string
		AllowOverseasModels bool
		Revision            int64
		LimitMicros         int64
		ReservedMicros      int64
		SettledMicros       int64
		IsOverrun           bool
		BudgetRevision      int64
	}
	result := database.WithContext(ctx).Raw(`
		SELECT p.org_id, p.style_preset_id, p.aspect_ratio, p.style_type, p.style_subtype,
		       p.resolution, p.status, p.allow_overseas_models, p.revision,
		       b.limit_micros, b.reserved_micros, b.settled_micros, b.is_overrun,
		       b.revision AS budget_revision
		FROM workspace.project AS p
		JOIN billing.budget AS b ON b.project_id = p.id
		WHERE p.id = ?::uuid
	`, created.ID.String()).Scan(&row)
	if result.Error != nil || result.RowsAffected != 1 {
		t.Fatalf("read created project and budget: rows=%d err=%v", result.RowsAffected, result.Error)
	}
	if row.OrgID != actor.OrgID || row.StylePresetID != presetID ||
		row.AspectRatio != "16:9" || row.StyleType != "stylized" || row.StyleSubtype != "guofeng_xianxia" ||
		row.Resolution != "1080p" || row.Status != "active" || row.AllowOverseasModels ||
		row.Revision != 1 || row.LimitMicros != 0 || row.ReservedMicros != 0 ||
		row.SettledMicros != 0 || row.IsOverrun || row.BudgetRevision != 1 {
		t.Fatalf("created project or zero budget did not match the command: %+v", row)
	}

	var events []struct {
		Topic        string
		PartitionKey string
		Payload      []byte
	}
	if err := database.WithContext(ctx).Raw(`
		SELECT topic, partition_key, payload FROM infra.outbox
		WHERE partition_key = ? ORDER BY topic
	`, created.ID.String()).Scan(&events).Error; err != nil {
		t.Fatalf("read project events: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("created project events = %d, want audit and change", len(events))
	}
	seen := make(map[string]bool, len(events))
	for _, event := range events {
		if event.PartitionKey != created.ID.String() || !json.Valid(event.Payload) ||
			bytes.Contains(event.Payload, []byte(input.Name)) ||
			bytes.Contains(event.Payload, []byte(input.Description)) {
			t.Fatal("project event has invalid routing, JSON, or free-form project text")
		}
		if seen[event.Topic] {
			t.Fatalf("duplicate project event topic %q", event.Topic)
		}
		seen[event.Topic] = true
		switch event.Topic {
		case "lanverse.audit.recorded.v1":
			audit, parseErr := auditapp.NewRecordedActionParser().Parse(inbox.Record{
				Topic: event.Topic, Key: []byte(event.PartitionKey), Value: event.Payload,
			})
			if parseErr != nil || audit.Action != "project.created" ||
				audit.OrgID != actor.OrgID || audit.ProjectID == nil || *audit.ProjectID != created.ID ||
				audit.ActorID == nil || *audit.ActorID != actor.ID ||
				audit.ObjectType != "project" || audit.ObjectID != created.ID.String() ||
				audit.RequestID != input.RequestID {
				t.Fatalf("project audit did not pass the consumer contract: action=%q object=%q err=%v", audit.Action, audit.ObjectID, parseErr)
			}
			var after map[string]json.RawMessage
			if err := json.Unmarshal(audit.After, &after); err != nil ||
				len(after) != 6 || after["aspect_ratio"] == nil || after["style_type"] == nil ||
				after["style_subtype"] == nil || after["style_preset_id"] == nil ||
				after["status"] == nil || after["revision"] == nil {
				t.Fatalf("project audit did not contain its bounded specification summary: %v", err)
			}
			var summary struct {
				AspectRatio  string    `json:"aspect_ratio"`
				StyleType    string    `json:"style_type"`
				StyleSubtype string    `json:"style_subtype"`
				StylePreset  uuid.UUID `json:"style_preset_id"`
				Status       string    `json:"status"`
				Revision     int64     `json:"revision"`
			}
			if err := json.Unmarshal(audit.After, &summary); err != nil ||
				summary.AspectRatio != input.AspectRatio || summary.StyleType != input.StyleType ||
				summary.StyleSubtype != input.StyleSubtype || summary.StylePreset != input.StylePresetID ||
				summary.Status != "active" || summary.Revision != 1 {
				t.Fatalf("project audit summary does not match committed specification: %v", err)
			}
		case "lanverse.workspace.project_changed.v1":
			var envelope struct {
				EventID   uuid.UUID `json:"event_id"`
				EventType string    `json:"event_type"`
				ProjectID uuid.UUID `json:"project_id"`
				Aggregate struct {
					Type string    `json:"type"`
					ID   uuid.UUID `json:"id"`
				} `json:"aggregate"`
			}
			if err := json.Unmarshal(event.Payload, &envelope); err != nil ||
				envelope.EventID == uuid.Nil || envelope.EventType != event.Topic ||
				envelope.ProjectID != created.ID || envelope.Aggregate.Type != "project" ||
				envelope.Aggregate.ID != created.ID {
				t.Fatalf("invalid project change envelope: %v", err)
			}
		default:
			t.Fatalf("unexpected project event topic %q", event.Topic)
		}
	}
	if !seen["lanverse.audit.recorded.v1"] || !seen["lanverse.workspace.project_changed.v1"] {
		t.Fatalf("missing required project events: %+v", seen)
	}
}

func TestCreateProjectCommandRejectsUnusablePresetWithoutWrites(t *testing.T) {
	ctx, database := workspaceMigrationDB(t)
	orgID := insertWorkspaceOrganization(ctx, t, database)
	otherOrgID := insertWorkspaceOrganization(ctx, t, database)
	actor := insertWorkspaceActor(ctx, t, database, orgID)
	otherProjectID := insertWorkspaceProject(ctx, t, database, orgID, "16:9", "stylized", "guofeng_xianxia")
	command := workspaceapp.NewCreateProjectCommand(pgworkspace.NewStore(database), time.Now)
	for _, test := range []struct {
		name     string
		presetID uuid.UUID
	}{
		{"missing", uuid.New()},
		{"other organization", insertStylePreset(ctx, t, database, otherOrgID, "", false)},
		{"deleted", insertStylePreset(ctx, t, database, orgID, "", true)},
		{"other project", insertStylePreset(ctx, t, database, orgID, otherProjectID, false)},
		{"wrong style", insertRealisticPreset(ctx, t, database, orgID)},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := projectCreateIntegrationInput(test.presetID)
			input.Name = "无效预设-" + uuid.NewString()[:8]
			if _, err := command.Execute(ctx, actor, input); err == nil {
				t.Fatal("unusable preset accepted")
			}
			if count := countProjectsNamed(ctx, t, database, actor.OrgID, input.Name); count != 0 {
				t.Fatalf("rejected preset left %d projects", count)
			}
			var events int64
			if err := database.WithContext(ctx).Raw(`
				SELECT count(*) FROM infra.outbox WHERE payload #>> '{data,request_id}' = ?
			`, input.RequestID).Scan(&events).Error; err != nil || events != 0 {
				t.Fatalf("rejected preset left %d audit events: %v", events, err)
			}
		})
	}
}

func TestCreateProjectCommandRechecksCurrentActorAndOrganization(t *testing.T) {
	ctx, database := workspaceMigrationDB(t)
	orgID := insertWorkspaceOrganization(ctx, t, database)
	otherOrgID := insertWorkspaceOrganization(ctx, t, database)
	actor := insertWorkspaceActor(ctx, t, database, orgID)
	otherActor := insertWorkspaceActor(ctx, t, database, otherOrgID)
	command := workspaceapp.NewCreateProjectCommand(pgworkspace.NewStore(database), time.Now)

	forged := otherActor
	forged.OrgID = actor.OrgID
	input := projectCreateIntegrationInput(uuid.Nil)
	input.Name = "伪造组织"
	if _, err := command.Execute(ctx, forged, input); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatalf("forged organization result = %v, want forbidden", err)
	}
	if count := countProjectsNamed(ctx, t, database, actor.OrgID, input.Name); count != 0 {
		t.Fatalf("forged organization left %d projects", count)
	}

	for _, test := range []struct {
		name       string
		change     string
		restore    string
		identifier uuid.UUID
	}{
		{"password change required", `UPDATE identity."user" SET must_change_password = true WHERE id = ?::uuid`, `UPDATE identity."user" SET must_change_password = false WHERE id = ?::uuid`, actor.ID},
		{"disabled actor", `UPDATE identity."user" SET status = 'disabled' WHERE id = ?::uuid`, `UPDATE identity."user" SET status = 'active' WHERE id = ?::uuid`, actor.ID},
		{"disabled organization", `UPDATE workspace.organization SET status = 'disabled' WHERE id = ?::uuid`, `UPDATE workspace.organization SET status = 'active' WHERE id = ?::uuid`, actor.OrgID},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := database.WithContext(ctx).Exec(test.change, test.identifier.String()).Error; err != nil {
				t.Fatalf("revoke actor access: %v", err)
			}
			input := projectCreateIntegrationInput(uuid.Nil)
			input.Name = "撤权-" + uuid.NewString()[:8]
			if _, err := command.Execute(ctx, actor, input); !errors.Is(err, identityapp.ErrForbidden) {
				t.Fatalf("revoked actor result = %v, want forbidden", err)
			}
			if count := countProjectsNamed(ctx, t, database, actor.OrgID, input.Name); count != 0 {
				t.Fatalf("revoked actor left %d projects", count)
			}
			if err := database.WithContext(ctx).Exec(test.restore, test.identifier.String()).Error; err != nil {
				t.Fatalf("restore actor access: %v", err)
			}
		})
	}
}

func TestCreateProjectCommandRollsBackWhenAuditOutboxInsertFails(t *testing.T) {
	ctx, database := workspaceMigrationDB(t)
	orgID := insertWorkspaceOrganization(ctx, t, database)
	actor := insertWorkspaceActor(ctx, t, database, orgID)
	input := projectCreateIntegrationInput(uuid.Nil)
	input.Name = "审计回滚-" + uuid.NewString()[:8]
	var budgetsBefore int64
	if err := database.WithContext(ctx).Raw(`SELECT count(*) FROM billing.budget`).Scan(&budgetsBefore).Error; err != nil {
		t.Fatalf("count budgets before audit failure: %v", err)
	}
	identifier := strings.ReplaceAll(uuid.NewString(), "-", "")
	function := "reject_project_audit_" + identifier
	trigger := "reject_project_audit_before_insert_" + identifier
	functionSQL := fmt.Sprintf(`
		CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
		  IF NEW.topic = 'lanverse.audit.recorded.v1'
		     AND NEW.payload #>> '{data,request_id}' = '%s'
		  THEN RAISE EXCEPTION 'reject project audit'; END IF;
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

	command := workspaceapp.NewCreateProjectCommand(pgworkspace.NewStore(database), time.Now)
	if _, err := command.Execute(ctx, actor, input); err == nil {
		t.Fatal("audit insert failure committed project")
	}
	if count := countProjectsNamed(ctx, t, database, actor.OrgID, input.Name); count != 0 {
		t.Fatalf("audit insert failure left %d projects", count)
	}
	var budgets, events int64
	if err := database.WithContext(ctx).Raw(`
		SELECT count(*) FROM billing.budget
	`).Scan(&budgets).Error; err != nil || budgets != budgetsBefore {
		t.Fatalf("audit insert failure changed budget count from %d to %d: %v", budgetsBefore, budgets, err)
	}
	if err := database.WithContext(ctx).Raw(`
		SELECT count(*) FROM infra.outbox WHERE payload #>> '{data,request_id}' = ?
	`, input.RequestID).Scan(&events).Error; err != nil || events != 0 {
		t.Fatalf("audit insert failure left %d events: %v", events, err)
	}
}

func TestCreateProjectStoreRejectsTamperedEventsBeforeWriting(t *testing.T) {
	ctx, database := workspaceMigrationDB(t)
	orgID := insertWorkspaceOrganization(ctx, t, database)
	actor := insertWorkspaceActor(ctx, t, database, orgID)
	for _, test := range []struct {
		name   string
		change func(*testing.T, []identityapp.OutboxEvent)
	}{
		{"wrong partition", func(_ *testing.T, events []identityapp.OutboxEvent) {
			events[0].PartitionKey = actor.OrgID.String()
		}},
		{"duplicate event ID", func(_ *testing.T, events []identityapp.OutboxEvent) {
			events[1].ID = events[0].ID
		}},
		{"free-form audit text", func(t *testing.T, events []identityapp.OutboxEvent) {
			t.Helper()
			var body map[string]any
			if err := json.Unmarshal(events[1].Payload, &body); err != nil {
				t.Fatal(err)
			}
			data := body["data"].(map[string]any)
			after := data["after"].(map[string]any)
			after["description"] = "must not leave the process"
			payload, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			events[1].Payload = payload
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			capture := &createProjectCommandStore{}
			input := projectCreateIntegrationInput(uuid.Nil)
			input.Name = "拒绝污染-" + uuid.NewString()[:8]
			_, err := workspaceapp.NewCreateProjectCommand(capture, time.Now).Execute(ctx, actor, input)
			if err != nil {
				t.Fatalf("build valid project events: %v", err)
			}
			test.change(t, capture.events)
			_, err = pgworkspace.NewStore(database).CreateProjectWithEvents(ctx, actor, capture.project, capture.events)
			if !errors.Is(err, workspaceapp.ErrInvalidCreateProject) {
				t.Fatalf("tampered event result = %v, want invalid command", err)
			}
			projectCount, budgetCount := projectAndBudgetCount(ctx, t, database, capture.project.ID)
			if projectCount != 0 || budgetCount != 0 {
				t.Fatalf("tampered event wrote project/budget rows = %d/%d", projectCount, budgetCount)
			}
		})
	}
}
