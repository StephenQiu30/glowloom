package workspace_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/postgres"
)

func TestProjectRetentionClearsOnlyExpiredGraphAndDraftPayloads(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	actor := insertWorkspaceActor(ctx, t, db, insertWorkspaceOrganization(ctx, t, db))
	expired := uuid.MustParse(insertWorkspaceProject(ctx, t, db, actor, "16:9", "realistic", nil))
	live := uuid.MustParse(insertWorkspaceProject(ctx, t, db, actor, "16:9", "realistic", nil))
	asset := seedCoverAsset(t, db, live, "image")
	var targetDoc, targetNode uuid.UUID
	for _, project := range []uuid.UUID{expired, live} {
		doc, node := uuid.New(), uuid.New()
		lifecycleFixtureSQL(t, db, `INSERT INTO canvas.document(id,project_id,name)VALUES(?,?,'retention fixture')`, doc, project)
		lifecycleFixtureSQL(t, db, `UPDATE workspace.project SET initial_canvas_id=? WHERE id=?`, doc, project)
		lifecycleFixtureSQL(t, db, `INSERT INTO canvas.node(id,document_id,node_type,node_action,ref_type,ref_id,config,x,y)VALUES(?,?,'image','resource','media',?,'{"text":"private"}',0,0)`, node, doc, asset)
		if project == expired {
			targetDoc, targetNode = doc, node
		}
	}
	lifecycleFixtureSQL(t, db, `INSERT INTO canvas.node_input(node_id,input_revision,draft,created_by)VALUES(?,1,'{"private":"input"}',?)`, targetNode, actor.ID)
	lifecycleFixtureSQL(t, db, `UPDATE canvas.node SET input_revision=1 WHERE id=?`, targetNode)
	lifecycleFixtureSQL(t, db, `INSERT INTO canvas.command_log(id,document_id,revision,commands)VALUES(?,?,1,'[{"private":"history"}]')`, uuid.New(), targetDoc)
	lifecycleFixtureSQL(t, db, `INSERT INTO canvas.save_receipt(actor_id,idem_key,document_id,request_hash,result_revision,response_body)VALUES(?,?,?,repeat('a',64),2,'{"private":"snapshot"}')`, actor.ID, uuid.New(), targetDoc)
	lifecycleFixtureSQL(t, db, `INSERT INTO workspace.prompt_draft(id,project_id,actor_id,revision,draft,draft_hash,input_hash)VALUES(?,?,?,1,'{"private":"draft"}',repeat('a',64),repeat('b',64))`, uuid.New(), expired, actor.ID)
	lifecycleFixtureSQL(t, db, `UPDATE workspace.project SET is_delete=true,revision=2,delete_time=clock_timestamp()-interval '31 days',purge_after=clock_timestamp()-interval '1 day' WHERE id=?`, expired)
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE glowloom_app`).Error; err != nil {
			return err
		}
		_, err := app.NewProjectRetention(tx).Run(ctx, 500)
		return err
	}); err != nil {
		t.Fatal("runtime-role retention", err)
	}
	checks := []struct {
		sql  string
		id   any
		want int64
	}{
		{`SELECT count(*) FROM canvas.node WHERE document_id=?`, targetDoc, 0},
		{`SELECT count(*) FROM canvas.node_input WHERE node_id=?`, targetNode, 0},
		{`SELECT count(*) FROM workspace.prompt_draft WHERE project_id=?`, expired, 0},
		{`SELECT count(*) FROM canvas.command_log WHERE document_id=? AND commands='[]'::jsonb AND is_delete`, targetDoc, 1},
		{`SELECT count(*) FROM canvas.save_receipt WHERE document_id=? AND response_body='{"retention_expired":true}'::jsonb`, targetDoc, 1},
		{`SELECT count(*) FROM canvas.node n JOIN canvas.document d ON d.id=n.document_id WHERE d.project_id=?`, live, 1},
		{`SELECT count(*) FROM media.media_asset WHERE id=? AND NOT is_delete`, asset, 1},
		{`SELECT count(*) FROM workspace.project WHERE id=? AND is_delete`, expired, 1},
	}
	for _, check := range checks {
		var n int64
		if err := db.Raw(check.sql, check.id).Scan(&n).Error; err != nil || n != check.want {
			t.Fatalf("%s: count=%d want=%d err=%v", check.sql, n, check.want, err)
		}
	}
}

type failingRetentionContent struct{ tx *gorm.DB }

func (s failingRetentionContent) ClearProjectContent(ctx context.Context, id uuid.UUID) error {
	if err := s.tx.WithContext(ctx).Exec(`UPDATE workspace.project SET description='must rollback' WHERE id=?`, id).Error; err != nil {
		return err
	}
	return errors.New("injected content failure")
}
func TestProjectRetentionFailureRollsBackContentAndRemainsRetryable(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	actor := insertWorkspaceActor(ctx, t, db, insertWorkspaceOrganization(ctx, t, db))
	project := insertWorkspaceProject(ctx, t, db, actor, "16:9", "realistic", nil)
	lifecycleFixtureSQL(t, db, `UPDATE workspace.project SET is_delete=true,revision=2,delete_time=clock_timestamp()-interval '31 days',purge_after=clock_timestamp()-interval '1 day' WHERE id=?`, project)
	store := pgworkspace.NewProjectRetentionStore(db, func(tx *gorm.DB) pgworkspace.ProjectRetentionContent { return failingRetentionContent{tx} })
	if _, err := store.EnqueueExpired(ctx, 500); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ProcessNext(ctx); err == nil {
		t.Fatal("failure reported success")
	}
	var description string
	if err := db.Raw(`SELECT description FROM workspace.project WHERE id=?`, project).Scan(&description).Error; err != nil || description != "" {
		t.Fatal("content partially committed", description, err)
	}
	var job struct {
		Status    string
		Attempts  int
		LastError string
	}
	if err := db.Raw(`SELECT status,attempts,last_error FROM workspace.project_retention_job WHERE project_id=? AND deleted_revision=2`, project).Scan(&job).Error; err != nil || job.Status != "pending" || job.Attempts != 1 || job.LastError != "content_cleanup_failed" {
		t.Fatal("failure was not durable", job, err)
	}
	if _, err := app.NewProjectRetention(db).Run(ctx, 500); err != nil {
		t.Fatal("retry failed", err)
	}
}
