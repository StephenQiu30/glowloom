package canvas_test

import (
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	operationwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/operation"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	operationapp "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

func assetKindOperation(t *testing.T, db *gorm.DB, actor identity.Principal, project uuid.UUID) (uuid.UUID, []uuid.UUID, []uuid.UUID) {
	t.Helper()
	op := uuid.New()
	if err := db.Exec(`INSERT INTO operation.operation(id,project_id,target_type,capability,mode,input_hash,origin,status,region)VALUES(?,?,'agent_session','text.structured','test',repeat('a',64),'agent','completed','domestic')`, op, project).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO operation.quote_request(org_id,actor_id,request_id,fingerprint,result)VALUES(?,?,?,decode(repeat('ab',32),'hex'),jsonb_build_object('operation_id',?::text))`, actor.OrgID, actor.ID, uuid.New(), op.String()).Error; err != nil {
		t.Fatal(err)
	}
	assets, outputs := []uuid.UUID{}, []uuid.UUID{}
	for i, spec := range []struct{ kind, file, mime, codec string }{
		{"image", "fixture.png", "image/png", "png"},
		{"video", "fixture.mp4", "video/mp4", "h264"},
		{"audio", "fixture.mp3", "audio/mpeg", "mp3"},
		{"document", "fixture.txt", "text/plain", "txt"},
		{"model", "fixture.glb", "model/gltf-binary", "glb2"},
	} {
		asset, output := uuid.New(), uuid.New()
		key := "projects/" + project.String() + "/" + asset.String() + "/" + spec.file
		if err := db.Exec(`INSERT INTO media.media_asset(id,project_id,kind,origin,status,object_key,file_name,mime_type,byte_size,sha256,codec,moderation_status)VALUES(?,?,?,'upload','ready',?,?,?,1,repeat('a',64),?,'passed')`, asset, project, spec.kind, key, spec.file, spec.mime, spec.codec).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Exec(`INSERT INTO operation.operation_output(id,project_id,operation_id,seq_no,kind,media_asset_id,moderation_status)VALUES(?,?,?,?,'media',?,'passed')`, output, project, op, i, asset).Error; err != nil {
			t.Fatal(err)
		}
		assets, outputs = append(assets, asset), append(outputs, output)
	}
	output := uuid.New()
	if err := db.Exec(`INSERT INTO operation.operation_output(id,project_id,operation_id,seq_no,kind,json_payload,moderation_status)VALUES(?,?,?,5,'json','{"text":"正式正文"}','passed')`, output, project, op).Error; err != nil {
		t.Fatal(err)
	}
	return op, assets, append(outputs, output)
}

func assetKindSnapshot(t *testing.T, db *gorm.DB, op uuid.UUID) string {
	t.Helper()
	var snapshot string
	if err := db.Raw(`SELECT jsonb_build_object('outputs',(SELECT jsonb_agg(to_jsonb(o) ORDER BY o.id) FROM operation.operation_output o WHERE o.operation_id=?),'assets',(SELECT jsonb_agg(to_jsonb(a) ORDER BY a.id) FROM media.media_asset a WHERE a.id IN (SELECT media_asset_id FROM operation.operation_output WHERE operation_id=?)))::text`, op, op).Scan(&snapshot).Error; err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func TestAssetOutputKindsUseCanonicalMediaFactsAndPreserveStoredRows(t *testing.T) {
	db := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, db)
	op, assets, outputs := assetKindOperation(t, db, actor, project)
	before := assetKindSnapshot(t, db, op)
	store := operationwire.ProvideOperationStore(db)
	page, err := store.ListPersonalHistory(t.Context(), actor, operationapp.PersonalHistoryInput{Kind: "document"})
	if err != nil || len(page.Items) != 1 || len(page.Items[0].Outputs) != 6 {
		t.Fatalf("history lost canonical candidates: %+v %v", page, err)
	}
	detail, err := store.ReadPublicTask(t.Context(), actor, project, op)
	if err != nil || len(detail.Outputs) != 6 {
		t.Fatalf("detail lost canonical candidates: %+v %v", detail, err)
	}
	want := []string{"image", "video", "audio", "document", "model", "json"}
	for i, kind := range want {
		history, public := page.Items[0].Outputs[i], detail.Outputs[i]
		if history.ID != outputs[i] || public.ID != outputs[i] || history.Kind != kind || public.Kind != kind {
			t.Errorf("output %d: want real %s, history=%+v detail=%+v", i, kind, history, public)
		}
		if i < len(assets) && (history.MediaAssetID == nil || *history.MediaAssetID != assets[i] || public.MediaAssetID == nil || *public.MediaAssetID != assets[i]) {
			t.Errorf("output %d lost actual media identity", i)
		}
	}
	if page.Items[0].Outputs[5].Text == nil || *page.Items[0].Outputs[5].Text != "正式正文" || detail.Outputs[5].Text == nil || *detail.Outputs[5].Text != "正式正文" {
		t.Fatal("reviewed JSON body was changed or lost")
	}
	if after := assetKindSnapshot(t, db, op); after != before {
		t.Fatal("read projections rewrote stored output or media facts")
	}
	var mediaCount int64
	if err := db.Raw(`SELECT count(*) FROM operation.operation_output WHERE operation_id=? AND kind='media'`, op).Scan(&mediaCount).Error; err != nil || mediaCount != 5 {
		t.Fatal("canonical media discriminator changed", mediaCount, err)
	}
}

func TestAssetPublicTaskPreservesUnavailableCandidateFactsAndValidSiblings(t *testing.T) {
	db := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, db)
	op, assets, outputs := assetKindOperation(t, db, actor, project)
	for _, statement := range []struct {
		sql string
		id  uuid.UUID
	}{
		{`UPDATE media.media_asset SET is_delete=true WHERE id=?`, assets[1]},
		{`UPDATE media.media_asset SET status='rejected',moderation_status='rejected' WHERE id=?`, assets[2]},
		{`UPDATE media.media_asset SET status='processing',moderation_status='pending' WHERE id=?`, assets[3]},
		{`UPDATE operation.operation_output SET moderation_status='pending' WHERE id=?`, outputs[4]},
	} {
		if err := db.Exec(statement.sql, statement.id).Error; err != nil {
			t.Fatal(err)
		}
	}
	_, foreignProject := seedCanvasActor(t, db)
	foreign := seedCanvasMedia(t, db, foreignProject, "image")
	if err := db.Exec(`INSERT INTO operation.operation_output(id,project_id,operation_id,seq_no,kind,media_asset_id,moderation_status)VALUES(?,?,?,6,'media',?,'passed')`, uuid.New(), project, op, foreign).Error; err == nil {
		t.Fatal("canonical same-project media ownership constraint was lost")
	}
	objects := &downloadFixtureObjects{}
	downloads := mediaapp.NewAssetDownloads(pgmedia.NewScopedStore(db, mediawire.NewMediaScopeAccess), nil, objects)
	for _, id := range []uuid.UUID{assets[1], assets[2]} {
		if file, err := downloads.Open(t.Context(), actor, project, id); file != nil || err == nil {
			t.Fatal("candidate metadata granted unavailable media bytes", file, err)
		}
	}
	if objects.calls != 0 {
		t.Fatal("unavailable candidate opened physical bytes")
	}
	before := assetKindSnapshot(t, db, op)
	detail, err := operationwire.ProvideOperationStore(db).ReadPublicTask(t.Context(), actor, project, op)
	if err != nil || len(detail.Outputs) != 6 {
		t.Fatalf("unavailable media invalidated task or lost candidate facts: %+v %v", detail, err)
	}
	for i, expected := range []struct {
		id         uuid.UUID
		kind, mark string
	}{{outputs[0], "image", "passed"}, {outputs[1], "video", "passed"}, {outputs[2], "audio", "passed"}, {outputs[3], "document", "passed"}, {outputs[4], "model", "pending"}, {outputs[5], "json", "passed"}} {
		got := detail.Outputs[i]
		if got.ID != expected.id || got.Kind != expected.kind || got.ModerationStatus != expected.mark {
			t.Errorf("candidate %d changed: %+v", i, got)
		}
	}
	page, err := operationwire.ProvideOperationStore(db).ListPersonalHistory(t.Context(), actor, operationapp.PersonalHistoryInput{Kind: "document"})
	if err != nil || len(page.Items) != 1 || len(page.Items[0].Outputs) != 6 {
		t.Fatalf("history lost unavailable candidate facts: %+v %v", page, err)
	}
	for _, i := range []int{1, 2, 3, 4} {
		candidate := page.Items[0].Outputs[i]
		if candidate.MediaAssetID != nil || candidate.CanDownload || candidate.CanPublish {
			t.Errorf("unavailable media gained content action: %+v", candidate)
		}
	}
	if detail.Outputs[5].Text == nil || *detail.Outputs[5].Text != "正式正文" || assetKindSnapshot(t, db, op) != before {
		t.Fatal("partial media availability changed JSON body or stored evidence")
	}
}
