package workspace_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	identitydomain "github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/domain"
)

func TestProjectRecoveryExactThirtyDayBoundary(t *testing.T) {
	for _, archived := range []bool{false, true} {
		for _, step := range []struct {
			name    string
			offset  time.Duration
			expired bool
		}{
			{"before", -time.Nanosecond, false}, {"at", 0, true}, {"after", time.Nanosecond, true},
		} {
			state := "active"
			if archived {
				state = "archived"
			}
			t.Run(state+"/"+step.name, func(t *testing.T) {
				actor := projectCommandActor(identitydomain.RoleProducer)
				project := updateProjectFixture(actor)
				project.CanvasID = uuid.New()
				deletedAt := time.Date(2026, 10, 21, 8, 35, 40, 0, time.UTC)
				if archived {
					if err := project.Archive(deletedAt.Add(-time.Hour), false); err != nil {
						t.Fatal(err)
					}
				}
				if err := project.Delete(deletedAt, false); err != nil {
					t.Fatal(err)
				}
				deadline := deletedAt.Add(30 * 24 * time.Hour)
				if project.PurgeAfter == nil || !project.PurgeAfter.Equal(deadline) {
					t.Fatal("recovery deadline must be exactly thirty days")
				}
				input := workspaceapp.ProjectChangeInput{Action: "restore", IdempotencyKey: uuid.New(), Patch: workspaceapp.UpdateProjectInput{ProjectID: project.ID, ExpectedRevision: project.Revision, RequestID: uuid.NewString()}}
				restored, _, err := workspaceapp.PrepareProjectChange(actor, input, project, deadline.Add(step.offset), false)
				if step.expired {
					if !errors.Is(err, domain.ErrProjectRestoreExpired) {
						t.Fatalf("expected expired recovery: %v", err)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if restored.IsDelete || restored.Status != state || restored.ID != project.ID || restored.CanvasID != project.CanvasID || restored.Description != project.Description || restored.DeleteTime != nil || restored.PurgeAfter != nil {
					t.Fatal("restoration lost original identity, content or lifecycle state")
				}
			})
		}
	}
}
