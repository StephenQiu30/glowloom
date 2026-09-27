package workspace_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	pgworkspace "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

func insertWorkspaceActor(ctx context.Context, t *testing.T, database *gorm.DB, orgID string) identityapp.Principal {
	t.Helper()
	id := uuid.New()
	if err := database.WithContext(ctx).Exec(`
		INSERT INTO identity."user"
		  (id, org_id, login_name, display_name, role, password_hash, must_change_password)
		VALUES (?::uuid, ?::uuid, ?, 'Workspace Producer', 'producer', 'test-hash', false)
	`, id.String(), orgID, "workspace-"+id.String()).Error; err != nil {
		t.Fatalf("insert workspace actor: %v", err)
	}
	parsedOrgID, err := uuid.Parse(orgID)
	if err != nil {
		t.Fatalf("parse organization ID: %v", err)
	}
	return identityapp.Principal{ID: id, OrgID: parsedOrgID, Role: identitydomain.RoleProducer}
}

func projectForActor(actor identityapp.Principal) domain.Project {
	return domain.Project{
		ID: uuid.New(), OrgID: actor.OrgID, Name: "逆光", AspectRatio: "16:9",
		StyleType: "stylized", StyleSubtype: "guofeng_xianxia",
		Resolution: "1080p", Status: "active", Revision: 1,
	}
}

func createWorkspaceProjectInStore(ctx context.Context, store *pgworkspace.Store, actor identityapp.Principal, project domain.Project) error {
	changedID, auditID := uuid.New(), uuid.New()
	occurredAt := time.Now().UTC()
	changed, err := json.Marshal(map[string]any{
		"event_id": changedID, "event_type": "lanverse.workspace.project_changed.v1",
		"occurred_at": occurredAt, "org_id": actor.OrgID, "project_id": project.ID,
		"actor":     map[string]any{"kind": "user", "id": actor.ID},
		"aggregate": map[string]any{"type": "project", "id": project.ID, "revision": project.Revision},
		"data":      map[string]any{"change": "created"},
	})
	if err != nil {
		return fmt.Errorf("encode project change fixture: %w", err)
	}
	after := map[string]any{
		"aspect_ratio": project.AspectRatio, "style_type": project.StyleType,
		"status": project.Status, "revision": project.Revision,
	}
	if project.StyleSubtype != "" {
		after["style_subtype"] = project.StyleSubtype
	}
	if project.StylePresetID != uuid.Nil {
		after["style_preset_id"] = project.StylePresetID
	}
	audit, err := json.Marshal(map[string]any{
		"event_id": auditID, "event_type": "lanverse.audit.recorded.v1",
		"occurred_at": occurredAt, "org_id": actor.OrgID, "project_id": project.ID,
		"actor":     map[string]any{"kind": "user", "id": actor.ID},
		"aggregate": map[string]any{"type": "audit", "id": auditID},
		"data": map[string]any{
			"action": "project.created", "object": map[string]any{"type": "project", "id": project.ID},
			"request_id": uuid.NewString(), "after": after,
		},
	})
	if err != nil {
		return fmt.Errorf("encode project audit fixture: %w", err)
	}
	key := project.ID.String()
	_, err = store.CreateProjectWithEvents(ctx, actor, project, []identityapp.OutboxEvent{
		{ID: changedID, Topic: "lanverse.workspace.project_changed.v1", PartitionKey: key, Payload: changed},
		{ID: auditID, Topic: "lanverse.audit.recorded.v1", PartitionKey: key, Payload: audit},
	})
	return err
}

func insertStylePreset(ctx context.Context, t *testing.T, database *gorm.DB, orgID, projectID string, deleted bool) uuid.UUID {
	t.Helper()
	id := uuid.New()
	var scopedProject any
	if projectID != "" {
		scopedProject = projectID
	}
	if err := database.WithContext(ctx).Exec(`
		INSERT INTO workspace.style_preset
		  (id, org_id, project_id, name, style_type, style_subtype, is_delete)
		VALUES (?::uuid, ?::uuid, ?::uuid, '国风仙侠', 'stylized', 'guofeng_xianxia', ?)
	`, id.String(), orgID, scopedProject, deleted).Error; err != nil {
		t.Fatalf("insert style preset: %v", err)
	}
	return id
}

