package media_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
)

func TestDocumentSchemaPreservesLegacyRowsButAllOwningReadsAndCopyFailClosed(t *testing.T) {
	db, owner := documentTestDB(t)
	actor, project := mediaStoreProject(t, owner)
	id := uuid.New()
	// Recover the actual schema constraint instead of retaining a second DDL source.
	// The transaction prevents other sessions from observing the relaxed fixture.
	if err := owner.Transaction(func(tx *gorm.DB) error {
		var definition string
		if err := tx.Raw(`SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conrelid='media.media_asset'::regclass AND conname='media_asset_document_facts_check'`).Scan(&definition).Error; err != nil {
			return err
		}
		if definition == "" || !strings.HasSuffix(definition, " NOT VALID") {
			return errors.New("legacy fixture requires the schema document NOT VALID constraint")
		}
		if err := tx.Exec(`ALTER TABLE media.media_asset DROP CONSTRAINT media_asset_document_facts_check`).Error; err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO media.media_asset(id,project_id,kind,origin,status,object_key,file_name,mime_type,byte_size,moderation_status) VALUES(?,?,'document','upload','ready',?,'legacy.pdf','application/pdf',1,'passed')`, id, project, "projects/"+project.String()+"/document/2026/10/"+id.String()+".pdf").Error; err != nil {
			return err
		}
		return tx.Exec(`ALTER TABLE media.media_asset ADD CONSTRAINT media_asset_document_facts_check ` + definition).Error
	}); err != nil {
		t.Fatal("restore schema document constraint around legacy fixture", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := owner.WithContext(ctx).Exec(`DELETE FROM media.upload_request WHERE asset_id=? AND project_id=?`, id, project).Error; err != nil {
			t.Error("cleanup exact legacy receipt fixture", err)
		}
		if err := owner.WithContext(ctx).Exec(`DELETE FROM media.media_asset WHERE id=? AND project_id=?`, id, project).Error; err != nil {
			t.Error("cleanup exact legacy fixture", err)
		}
	})
	var validated bool
	if err := owner.Raw(`SELECT convalidated FROM pg_constraint WHERE conrelid='media.media_asset'::regclass AND conname='media_asset_document_facts_check'`).Scan(&validated).Error; err != nil || validated {
		t.Fatal("legacy rows unexpectedly validated or removed", err)
	}
	if err := owner.Exec(`UPDATE media.media_asset SET revision=revision+1 WHERE id=?`, id).Error; err == nil {
		t.Fatal("new invalid document write bypassed NOT VALID check")
	}
	key := uuid.New()
	prior := mediaapp.UploadResult{Asset: mediaapp.AssetSummary{ID: id, ProjectID: project, Kind: "document", FileName: "legacy.pdf", MIMEType: "application/pdf", ByteSize: 1, Revision: 1}}
	response, err := json.Marshal(prior)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Exec(`INSERT INTO media.upload_request(project_id,principal_id,request_key,sha256,file_name,byte_size,asset_id,response) VALUES(?,?,?,?,?,1,?,?::jsonb)`, project, actor.ID, key, strings.Repeat("a", 64), "legacy.pdf", id, string(response)).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := pgmedia.NewStore(db).FindUpload(t.Context(), actor, mediaapp.UploadRequest{ProjectID: project, Key: key, RequestID: uuid.New(), SHA256: strings.Repeat("a", 64), FileName: "legacy.pdf", ByteSize: 1}); !errors.Is(err, mediaapp.ErrUnavailable) {
		t.Fatal("legacy invalid document upload receipt bypassed owner closure", err)
	}
	reader := mediaapp.NewDocumentSources(pgmedia.NewDocumentSourceStore(db), nil)
	if _, err := reader.Freeze(t.Context(), actor, project, []uuid.UUID{id}); !errors.Is(err, mediaapp.ErrUnavailable) {
		t.Fatal("legacy invalid document silently exposed", err)
	}
	query := mediaapp.NewAssetQuery(pgmedia.NewStore(db), nil)
	if _, err := query.List(t.Context(), actor, project, "document", "", 50); err == nil {
		t.Fatal("legacy invalid document silently listed or skipped")
	}
	binding := mediaapp.ProjectCopyBinding{JobID: uuid.New(), OrgID: actor.OrgID, SourceProjectID: project, TargetProjectID: uuid.New()}
	if err := owner.Exec(`INSERT INTO workspace.project(id,org_id,name,aspect_ratio,style_type,status) VALUES(?,?,'legacy复制拒绝目标','16:9','realistic','copying')`, binding.TargetProjectID, actor.OrgID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		_, err := pgmedia.NewProjectCopyStore(tx).Freeze(t.Context(), actor, binding, time.Now())
		return err
	}); !errors.Is(err, mediaapp.ErrProjectCopyMediaUnavailable) {
		t.Fatal("copy silently dropped legacy source", err)
	}
	var count int64
	if err := owner.Raw(`SELECT count(*) FROM media.media_asset WHERE id=? AND project_id=? AND NOT is_delete`, id, project).Scan(&count).Error; err != nil || count != 1 {
		t.Fatal("failing reads mutated legacy record", count, err)
	}
}
