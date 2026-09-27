package workspace_test

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	pgworkspace "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
)

func insertProjectListFixture(
	ctx context.Context, t *testing.T, database *gorm.DB,
	orgID, name, status string, deleted bool, updatedAt time.Time,
) uuid.UUID {
	t.Helper()
	id := uuid.New()
	var archivedAt, deletedAt, purgeAfter any
	if status == "archived" {
		archivedAt = updatedAt.Add(-48 * time.Hour)
	}
	if deleted {
		deletedTime := updatedAt.Add(-24 * time.Hour)
		deletedAt = deletedTime
		purgeAfter = deletedTime.Add(30 * 24 * time.Hour)
	}
	result := database.WithContext(ctx).Exec(`
		INSERT INTO workspace.project
		  (id, org_id, name, aspect_ratio, style_type, status, archived_at,
		   is_delete, delete_time, purge_after, revision, create_time, update_time)
		VALUES (?::uuid, ?::uuid, ?, '16:9', 'realistic', ?, ?, ?, ?, ?, 2, ?, ?)
	`, id.String(), orgID, name, status, archivedAt, deleted, deletedAt,
		purgeAfter, updatedAt.Add(-7*24*time.Hour), updatedAt)
	if result.Error != nil || result.RowsAffected != 1 {
		t.Fatalf("insert project list fixture: rows=%d err=%v", result.RowsAffected, result.Error)
	}
	return id
}

func TestListProjectsScopesStatusesRecycleBinAndLiteralSearchOnPostgres(t *testing.T) {
	ctx, database := workspaceMigrationDB(t)
	orgID := insertWorkspaceOrganization(ctx, t, database)
	otherOrgID := insertWorkspaceOrganization(ctx, t, database)
	actor := insertWorkspaceActor(ctx, t, database, orgID)
	query := workspaceapp.NewListProjectsQuery(pgworkspace.NewStore(database))
	updatedAt := time.Date(2026, time.September, 27, 8, 0, 0, 0, time.UTC)
	activeID := insertProjectListFixture(ctx, t, database, orgID, "逆光", "active", false, updatedAt)
	specialID := insertProjectListFixture(ctx, t, database, orgID, "片名%_终章", "active", false, updatedAt.Add(time.Minute))
	decoyID := insertProjectListFixture(ctx, t, database, orgID, "片名甲乙终章", "active", false, updatedAt.Add(2*time.Minute))
	archivedID := insertProjectListFixture(ctx, t, database, orgID, "往事", "archived", false, updatedAt.Add(3*time.Minute))
	deletedID := insertProjectListFixture(ctx, t, database, orgID, "回收中", "archived", true, updatedAt.Add(4*time.Minute))
	insertProjectListFixture(ctx, t, database, otherOrgID, "片名%_他人", "active", false, updatedAt.Add(5*time.Minute))

	page, err := query.Execute(ctx, actor, workspaceapp.ListProjectsInput{Limit: 20})
	if err != nil || len(page.Projects) != 4 || page.Next != nil {
		t.Fatalf("default project page = %+v: %v", page, err)
	}
	wantVisible := map[uuid.UUID]bool{activeID: true, specialID: true, decoyID: true, archivedID: true}
	for _, item := range page.Projects {
		if !wantVisible[item.ID] || item.OrgID != actor.OrgID || item.IsDelete ||
			item.Name == "" || item.Revision != 2 || item.UpdateTime.IsZero() {
			t.Fatalf("unexpected project in current organization: %+v", item)
		}
		delete(wantVisible, item.ID)
	}
	if len(wantVisible) != 0 {
		t.Fatalf("visible projects omitted: %+v", wantVisible)
	}

	active, err := query.Execute(ctx, actor, workspaceapp.ListProjectsInput{Status: "active", Limit: 20})
	if err != nil || len(active.Projects) != 3 {
		t.Fatalf("active project page = %+v: %v", active, err)
	}
	for _, item := range active.Projects {
		if item.Status != "active" || item.IsDelete {
			t.Fatalf("active filter returned %+v", item)
		}
	}
	archived, err := query.Execute(ctx, actor, workspaceapp.ListProjectsInput{Status: "archived", Limit: 20})
	if err != nil || len(archived.Projects) != 1 || archived.Projects[0].ID != archivedID {
		t.Fatalf("archived project page = %+v: %v", archived, err)
	}
	recycle, err := query.Execute(ctx, actor, workspaceapp.ListProjectsInput{Deleted: true, Limit: 20})
	if err != nil || len(recycle.Projects) != 1 || recycle.Projects[0].ID != deletedID ||
		!recycle.Projects[0].IsDelete || recycle.Projects[0].Status != "archived" ||
		recycle.Projects[0].DeleteTime == nil || recycle.Projects[0].PurgeAfter == nil {
		t.Fatalf("recycle project page = %+v: %v", recycle, err)
	}
	searched, err := query.Execute(ctx, actor, workspaceapp.ListProjectsInput{Query: "%_", Limit: 20})
	if err != nil || len(searched.Projects) != 1 || searched.Projects[0].ID != specialID {
		t.Fatalf("literal wildcard search = %+v: %v", searched, err)
	}
}