func insertRealisticPreset(ctx context.Context, t *testing.T, database *gorm.DB, orgID string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if err := database.WithContext(ctx).Exec(`
		INSERT INTO workspace.style_preset
		  (id, org_id, name, style_type)
		VALUES (?::uuid, ?::uuid, '写实', 'realistic')
	`, id.String(), orgID).Error; err != nil {
		t.Fatalf("insert realistic style preset: %v", err)
	}
	return id
}

func projectAndBudgetCount(ctx context.Context, t *testing.T, database *gorm.DB, projectID uuid.UUID) (int64, int64) {
	t.Helper()
	var projectCount, budgetCount int64
	if err := database.WithContext(ctx).Raw(`SELECT count(*) FROM workspace.project WHERE id = ?::uuid`, projectID.String()).Scan(&projectCount).Error; err != nil {
		t.Fatalf("count project: %v", err)
	}
	if err := database.WithContext(ctx).Raw(`SELECT count(*) FROM billing.budget WHERE project_id = ?::uuid`, projectID.String()).Scan(&budgetCount).Error; err != nil {
		t.Fatalf("count budget: %v", err)
	}
	return projectCount, budgetCount
}

func TestWorkspaceStoreCreateProjectAndZeroBudgetAtomically(t *testing.T) {
	ctx, database := workspaceMigrationDB(t)
	orgID := insertWorkspaceOrganization(ctx, t, database)
	actor := insertWorkspaceActor(ctx, t, database, orgID)
	store := pgworkspace.NewStore(database)

	project := projectForActor(actor)
	project.StylePresetID = insertStylePreset(ctx, t, database, orgID, "", false)
	if err := createWorkspaceProjectInStore(ctx, store, actor, project); err != nil {
		t.Fatalf("create project with organization preset: %v", err)
	}
	projectCount, budgetCount := projectAndBudgetCount(ctx, t, database, project.ID)
	if projectCount != 1 || budgetCount != 1 {
		t.Fatalf("created project/budget rows = %d/%d, want 1/1", projectCount, budgetCount)
	}
	var row struct {
		OrgID               uuid.UUID
		StylePresetID       uuid.UUID
		Resolution          string
		AllowOverseasModels bool
		Status              string
		Revision            int64
		LimitMicros         int64
		ReservedMicros      int64
		SettledMicros       int64
		IsOverrun           bool
		BudgetRevision      int64
	}
	result := database.WithContext(ctx).Raw(`
		SELECT p.org_id, p.style_preset_id, p.resolution, p.allow_overseas_models,
		       p.status, p.revision, b.limit_micros, b.reserved_micros,
		       b.settled_micros, b.is_overrun, b.revision AS budget_revision
		FROM workspace.project AS p
		JOIN billing.budget AS b ON b.project_id = p.id
		WHERE p.id = ?::uuid
	`, project.ID.String()).Scan(&row)
	if result.Error != nil || result.RowsAffected != 1 {
		t.Fatalf("read created project and budget: rows=%d err=%v", result.RowsAffected, result.Error)
	}
	if row.OrgID != actor.OrgID || row.StylePresetID != project.StylePresetID ||
		row.Resolution != "1080p" || row.AllowOverseasModels || row.Status != "active" || row.Revision != 1 ||
		row.LimitMicros != 0 || row.ReservedMicros != 0 || row.SettledMicros != 0 || row.IsOverrun || row.BudgetRevision != 1 {
		t.Fatalf("created project and budget state = %+v", row)
	}

	// The budget table deliberately has no project FK yet. A colliding budget row
	// forces failure after a valid project insert; the project must roll back.
	rolledBack := projectForActor(actor)
	if err := database.WithContext(ctx).Exec(`
		INSERT INTO billing.budget (id, project_id, limit_micros)
		VALUES (?::uuid, ?::uuid, 0)
	`, uuid.NewString(), rolledBack.ID.String()).Error; err != nil {
		t.Fatalf("seed colliding budget: %v", err)
	}
	if err := createWorkspaceProjectInStore(ctx, store, actor, rolledBack); err == nil {
		t.Fatal("budget collision accepted during project creation")
	}
	projectCount, budgetCount = projectAndBudgetCount(ctx, t, database, rolledBack.ID)
	if projectCount != 0 || budgetCount != 1 {
		t.Fatalf("after budget collision project/budget rows = %d/%d, want 0/1", projectCount, budgetCount)
	}
}

