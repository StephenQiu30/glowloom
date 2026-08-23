package identity_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	. "github.com/stephenqiu30/lanverse/backend/src/identity"
	"github.com/stephenqiu30/lanverse/backend/src/platform/database"
)

func TestAuthorizePathUsesCanonicalProjectAndSourceRevisionTables(t *testing.T) {
	if os.Getenv("LANVERSE_INTEGRATION") != "1" {
		t.Skip("set LANVERSE_INTEGRATION=1 to run PostgreSQL/GORM integration")
	}

	ctx := context.Background()
	pool, err := database.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	orm, err := database.OpenGORM(pool)
	if err != nil {
		pool.Close()
		t.Fatal(err)
	}

	workspaceID, projectID, revisionID, artifactID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	t.Cleanup(func() {
		orm.Table("nar_source_revisions").Where("id = ?", revisionID).Delete(&struct{ ID uuid.UUID }{})
		orm.Table("media_artifact_locations").Where("artifact_id = ?", artifactID).Delete(&struct{ ID uuid.UUID }{})
		orm.Table("media_artifacts").Where("id = ?", artifactID).Delete(&struct{ ID uuid.UUID }{})
		orm.Table("projects").Where("id = ?", projectID).Delete(&struct{ ID uuid.UUID }{})
		orm.Table("workspaces").Where("id = ?", workspaceID).Delete(&struct{ ID uuid.UUID }{})
		pool.Close()
	})

	if err := orm.Table("workspaces").Create(&struct {
		ID   uuid.UUID
		Name string
	}{workspaceID, "identity integration workspace"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := orm.Table("projects").Create(&struct {
		ID          uuid.UUID
		WorkspaceID uuid.UUID
		Name        string
	}{projectID, workspaceID, "identity integration project"}).Error; err != nil {
		t.Fatal(err)
	}
	contentHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if err := orm.Table("media_artifacts").Create(map[string]any{
		"id": artifactID, "workspace_id": workspaceID, "project_id": projectID, "content_hash": contentHash,
		"size_bytes": 1, "media_type": "text/plain", "purpose": "source", "retention_class": "standard", "status": "ready",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := orm.Table("media_artifact_locations").Create(map[string]any{
		"id": uuid.New(), "artifact_id": artifactID, "storage_profile": "test", "bucket": "test", "object_key": uuid.NewString(),
		"object_version_id": uuid.NewString(), "size_bytes": 1, "content_hash": contentHash, "status": "active",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := orm.Table("nar_source_revisions").Create(map[string]any{
		"id": revisionID, "project_id": projectID, "artifact_id": artifactID, "name": "identity-integration.txt",
		"source_type": "txt", "status": "uploaded",
	}).Error; err != nil {
		t.Fatal(err)
	}

	repository := NewIdentityRepository(orm)
	authorizedContext := database.WithWorkspaceID(ctx, workspaceID)
	for _, path := range []string{
		"/api/projects/" + projectID.String() + "/script-revisions",
		"/api/script-revisions/" + revisionID.String() + "/analysis",
	} {
		if err := repository.AuthorizePath(authorizedContext, workspaceID, path); err != nil {
			t.Fatalf("AuthorizePath(%q) error = %v", path, err)
		}
	}
}

func TestWorkspaceMemberChangeWritesRestorableAuditInSameTransaction(t *testing.T) {
	if os.Getenv("LANVERSE_INTEGRATION") != "1" {
		t.Skip("set LANVERSE_INTEGRATION=1 to run PostgreSQL/GORM integration")
	}

	ctx := context.Background()
	pool, err := database.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	orm, err := database.OpenGORM(pool)
	if err != nil {
		pool.Close()
		t.Fatal(err)
	}

	workspaceID, foreignWorkspaceID, actorUserID, targetUserID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	actorMembershipID, targetMembershipID := uuid.New(), uuid.New()
	t.Cleanup(func() {
		orm.Table("audit_events").Where("workspace_id IN ?", []uuid.UUID{workspaceID, foreignWorkspaceID}).Delete(&struct{ ID uuid.UUID }{})
		orm.Table("iam_memberships").Where("workspace_id = ?", workspaceID).Delete(&struct{ ID uuid.UUID }{})
		orm.Table("iam_users").Where("id IN ?", []uuid.UUID{actorUserID, targetUserID}).Delete(&struct{ ID uuid.UUID }{})
		orm.Table("workspaces").Where("id IN ?", []uuid.UUID{workspaceID, foreignWorkspaceID}).Delete(&struct{ ID uuid.UUID }{})
		pool.Close()
	})

	for _, workspace := range []map[string]any{
		{"id": workspaceID, "name": "audit integration workspace"},
		{"id": foreignWorkspaceID, "name": "foreign audit workspace"},
	} {
		if err := orm.Table("workspaces").Create(workspace).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, user := range []map[string]any{
		{"id": actorUserID, "identity_subject": "audit-actor-" + actorUserID.String(), "display_name": "Audit Actor", "status": "active"},
		{"id": targetUserID, "identity_subject": "audit-target-" + targetUserID.String(), "display_name": "Audit Target", "status": "active"},
	} {
		if err := orm.Table("iam_users").Create(user).Error; err != nil {
			t.Fatal(err)
		}
	}
	var adminRole, userRole struct {
		ID uuid.UUID `gorm:"column:id"`
	}
	if err := orm.Table("iam_roles").Select("id").Where("code = ?", RoleAdmin).Take(&adminRole).Error; err != nil {
		t.Fatal(err)
	}
	if err := orm.Table("iam_roles").Select("id").Where("code = ?", RoleUser).Take(&userRole).Error; err != nil {
		t.Fatal(err)
	}
	for _, membership := range []map[string]any{
		{"id": actorMembershipID, "workspace_id": workspaceID, "user_id": actorUserID, "role_id": adminRole.ID, "status": "active"},
		{"id": targetMembershipID, "workspace_id": workspaceID, "user_id": targetUserID, "role_id": userRole.ID, "status": "active"},
	} {
		if err := orm.Table("iam_memberships").Create(membership).Error; err != nil {
			t.Fatal(err)
		}
	}

	requestID := "audit-member-change-" + uuid.NewString()
	reason := "项目职责调整"
	role := RoleBan
	repository := NewIdentityRepository(orm)
	updated, err := repository.UpdateWorkspaceMember(database.WithWorkspaceID(ctx, workspaceID), workspaceID, targetMembershipID, Principal{
		UserID:       actorUserID,
		WorkspaceID:  workspaceID,
		MembershipID: actorMembershipID,
		Role:         RoleAdmin,
	}, WorkspaceMemberUpdate{Role: &role, Reason: reason, RequestID: requestID})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Role != RoleBan || updated.MembershipStatus != MembershipStatusActive {
		t.Fatalf("updated member = %#v", updated)
	}

	var event struct {
		WorkspaceID uuid.UUID       `gorm:"column:workspace_id"`
		ActorType   string          `gorm:"column:actor_type"`
		ActorID     string          `gorm:"column:actor_id"`
		Action      string          `gorm:"column:action"`
		ObjectType  string          `gorm:"column:object_type"`
		ObjectID    uuid.UUID       `gorm:"column:object_id"`
		BeforeState json.RawMessage `gorm:"column:before_state"`
		AfterState  json.RawMessage `gorm:"column:after_state"`
		BeforeHash  string          `gorm:"column:before_hash"`
		AfterHash   string          `gorm:"column:after_hash"`
		RequestID   string          `gorm:"column:request_id"`
		Reason      string          `gorm:"column:reason"`
		Result      string          `gorm:"column:result"`
	}
	if err := orm.Table("audit_events").Where("workspace_id = ? AND request_id = ?", workspaceID, requestID).Take(&event).Error; err != nil {
		t.Fatal(err)
	}
	if event.WorkspaceID != workspaceID || event.ActorType != "user" || event.ActorID != actorUserID.String() ||
		event.Action != "iam.membership.updated" || event.ObjectType != "iam_membership" || event.ObjectID != targetMembershipID ||
		event.RequestID != requestID || event.Reason != reason || event.Result != "succeeded" {
		t.Fatalf("audit identity = %#v", event)
	}
	var before, after struct {
		Role   RoleCode         `json:"role"`
		Status MembershipStatus `json:"status"`
	}
	if err := json.Unmarshal(event.BeforeState, &before); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(event.AfterState, &after); err != nil {
		t.Fatal(err)
	}
	if before.Role != RoleUser || before.Status != MembershipStatusActive || after.Role != RoleBan || after.Status != MembershipStatusActive {
		t.Fatalf("audit states before=%#v after=%#v", before, after)
	}
	if len(event.BeforeHash) != 64 || len(event.AfterHash) != 64 || event.BeforeHash == event.AfterHash {
		t.Fatalf("audit hashes before=%q after=%q", event.BeforeHash, event.AfterHash)
	}

	if err := orm.Table("audit_events").Create(map[string]any{
		"id": uuid.New(), "workspace_id": foreignWorkspaceID, "actor_type": "user", "actor_id": actorUserID.String(),
		"action": "iam.membership.updated", "object_type": "iam_membership", "object_id": targetMembershipID,
		"before_state": json.RawMessage(`{"role":"user","status":"active"}`), "after_state": json.RawMessage(`{"role":"ban","status":"active"}`),
		"before_hash": strings.Repeat("0", 64), "after_hash": strings.Repeat("1", 64),
		"request_id": "foreign-audit-request", "reason": reason, "result": "succeeded", "occurred_at": time.Now().UTC(),
	}).Error; err != nil {
		t.Fatal(err)
	}
	from, to := time.Now().UTC().Add(-time.Minute), time.Now().UTC().Add(time.Minute)
	auditPage, err := repository.ListAccessAudit(database.WithWorkspaceID(ctx, workspaceID), workspaceID, AccessAuditQuery{
		Actor: "Audit Actor", Object: "Audit Target", Action: "iam.membership.updated", Result: "succeeded",
		OccurredFrom: &from, OccurredTo: &to, Page: 1, PageSize: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if auditPage.Total != 1 || len(auditPage.Items) != 1 {
		t.Fatalf("audit page = %#v, want one current-workspace event", auditPage)
	}
	listed := auditPage.Items[0]
	if listed.WorkspaceID != workspaceID || listed.ActorDisplayName != "Audit Actor" || listed.ObjectDisplayName != "Audit Target" ||
		listed.RequestID != requestID || listed.Reason != reason || listed.Result != "succeeded" ||
		listed.BeforeState["role"] != "user" || listed.AfterState["role"] != "ban" {
		t.Fatalf("listed audit event = %#v", listed)
	}

	if err := orm.Exec(`
		CREATE OR REPLACE FUNCTION lanverse_test_reject_audit_write() RETURNS trigger AS $test$
		BEGIN
			IF NEW.reason = 'force audit failure' THEN
				RAISE EXCEPTION 'forced audit failure';
			END IF;
			RETURN NEW;
		END;
		$test$ LANGUAGE plpgsql;
	`).Error; err != nil {
		t.Fatal(err)
	}
	if err := orm.Exec(`CREATE TRIGGER lanverse_test_reject_audit_write
		BEFORE INSERT ON audit_events
		FOR EACH ROW EXECUTE FUNCTION lanverse_test_reject_audit_write()`).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		orm.Exec("DROP TRIGGER IF EXISTS lanverse_test_reject_audit_write ON audit_events")
		orm.Exec("DROP FUNCTION IF EXISTS lanverse_test_reject_audit_write()")
	})

	rejectedRequestID := "audit-member-change-rejected-" + uuid.NewString()
	role = RoleUser
	_, err = repository.UpdateWorkspaceMember(database.WithWorkspaceID(ctx, workspaceID), workspaceID, targetMembershipID, Principal{
		UserID:       actorUserID,
		WorkspaceID:  workspaceID,
		MembershipID: actorMembershipID,
		Role:         RoleAdmin,
	}, WorkspaceMemberUpdate{Role: &role, Reason: "force audit failure", RequestID: rejectedRequestID})
	if err == nil {
		t.Fatal("member update succeeded while audit insert failed")
	}
	var persisted struct {
		Role RoleCode `gorm:"column:role"`
	}
	if err := orm.Table("iam_memberships AS memberships").
		Select("roles.code AS role").
		Joins("JOIN iam_roles AS roles ON roles.id = memberships.role_id").
		Where("memberships.id = ? AND memberships.workspace_id = ?", targetMembershipID, workspaceID).
		Take(&persisted).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.Role != RoleBan {
		t.Fatalf("member role = %q after audit failure, want %q", persisted.Role, RoleBan)
	}
	var rejectedAuditCount int64
	if err := orm.Table("audit_events").Where("workspace_id = ? AND request_id = ?", workspaceID, rejectedRequestID).Count(&rejectedAuditCount).Error; err != nil {
		t.Fatal(err)
	}
	if rejectedAuditCount != 0 {
		t.Fatalf("rejected audit count = %d, want 0", rejectedAuditCount)
	}
}
