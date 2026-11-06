package operation_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	pgoperation "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/postgres"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

type publicationAccess struct {
	actor  identity.Principal
	denied *bool
}

func (a publicationAccess) AuthorizeOperation(_ context.Context, actor identity.Principal, _ uuid.UUID, _ bool) error {
	if actor != a.actor || *a.denied {
		return identity.ErrForbidden
	}
	return nil
}
func TestHistoryPublicationPostgresSelectionReplayAndRevocation(t *testing.T) {
	dsn := os.Getenv("GLOWLOOM_TEST_PUBLICATION_DB_DSN")
	if dsn == "" {
		t.Skip("requires a fresh disposable publication database")
	}
	db := businessEmptyFixtureDB(t, dsn)
	var existing int64
	if err := db.Raw(`SELECT count(*) FROM information_schema.schemata WHERE schema_name IN ('workspace','operation','media')`).Scan(&existing).Error; err != nil || existing != 0 {
		t.Fatalf("fresh disposable DB required: schemas=%d err=%v", existing, err)
	}
	// Minimal query fixture allows negative provenance cases without a full operation admission.
	const ddl = `CREATE SCHEMA workspace;CREATE SCHEMA operation;CREATE SCHEMA media;` + businessSQLLocationTables + `
 CREATE TABLE workspace.project(id uuid PRIMARY KEY,org_id uuid,owner_scope_id uuid,status text DEFAULT 'active',is_delete boolean DEFAULT false);
 CREATE TABLE operation.operation(id uuid PRIMARY KEY,project_id uuid,capability text DEFAULT 'image.generate',mode text DEFAULT 'text_to_image',status text DEFAULT 'completed',is_delete boolean DEFAULT false);
 CREATE TABLE operation.operation_input(id uuid,operation_id uuid,seq_no int,role text,text_value text,is_delete boolean DEFAULT false);
 CREATE TABLE operation.operation_output(id uuid PRIMARY KEY,project_id uuid,operation_id uuid,media_asset_id uuid,moderation_status text,is_delete boolean DEFAULT false);
 CREATE TABLE operation.prompt_submission(operation_id uuid,actor_id uuid,project_id uuid);
 CREATE TABLE operation.quote_request(org_id uuid,actor_id uuid,result jsonb);
 CREATE TABLE media.media_asset(id uuid PRIMARY KEY,object_location_id uuid NOT NULL REFERENCES media.object_location(id),project_id uuid,kind text,status text,moderation_status text,is_delete boolean DEFAULT false,contains_real_person boolean DEFAULT false,consent_record_id uuid,object_key text,revision bigint DEFAULT 1);
CREATE TABLE operation.published_work (
 id uuid PRIMARY KEY,
 actor_id uuid NOT NULL,
 org_id uuid NOT NULL,
 scope_id uuid NOT NULL,
 project_id uuid NOT NULL REFERENCES workspace.project(id),
 operation_id uuid NOT NULL REFERENCES operation.operation(id),
 output_id uuid NOT NULL REFERENCES operation.operation_output(id),
 media_asset_id uuid NOT NULL REFERENCES media.media_asset(id),
 title text NOT NULL CHECK (char_length(title) BETWEEN 1 AND 120),
 description text NOT NULL DEFAULT '' CHECK (char_length(description)<=2000),
 prompt text,
 created_at timestamptz NOT NULL DEFAULT now(),
 withdrawn_at timestamptz,
 UNIQUE(actor_id,output_id)
);
CREATE TABLE operation.history_publication_receipt (
 actor_id uuid NOT NULL,
 org_id uuid NOT NULL,
 scope_id uuid NOT NULL,
 request_key uuid NOT NULL,
 request_hash text NOT NULL,
 response jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(actor_id,org_id,scope_id,request_key)
);`
	if err := db.Exec(ddl).Error; err != nil {
		t.Fatal(err)
	}
	actor := identity.Principal{ID: uuid.New(), OrgID: uuid.New(), ScopeID: uuid.New(), SessionID: uuid.New(), CredentialEpoch: 1}
	project, operation, output, sibling, media, foreignOperation, foreignOutput := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec := func(query string, args ...any) {
		t.Helper()
		if err := db.Exec(query, args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO workspace.project(id,org_id,owner_scope_id)VALUES(?,?,?)`, project, actor.OrgID, actor.ScopeID)
	exec(`INSERT INTO operation.operation(id,project_id)VALUES(?,?),(?,?)`, operation, project, foreignOperation, project)
	exec(`INSERT INTO operation.prompt_submission VALUES(?,?,?),(?,?,?)`, operation, actor.ID, project, foreignOperation, uuid.New(), project)
	exec(`INSERT INTO media.media_asset(id,project_id,kind,status,moderation_status,object_key,object_location_id) VALUES(?,?,'image','ready','passed','safe/result.png',?)`, media, project, businessFixtureLocation(t, db, "safe/result.png"))
	exec(`INSERT INTO operation.operation_output(id,project_id,operation_id,media_asset_id,moderation_status)VALUES(?,?,?,?,'passed'),(?,?,?,?,'passed'),(?,?,?,?,'passed')`, output, project, operation, media, sibling, project, operation, media, foreignOutput, project, foreignOperation, media)
	exec(`INSERT INTO operation.operation_input(id,operation_id,seq_no,role,text_value)VALUES(?,?,0,'prompt','private prompt'),(?,?,1,'reference','private reference')`, uuid.New(), operation, uuid.New(), operation)
	denied := false
	store := pgoperation.NewScopedStore(db, func(*gorm.DB) app.ProjectAccess { return publicationAccess{actor: actor, denied: &denied} }, nil)
	command := app.NewHistoryPublisher(store)
	key := uuid.New()
	input := app.HistoryPublicationInput{Title: "选中作品", Description: "说明", Targets: []app.HistoryPublicationTarget{{OperationID: operation, OutputID: output}, {OperationID: foreignOperation, OutputID: foreignOutput}}}
	receipt, err := command.Publish(context.Background(), actor, key, input)
	if err != nil {
		t.Fatal(err)
	}
	if len(receipt.Items) != 2 || receipt.Items[0].Status != "published" || receipt.Items[1].Status != "failed" || receipt.Items[0].WorkID == nil {
		t.Fatalf("receipt=%+v", receipt)
	}
	workID := *receipt.Items[0].WorkID
	file, err := store.FindPublishedWork(context.Background(), workID)
	if err != nil {
		t.Fatal(err)
	}
	if file.Work.Prompt != nil || file.Work.Title != "选中作品" || file.AssetID != media {
		t.Fatalf("public projection=%+v", file.Work)
	}
	replay, err := command.Publish(context.Background(), actor, key, input)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(receipt)
	b, _ := json.Marshal(replay)
	if string(a) != string(b) {
		t.Fatal("replay changed receipt")
	}
	input.Title = "不同正文"
	if _, err := command.Publish(context.Background(), actor, key, input); !errors.Is(err, app.ErrPublicationConflict) {
		t.Fatalf("different body err=%v", err)
	}
	input.Title = "选中作品"
	denied = true
	if _, err := command.Publish(context.Background(), actor, key, input); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("revoked replay=%v", err)
	}
	denied = false
	var count int64
	if err := db.Raw(`SELECT count(*) FROM operation.published_work`).Scan(&count).Error; err != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	input.Targets = []app.HistoryPublicationTarget{{OperationID: operation, OutputID: sibling}}
	input.PublishPrompt = true
	explicit, err := command.Publish(context.Background(), actor, uuid.New(), input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.FindPublishedWork(context.Background(), *explicit.Items[0].WorkID)
	if err != nil || second.Work.Prompt == nil || *second.Work.Prompt != "private prompt" {
		t.Fatalf("explicit prompt=%+v err=%v", second.Work, err)
	}
	exec(`UPDATE media.media_asset SET moderation_status='rejected' WHERE id=?`, media)
	if _, err := store.FindPublishedWork(context.Background(), workID); !errors.Is(err, app.ErrPublicNotFound) {
		t.Fatalf("rejected work still readable: %v", err)
	}
}