func TestWorkspaceStoreRejectsUnusablePresetWithoutPartialProject(t *testing.T) {
	ctx, database := workspaceMigrationDB(t)
	orgID := insertWorkspaceOrganization(ctx, t, database)
	otherOrgID := insertWorkspaceOrganization(ctx, t, database)
	actor := insertWorkspaceActor(ctx, t, database, orgID)
	store := pgworkspace.NewStore(database)
	otherProjectID := insertWorkspaceProject(ctx, t, database, orgID, "16:9", "stylized", "guofeng_xianxia")

	for _, test := range []struct {
		name string
		id   uuid.UUID
	}{
		{name: "missing preset", id: uuid.New()},
		{name: "other organization", id: insertStylePreset(ctx, t, database, otherOrgID, "", false)},
		{name: "deleted preset", id: insertStylePreset(ctx, t, database, orgID, "", true)},
		{name: "different project", id: insertStylePreset(ctx, t, database, orgID, otherProjectID, false)},
		{name: "different style", id: insertRealisticPreset(ctx, t, database, orgID)},
	} {
		t.Run(test.name, func(t *testing.T) {
			project := projectForActor(actor)
			project.StylePresetID = test.id
			if err := createWorkspaceProjectInStore(ctx, store, actor, project); err == nil {
				t.Fatal("unusable style preset accepted")
			}
			projectCount, budgetCount := projectAndBudgetCount(ctx, t, database, project.ID)
			if projectCount != 0 || budgetCount != 0 {
				t.Fatalf("rejected preset left project/budget rows = %d/%d", projectCount, budgetCount)
			}
		})
	}
	invalid := projectForActor(actor)
	invalid.Name = " "
	if err := createWorkspaceProjectInStore(ctx, store, actor, invalid); !errors.Is(err, domain.ErrInvalidProject) {
		t.Fatalf("invalid project result = %v, want ErrInvalidProject", err)
	}
	projectCount, budgetCount := projectAndBudgetCount(ctx, t, database, invalid.ID)
	if projectCount != 0 || budgetCount != 0 {
		t.Fatalf("invalid project left project/budget rows = %d/%d", projectCount, budgetCount)
	}
}

