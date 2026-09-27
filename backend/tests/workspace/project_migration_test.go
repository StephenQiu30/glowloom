package workspace_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

func workspaceMigrationDB(t *testing.T) (context.Context, *gorm.DB) {
	t.Helper()
	dsn := os.Getenv("LV_TEST_WORKSPACE_DB_DSN")
	if dsn == "" {
		t.Skip("set LV_TEST_WORKSPACE_DB_DSN to an isolated PostgreSQL database with workspace migrations applied")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatalf("open workspace database: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return ctx, conn.DB.WithContext(ctx)
}

func insertWorkspaceOrganization(t *testing.T, ctx context.Context, database *gorm.DB) string {
	t.Helper()
	id := uuid.NewString()
	if err := database.WithContext(ctx).Exec(`
		INSERT INTO workspace.organization (id, name) VALUES (?::uuid, ?)
	`, id, "test-"+id).Error; err != nil {
		t.Fatalf("insert organization: %v", err)
	}
	return id
}

func insertWorkspaceProject(t *testing.T, ctx context.Context, database *gorm.DB, orgID, aspectRatio, styleType string, styleSubtype any) string {
	t.Helper()
	id := uuid.NewString()
	if err := database.WithContext(ctx).Exec(`
		INSERT INTO workspace.project (id, org_id, name, aspect_ratio, style_type, style_subtype)
		VALUES (?::uuid, ?::uuid, ?, ?, ?, ?)
	`, id, orgID, "project-"+id, aspectRatio, styleType, styleSubtype).Error; err != nil {
		t.Fatalf("insert project: %v", err)
	}
	return id
}

func TestWorkspaceProjectMigrationEnforcesSpecification(t *testing.T) {
	ctx, database := workspaceMigrationDB(t)
	orgID := insertWorkspaceOrganization(t, ctx, database)
	projectID := insertWorkspaceProject(t, ctx, database, orgID, "9:16", "realistic", nil)

	var defaults struct {
		Resolution          string
		AllowOverseasModels bool
		Status              string
		Revision            int
	}
	if err := database.Raw(`
		SELECT resolution, allow_overseas_models, status, revision
		FROM workspace.project WHERE id = ?::uuid
	`, projectID).Scan(&defaults).Error; err != nil {
		t.Fatalf("read project defaults: %v", err)
	}
	if defaults.Resolution != "1080p" || defaults.AllowOverseasModels ||
		defaults.Status != "active" || defaults.Revision != 1 {
		t.Fatalf("project defaults = %+v", defaults)
	}

	for name, input := range map[string]struct {
		aspectRatio  string
		styleType    string
		styleSubtype any
		resolution   string
	}{
		"unsupported aspect ratio": {"4:3", "realistic", nil, "1080p"},
		"unsupported resolution":   {"9:16", "realistic", nil, "720p"},
		"stylized without subtype": {"9:16", "stylized", nil, "1080p"},
		"unsupported subtype":      {"9:16", "stylized", "watercolor", "1080p"},
		"realistic with subtype":   {"9:16", "realistic", "anime_jp", "1080p"},
	} {
		t.Run(name, func(t *testing.T) {
			id := uuid.NewString()
			err := database.Exec(`
				INSERT INTO workspace.project
				  (id, org_id, name, aspect_ratio, style_type, style_subtype, resolution)
				VALUES (?::uuid, ?::uuid, 'Invalid', ?, ?, ?, ?)
			`, id, orgID, input.aspectRatio, input.styleType, input.styleSubtype, input.resolution).Error
			if err == nil {
				t.Fatalf("invalid project %q was accepted", name)
			}
		})
	}

	insertWorkspaceProject(t, ctx, database, orgID, "16:9", "stylized", "guofeng_xianxia")
	for name, statement := range map[string]string{
		"aspect ratio": `UPDATE workspace.project SET aspect_ratio = '16:9' WHERE id = ?::uuid`,
		"style type":   `UPDATE workspace.project SET style_type = 'stylized', style_subtype = 'anime_jp' WHERE id = ?::uuid`,
		"resolution":   `UPDATE workspace.project SET resolution = '720p' WHERE id = ?::uuid`,
	} {
		t.Run("immutable or fixed "+name, func(t *testing.T) {
			if err := database.Exec(statement, projectID).Error; err == nil {
				t.Fatalf("project %s update was accepted", name)
			}
		})
	}
	if err := database.Exec(`
		UPDATE workspace.project
		SET name = '更新名称', allow_overseas_models = true
		WHERE id = ?::uuid
	`, projectID).Error; err != nil {
		t.Fatalf("update mutable project fields: %v", err)
	}
	var updated struct {
		Name                string
		AspectRatio         string
		StyleType           string
		Resolution          string
		AllowOverseasModels bool
	}
	if err := database.Raw(`
		SELECT name, aspect_ratio, style_type, resolution, allow_overseas_models
		FROM workspace.project WHERE id = ?::uuid
	`, projectID).Scan(&updated).Error; err != nil {
		t.Fatalf("read updated project: %v", err)
	}
	if updated.Name != "更新名称" || !updated.AllowOverseasModels ||
		updated.AspectRatio != "9:16" || updated.StyleType != "realistic" || updated.Resolution != "1080p" {
		t.Fatalf("project after accepted and rejected updates = %+v", updated)
	}
}

func TestWorkspaceBudgetMigrationEnforcesOneBalancedBudgetPerProject(t *testing.T) {
	ctx, database := workspaceMigrationDB(t)
	orgID := insertWorkspaceOrganization(t, ctx, database)
	projectID := insertWorkspaceProject(t, ctx, database, orgID, "9:16", "realistic", nil)
	budgetID := uuid.NewString()
	if err := database.Exec(`
		INSERT INTO billing.budget (id, project_id, limit_micros)
		VALUES (?::uuid, ?::uuid, 0)
	`, budgetID, projectID).Error; err != nil {
		t.Fatalf("insert zero budget: %v", err)
	}
	var defaults struct {
		LimitMicros    int64
		ReservedMicros int64
		SettledMicros  int64
		IsOverrun      bool
		Revision       int
	}
	if err := database.Raw(`
		SELECT limit_micros, reserved_micros, settled_micros, is_overrun, revision
		FROM billing.budget WHERE project_id = ?::uuid
	`, projectID).Scan(&defaults).Error; err != nil {
		t.Fatalf("read budget defaults: %v", err)
	}
	if defaults.LimitMicros != 0 || defaults.ReservedMicros != 0 || defaults.SettledMicros != 0 ||
		defaults.IsOverrun || defaults.Revision != 1 {
		t.Fatalf("budget defaults = %+v", defaults)
	}
	if err := database.Exec(`
		INSERT INTO billing.budget (id, project_id, limit_micros)
		VALUES (?::uuid, ?::uuid, 0)
	`, uuid.NewString(), projectID).Error; err == nil {
		t.Fatal("second budget for one project was accepted")
	}
	if err := database.Exec(`
		UPDATE billing.budget SET reserved_micros = 1 WHERE id = ?::uuid
	`, budgetID).Error; err == nil {
		t.Fatal("reservation above zero budget was accepted")
	}
	if err := database.Exec(`
		UPDATE billing.budget
		SET limit_micros = 100, reserved_micros = 70, settled_micros = 30
		WHERE id = ?::uuid
	`, budgetID).Error; err != nil {
		t.Fatalf("balance at limit rejected: %v", err)
	}
	if err := database.Exec(`
		UPDATE billing.budget SET reserved_micros = 71 WHERE id = ?::uuid
	`, budgetID).Error; err == nil {
		t.Fatal("balance above limit without overrun flag was accepted")
	}
	if err := database.Exec(`
		UPDATE billing.budget SET settled_micros = 31, is_overrun = true WHERE id = ?::uuid
	`, budgetID).Error; err != nil {
		t.Fatalf("explicitly marked settlement overrun rejected: %v", err)
	}
}

func TestWorkspaceStylePresetMigrationScopesOrganizationAndProject(t *testing.T) {
	ctx, database := workspaceMigrationDB(t)
	orgID := insertWorkspaceOrganization(t, ctx, database)
	projectID := insertWorkspaceProject(t, ctx, database, orgID, "16:9", "stylized", "anime_jp")
	var projectPresetID string
	for name, input := range map[string]struct {
		orgID        string
		projectID    any
		styleType    string
		styleSubtype any
	}{
		"organization level": {orgID, nil, "realistic", nil},
		"project level":      {orgID, projectID, "stylized", "anime_jp"},
	} {
		t.Run(name, func(t *testing.T) {
			id := uuid.NewString()
			if err := database.Exec(`
				INSERT INTO workspace.style_preset
				  (id, org_id, project_id, name, style_type, style_subtype)
				VALUES (?::uuid, ?::uuid, ?::uuid, 'Preset', ?, ?)
			`, id, input.orgID, input.projectID, input.styleType, input.styleSubtype).Error; err != nil {
				t.Fatalf("insert %s preset: %v", name, err)
			}
			var ownership struct {
				OrgID     string
				ProjectID *string
			}
			if err := database.Raw(`
				SELECT org_id::text AS org_id, project_id::text AS project_id
				FROM workspace.style_preset WHERE id = ?::uuid
			`, id).Scan(&ownership).Error; err != nil {
				t.Fatalf("read %s preset: %v", name, err)
			}
			if ownership.OrgID != orgID {
				t.Fatalf("%s preset org = %q, want %q", name, ownership.OrgID, orgID)
			}
			if input.projectID == nil && ownership.ProjectID != nil {
				t.Fatalf("organization preset unexpectedly belongs to project %q", *ownership.ProjectID)
			}
			if input.projectID != nil && (ownership.ProjectID == nil || *ownership.ProjectID != projectID) {
				t.Fatalf("project preset ownership = %v, want %q", ownership.ProjectID, projectID)
			}
			if input.projectID != nil {
				projectPresetID = id
			}
		})
	}
	if err := database.Exec(`
		UPDATE workspace.project SET style_preset_id = ?::uuid WHERE id = ?::uuid
	`, uuid.NewString(), projectID).Error; err == nil {
		t.Fatal("project accepted a missing style preset")
	}
	if err := database.Exec(`
		UPDATE workspace.project SET style_preset_id = ?::uuid WHERE id = ?::uuid
	`, projectPresetID, projectID).Error; err != nil {
		t.Fatalf("project rejected its existing style preset: %v", err)
	}
}
