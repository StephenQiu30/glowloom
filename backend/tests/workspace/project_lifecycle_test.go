package workspace_test

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	auditapp "github.com/StephenQiu30/lanverse/backend/internal/audit/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	inboxapp "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

func TestProjectLifecyclePreparationPreservesRecoveryAndAudit(t *testing.T) {
	actor := projectCommandActor(identitydomain.RoleProducer)
	project := updateProjectFixture(actor)
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	for _, action := range []string{"archive", "delete", "restore", "unarchive"} {
		before := project
		input := workspaceapp.ProjectChangeInput{Action: action, IdempotencyKey: uuid.New(), Patch: workspaceapp.UpdateProjectInput{ProjectID: project.ID, ExpectedRevision: project.Revision, RequestID: uuid.NewString()}}
		after, events, err := workspaceapp.PrepareProjectChange(actor, input, before, now, false)
		if err != nil || len(events) != 2 || after.Revision != before.Revision+1 {
			t.Fatalf("%s result=%+v events=%d err=%v", action, after, len(events), err)
		}
		if after.ID != before.ID || after.OrgID != before.OrgID || after.Description != before.Description || after.StylePresetID != before.StylePresetID {
			t.Fatalf("%s changed project content", action)
		}
		if action == "delete" && (!after.IsDelete || after.Status != "archived" || after.PurgeAfter == nil || !after.PurgeAfter.Equal(now.Add(30*24*time.Hour))) {
			t.Fatal("delete lost recovery facts")
		}
		if action == "restore" && (after.IsDelete || after.Status != "archived" || after.DeleteTime != nil || after.PurgeAfter != nil) {
			t.Fatal("restore changed prior status")
		}
		if _, err := auditapp.NewRecordedActionParser().Parse(inboxapp.Record{Topic: events[1].Topic, Key: []byte(events[1].PartitionKey), Value: events[1].Payload}); err != nil {
			t.Fatalf("%s audit consumer rejected real safe envelope: %v", action, err)
		}
		if bytes.Contains(events[1].Payload, []byte(before.Description)) && before.Description != "" {
			t.Fatal("private description entered lifecycle audit")
		}
		project = after
	}
}

func TestProjectLifecyclePreparationRejectsUnsafeChanges(t *testing.T) {
	actor := projectCommandActor(identitydomain.RoleProducer)
	project := updateProjectFixture(actor)
	now := time.Now().UTC()
	input := workspaceapp.ProjectChangeInput{Action: "delete", IdempotencyKey: uuid.New(), Patch: workspaceapp.UpdateProjectInput{ProjectID: project.ID, ExpectedRevision: project.Revision, RequestID: uuid.NewString()}}
	if _, _, err := workspaceapp.PrepareProjectChange(actor, input, project, now, true); !errors.Is(err, domain.ErrProjectHasInflightOperations) {
		t.Fatalf("blocking work err=%v", err)
	}
	input.Patch.ExpectedRevision++
	if _, _, err := workspaceapp.PrepareProjectChange(actor, input, project, now, false); !errors.Is(err, domain.ErrProjectRevisionConflict) {
		t.Fatalf("stale err=%v", err)
	}
	input.Patch.ExpectedRevision = project.Revision
	input.Action = "patch"
	nul := "bad\x00name"
	input.Patch.Name = &nul
	if _, _, err := workspaceapp.PrepareProjectChange(actor, input, project, now, false); !errors.Is(err, workspaceapp.ErrInvalidProjectChange) {
		t.Fatalf("NUL name err=%v", err)
	}
	unchanged := "  " + project.Name + "  "
	input.Patch.Name = &unchanged
	after, events, err := workspaceapp.PrepareProjectChange(actor, input, project, now, false)
	if err != nil || after.Revision != project.Revision || len(events) != 0 {
		t.Fatalf("no-op result=%+v events=%d err=%v", after, len(events), err)
	}
	input.Action = "restore"
	input.Patch.Name = nil
	if err := project.Delete(now, false); err != nil {
		t.Fatal(err)
	}
	input.Patch.ExpectedRevision = project.Revision
	if _, _, err := workspaceapp.PrepareProjectChange(actor, input, project, *project.PurgeAfter, false); !errors.Is(err, domain.ErrProjectRestoreExpired) {
		t.Fatalf("expiry err=%v", err)
	}
}