func TestWorkspaceStoreReadsOnlyCurrentActorsOrganization(t *testing.T) {
	ctx, database := workspaceMigrationDB(t)
	orgID := insertWorkspaceOrganization(ctx, t, database)
	otherOrgID := insertWorkspaceOrganization(ctx, t, database)
	actor := insertWorkspaceActor(ctx, t, database, orgID)
	otherActor := insertWorkspaceActor(ctx, t, database, otherOrgID)
	store := pgworkspace.NewStore(database)
	project := projectForActor(actor)
	if err := createWorkspaceProjectInStore(ctx, store, actor, project); err != nil {
		t.Fatalf("create actor project: %v", err)
	}
	loaded, err := store.FindProject(ctx, actor, project.ID)
	if err != nil || loaded.ID != project.ID || loaded.OrgID != actor.OrgID {
		t.Fatalf("own project = %+v, err=%v", loaded, err)
	}
	budget, err := store.FindBudget(ctx, actor, project.ID)
	if err != nil || budget.ProjectID != project.ID || budget.LimitMicros != 0 {
		t.Fatalf("own budget = %+v, err=%v", budget, err)
	}
	if _, err := store.FindProject(ctx, otherActor, project.ID); err == nil {
		t.Fatal("other organization read project")
	}
	if _, err := store.FindBudget(ctx, otherActor, project.ID); err == nil {
		t.Fatal("other organization read budget")
	}
	if _, err := store.FindBudget(ctx, actor, uuid.Nil); err == nil {
		t.Fatal("budget query without project ID succeeded")
	}
	if _, err := store.FindProject(ctx, actor, uuid.Nil); err == nil {
		t.Fatal("project query without project ID succeeded")
	}
	forged := otherActor
	forged.OrgID = actor.OrgID
	if _, err := store.FindProject(ctx, forged, project.ID); err == nil {
		t.Fatal("actor with forged organization read project")
	}
	if _, err := store.FindBudget(ctx, forged, project.ID); err == nil {
		t.Fatal("actor with forged organization read budget")
	}
	forgedProject := projectForActor(forged)
	if err := createWorkspaceProjectInStore(ctx, store, forged, forgedProject); err == nil {
		t.Fatal("actor with forged organization created project")
	}
	projectCount, budgetCount := projectAndBudgetCount(ctx, t, database, forgedProject.ID)
	if projectCount != 0 || budgetCount != 0 {
		t.Fatalf("forged actor left project/budget rows = %d/%d", projectCount, budgetCount)
	}
	if err := database.WithContext(ctx).Exec(`
		UPDATE identity."user" SET must_change_password = true WHERE id = ?::uuid
	`, actor.ID.String()).Error; err != nil {
		t.Fatalf("require actor password change: %v", err)
	}
	if _, err := store.FindProject(ctx, actor, project.ID); err == nil {
		t.Fatal("actor required to change password read project")
	}
	if err := database.WithContext(ctx).Exec(`
		UPDATE identity."user" SET must_change_password = false WHERE id = ?::uuid
	`, actor.ID.String()).Error; err != nil {
		t.Fatalf("clear actor password change requirement: %v", err)
	}
	if err := database.WithContext(ctx).Exec(`
		UPDATE identity."user" SET status = 'disabled' WHERE id = ?::uuid
	`, actor.ID.String()).Error; err != nil {
		t.Fatalf("disable actor: %v", err)
	}
	if _, err := store.FindProject(ctx, actor, project.ID); err == nil {
		t.Fatal("disabled actor read project")
	}
	if _, err := store.FindBudget(ctx, actor, project.ID); err == nil {
		t.Fatal("disabled actor read budget")
	}
	if err := database.WithContext(ctx).Exec(`
		UPDATE identity."user" SET status = 'active', is_delete = true WHERE id = ?::uuid
	`, actor.ID.String()).Error; err != nil {
		t.Fatalf("soft-delete actor: %v", err)
	}
	if _, err := store.FindProject(ctx, actor, project.ID); err == nil {
		t.Fatal("deleted actor read project")
	}
	if err := database.WithContext(ctx).Exec(`
		UPDATE identity."user" SET is_delete = false WHERE id = ?::uuid
	`, actor.ID.String()).Error; err != nil {
		t.Fatalf("restore actor: %v", err)
	}
	if err := database.WithContext(ctx).Exec(`
		UPDATE workspace.organization SET status = 'disabled' WHERE id = ?::uuid
	`, actor.OrgID.String()).Error; err != nil {
		t.Fatalf("disable organization: %v", err)
	}
	if _, err := store.FindProject(ctx, actor, project.ID); err == nil {
		t.Fatal("disabled organization read project")
	}
	if _, err := store.FindBudget(ctx, actor, project.ID); err == nil {
		t.Fatal("disabled organization read budget")
	}
	newProject := projectForActor(actor)
	if err := createWorkspaceProjectInStore(ctx, store, actor, newProject); err == nil {
		t.Fatal("disabled organization created project")
	}
	projectCount, budgetCount = projectAndBudgetCount(ctx, t, database, newProject.ID)
	if projectCount != 0 || budgetCount != 0 {
		t.Fatalf("disabled organization left project/budget rows = %d/%d", projectCount, budgetCount)
	}
	if err := database.WithContext(ctx).Exec(`
		UPDATE workspace.organization SET status = 'active', is_delete = true WHERE id = ?::uuid
	`, actor.OrgID.String()).Error; err != nil {
		t.Fatalf("soft-delete organization: %v", err)
	}
	if _, err := store.FindProject(ctx, actor, project.ID); err == nil {
		t.Fatal("deleted organization read project")
	}
	if err := createWorkspaceProjectInStore(ctx, store, actor, projectForActor(actor)); err == nil {
		t.Fatal("deleted organization created project")
	}
}

