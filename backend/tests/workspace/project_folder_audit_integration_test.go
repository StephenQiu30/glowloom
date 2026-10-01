package workspace_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	auditevent "github.com/StephenQiu30/lanverse/backend/internal/audit/adapter/event"
	auditapp "github.com/StephenQiu30/lanverse/backend/internal/audit/application"
	inboxpg "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/adapter/postgres"
	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
)

func TestProjectFolderPGSafetyAuditActualInbox(t *testing.T) {
	ctx, db, owner := folderTestDB(t)
	actor := insertWorkspaceActor(ctx, t, owner, insertWorkspaceOrganization(ctx, t, owner))
	project := uuid.MustParse(insertWorkspaceProject(ctx, t, owner, actor.OrgID.String(), "16:9", "realistic", nil))
	service := workspaceapp.NewProjectFolders(folderStore(db), time.Now)
	f := createTestFolder(ctx, t, service, actor, "不进入审计的私有标题")
	out := moveTestProject(ctx, t, service, actor, project, f, 0)
	f = *out.Folder
	rename := folderCommand("patch")
	name := "不进入审计的更名"
	rename.Name = &name
	rename.FolderID = f.ID
	rename.ExpectedRevision = f.Revision
	out, err := service.Change(ctx, actor, rename)
	if err != nil {
		t.Fatal(err)
	}
	f = *out.Folder
	recycle := folderCommand("recycle")
	recycle.FolderID = f.ID
	recycle.ExpectedRevision = f.Revision
	if _, err = service.Change(ctx, actor, recycle); err != nil {
		t.Fatal(err)
	}
	fields := []string{"folder_id", "project_id", "revision", "placement_revision", "recycled_count", "name_updated", "cover_updated"}
	// The composition root installs this exact action policy separately; no new event topic.
	policy := map[string][]string{"project_folder.created": fields, "project_folder.updated": fields, "project_folder.moved": fields, "project_folder.recycled": fields, "project.deleted": {"revision", "status", "is_delete", "archived_at", "delete_time", "purge_after"}}
	parser := auditapp.NewParser(policy)
	handler := auditevent.NewHandler(inboxpg.NewStore(db), parser)
	var rows []struct {
		ID           uuid.UUID
		PartitionKey string
		Payload      json.RawMessage
	}
	if err = db.Raw(`SELECT id,partition_key,payload FROM infra.outbox WHERE topic='lanverse.audit.recorded.v1' AND payload->'actor'->>'id'=? ORDER BY create_time,id`, actor.ID.String()).Scan(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 5 {
		t.Fatalf("expected 4 folder+1 project audit, got %d", len(rows))
	}
	for _, row := range rows {
		if strings.Contains(string(row.Payload), "不进入审计") {
			t.Fatal("private title persisted in audit")
		}
		record := inbox.Record{Topic: "lanverse.audit.recorded.v1", Key: []byte(row.PartitionKey), Value: row.Payload}
		if err = handler.Handle(ctx, record); err != nil {
			t.Fatalf("consume closed audit %v", err)
		}
		if err = handler.Handle(ctx, record); err != nil {
			t.Fatal(err)
		}
	}
	var counts struct{ Logs, Markers int64 }
	if err = db.Raw(`SELECT(SELECT count(*) FROM audit.audit_log WHERE actor_id=?)AS logs,(SELECT count(*) FROM infra.processed_event WHERE consumer='audit' AND event_id IN(SELECT id FROM infra.outbox WHERE topic='lanverse.audit.recorded.v1' AND payload->'actor'->>'id'=?))AS markers`, actor.ID, actor.ID.String()).Scan(&counts).Error; err != nil || counts.Logs != 5 || counts.Markers != 5 {
		t.Fatalf("actual inbox dedup %+v %v", counts, err)
	}
}
