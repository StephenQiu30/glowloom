package workspace_test

import (
	"testing"
	"time"

	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

func TestRecentContentMissingActivityUsesSameEffectiveTimeEverywhere(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	actor := insertWorkspaceActor(ctx, t, db, insertWorkspaceOrganization(ctx, t, db))
	missing := insertWorkspaceProject(ctx, t, db, actor, "16:9", "realistic", nil)
	recent := insertWorkspaceProject(ctx, t, db, actor, "16:9", "realistic", nil)
	old := insertWorkspaceProject(ctx, t, db, actor, "16:9", "realistic", nil)
	for _, row := range []struct {
		ID   string
		When time.Time
	}{{recent, time.Now().Add(time.Hour)}, {old, time.Now().Add(-time.Hour)}} {
		if err := db.Exec(`INSERT INTO workspace.project_activity(project_id,modified_at,basis) VALUES(?,?,'content')`, row.ID, row.When).Error; err != nil {
			t.Fatal(err)
		}
	}
	q := workspaceapp.NewListProjectsQuery(workspaceStore(db))
	input := workspaceapp.ListProjectsInput{Order: "recent_content", Limit: 1}
	for i, want := range []string{recent, missing, old} {
		page, err := q.Execute(ctx, actor, input)
		if err != nil || len(page.Projects) != 1 || page.Projects[0].ID.String() != want {
			t.Fatalf("page %d: %+v %v", i, page, err)
		}
		item := page.Projects[0]
		if i == 1 && (!item.LastModifiedAt.Equal(item.CreateTime) || item.LastModifiedBasis != "created") {
			t.Fatalf("missing row fallback: %+v", item)
		}
		if i < 2 && (page.Next == nil || !page.Next.UpdateTime.Equal(item.LastModifiedAt)) {
			t.Fatal("cursor used a different time than displayed")
		}
		input.After = page.Next
	}
}