func TestWorkspaceStoreListsOnlyVisibleStylePresets(t *testing.T) {
	ctx, database := workspaceMigrationDB(t)
	orgID := insertWorkspaceOrganization(ctx, t, database)
	otherOrgID := insertWorkspaceOrganization(ctx, t, database)
	actor := insertWorkspaceActor(ctx, t, database, orgID)
	store := pgworkspace.NewStore(database)
	project := projectForActor(actor)
	if err := createWorkspaceProjectInStore(ctx, store, actor, project); err != nil {
		t.Fatalf("create project: %v", err)
	}
	otherProject := projectForActor(actor)
	if err := createWorkspaceProjectInStore(ctx, store, actor, otherProject); err != nil {
		t.Fatalf("create other project: %v", err)
	}
	orgPresetID := insertStylePreset(ctx, t, database, orgID, "", false)
	projectPresetID := insertStylePreset(ctx, t, database, orgID, project.ID.String(), false)
	insertStylePreset(ctx, t, database, orgID, otherProject.ID.String(), false)
	insertStylePreset(ctx, t, database, orgID, "", true)
	insertStylePreset(ctx, t, database, otherOrgID, "", false)

	organizationPresets, err := store.ListStylePresets(ctx, actor, nil)
	if err != nil {
		t.Fatalf("list organization presets: %v", err)
	}
	if len(organizationPresets) != 1 || organizationPresets[0].ID != orgPresetID {
		t.Fatalf("organization presets = %+v, want only %s", organizationPresets, orgPresetID)
	}
	projectPresets, err := store.ListStylePresets(ctx, actor, &project.ID)
	if err != nil {
		t.Fatalf("list project presets: %v", err)
	}
	seen := make(map[uuid.UUID]bool, len(projectPresets))
	for _, preset := range projectPresets {
		seen[preset.ID] = true
	}
	if len(projectPresets) != 2 || !seen[orgPresetID] || !seen[projectPresetID] {
		t.Fatalf("project presets = %+v, want organization and matching project", projectPresets)
	}
	unknownProjectID := uuid.New()
	if _, err := store.ListStylePresets(ctx, actor, &unknownProjectID); err == nil {
		t.Fatal("unknown project ID accepted for style preset list")
	}
	if err := database.WithContext(ctx).Exec(`
		UPDATE workspace.project SET is_delete = true WHERE id = ?::uuid
	`, project.ID.String()).Error; err != nil {
		t.Fatalf("soft-delete project: %v", err)
	}
	if _, err := store.ListStylePresets(ctx, actor, &project.ID); err == nil {
		t.Fatal("deleted project ID accepted for style preset list")
	}
}

func TestWorkspaceStoreReturnsStylePresetGenerationContent(t *testing.T) {
	ctx, database := workspaceMigrationDB(t)
	orgID := insertWorkspaceOrganization(ctx, t, database)
	actor := insertWorkspaceActor(ctx, t, database, orgID)
	presetID := insertStylePreset(ctx, t, database, orgID, "", false)
	referenceIDs := []uuid.UUID{uuid.New(), uuid.New()}
	if err := database.WithContext(ctx).Exec(`
		UPDATE workspace.style_preset
		SET prompt_fragment = ?, negative_prompt = ?,
		    reference_asset_ids = ARRAY[?::uuid, ?::uuid]
		WHERE id = ?::uuid
	`, "古风光影", "现代建筑", referenceIDs[0].String(), referenceIDs[1].String(), presetID.String()).Error; err != nil {
		t.Fatalf("set style preset generation content: %v", err)
	}
	presets, err := pgworkspace.NewStore(database).ListStylePresets(ctx, actor, nil)
	if err != nil {
		t.Fatalf("list style presets: %v", err)
	}
	if len(presets) != 1 || presets[0].ID != presetID ||
		presets[0].PromptFragment != "古风光影" || presets[0].NegativePrompt != "现代建筑" ||
		len(presets[0].ReferenceAssetIDs) != len(referenceIDs) {
		t.Fatalf("style preset generation content = %+v", presets)
	}
	for index, id := range referenceIDs {
		if presets[0].ReferenceAssetIDs[index] != id {
			t.Fatalf("style preset reference at %d = %s, want %s", index, presets[0].ReferenceAssetIDs[index], id)
		}
	}
}