func TestListProjectsUsesUpdateTimeAndIDKeysetOnPostgres(t *testing.T) {
	ctx, database := workspaceMigrationDB(t)
	orgID := insertWorkspaceOrganization(ctx, t, database)
	actor := insertWorkspaceActor(ctx, t, database, orgID)
	query := workspaceapp.NewListProjectsQuery(pgworkspace.NewStore(database))
	recent := time.Date(2026, time.September, 27, 10, 0, 0, 0, time.UTC)
	older := recent.Add(-time.Hour)
	type expected struct {
		id        uuid.UUID
		updatedAt time.Time
	}
	want := []expected{
		{insertProjectListFixture(ctx, t, database, orgID, "同刻一", "active", false, recent), recent},
		{insertProjectListFixture(ctx, t, database, orgID, "同刻二", "active", false, recent), recent},
		{insertProjectListFixture(ctx, t, database, orgID, "较早一", "active", false, older), older},
		{insertProjectListFixture(ctx, t, database, orgID, "较早二", "active", false, older), older},
	}
	sort.Slice(want, func(i, j int) bool {
		if want[i].updatedAt.Equal(want[j].updatedAt) {
			return want[i].id.String() > want[j].id.String()
		}
		return want[i].updatedAt.After(want[j].updatedAt)
	})

	first, err := query.Execute(ctx, actor, workspaceapp.ListProjectsInput{Limit: 2})
	if err != nil || len(first.Projects) != 2 || first.Next == nil ||
		first.Projects[0].ID != want[0].id || first.Projects[1].ID != want[1].id ||
		first.Next.ID != want[1].id || !first.Next.UpdateTime.Equal(recent) {
		t.Fatalf("first keyset page = %+v: %v; want %+v", first, err, want)
	}
	second, err := query.Execute(ctx, actor, workspaceapp.ListProjectsInput{Limit: 2, After: first.Next})
	if err != nil || len(second.Projects) != 2 || second.Next != nil ||
		second.Projects[0].ID != want[2].id || second.Projects[1].ID != want[3].id {
		t.Fatalf("second keyset page = %+v: %v; want %+v", second, err, want)
	}
}

func TestListProjectsRechecksRevokedActorAndOrganizationOnPostgres(t *testing.T) {
	ctx, database := workspaceMigrationDB(t)
	orgID := insertWorkspaceOrganization(ctx, t, database)
	actor := insertWorkspaceActor(ctx, t, database, orgID)
	insertProjectListFixture(ctx, t, database, orgID, "待撤权", "active", false, time.Now().UTC())
	query := workspaceapp.NewListProjectsQuery(pgworkspace.NewStore(database))
	if _, err := query.Execute(ctx, actor, workspaceapp.ListProjectsInput{}); err != nil {
		t.Fatalf("list for current producer: %v", err)
	}
	if err := database.WithContext(ctx).Exec(`
		UPDATE identity."user" SET status = 'disabled' WHERE id = ?::uuid
	`, actor.ID.String()).Error; err != nil {
		t.Fatalf("disable actor: %v", err)
	}
	if _, err := query.Execute(ctx, actor, workspaceapp.ListProjectsInput{}); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatalf("disabled actor list = %v, want forbidden", err)
	}
	if err := database.WithContext(ctx).Exec(`
		UPDATE identity."user" SET status = 'active' WHERE id = ?::uuid
	`, actor.ID.String()).Error; err != nil {
		t.Fatalf("restore actor for organization revocation: %v", err)
	}
	if err := database.WithContext(ctx).Exec(`
		UPDATE workspace.organization SET status = 'disabled' WHERE id = ?::uuid
	`, orgID).Error; err != nil {
		t.Fatalf("disable organization: %v", err)
	}
	if _, err := query.Execute(ctx, actor, workspaceapp.ListProjectsInput{}); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatalf("disabled organization list = %v, want forbidden", err)
	}
}
