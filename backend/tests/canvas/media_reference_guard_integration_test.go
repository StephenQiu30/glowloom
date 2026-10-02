package canvas_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pgcanvas "github.com/StephenQiu30/lanverse/backend/internal/canvas/adapter/postgres"
	"github.com/StephenQiu30/lanverse/backend/internal/canvas/application"
	"github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
)

func canvasReferenceCheck(t *testing.T, db *gorm.DB, actor identityapp.Principal, project, asset uuid.UUID) (bool, error) {
	t.Helper()
	var found bool
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE lanverse_app`).Error; err != nil {
			return err
		}
		var err error
		found, err = pgcanvas.NewMediaReferenceGuard(tx).HasMediaReferences(t.Context(), actor, project, asset)
		return err
	})
	return found, err
}
func TestCanvasMediaReferenceGuardRetainsSoftRemovedNodesAndCompleteConfigHistory(t *testing.T) {
	db := canvasDB(t, "LV_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, db)
	doc, err := application.NewService(canvasStore(db)).Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "完整引用保护"})
	if err != nil {
		t.Fatal(err)
	}
	assets := []uuid.UUID{uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()}
	config, err := json.Marshal(domain.NodeConfig{Text: ptr("历史保留")})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO canvas.node(id,document_id,title,node_type,node_action,ref_type,ref_id,config,x,y,is_delete) VALUES(?,?,'已移除','image','resource','media_asset',?,?::jsonb,0,0,true)`, uuid.New(), doc.ID, assets[0], string(config)).Error; err != nil {
		t.Fatal(err)
	}
	generation := generationNode().Config
	generation.Generation.Inputs = []domain.GenerationReference{{Role: "reference", MediaAssetID: assets[1]}}
	timeline := timelineNode().Config
	timeline.Timeline.Clips[0].AssetID = &assets[2]
	director := directorNode().Config
	director.Director.Cover = &domain.DirectorCover{AssetID: assets[3], ShotID: director.Director.Shots[0].ID}
	director.Director.Panorama = &domain.DirectorPanorama{AssetID: assets[4]}
	director.Director.Objects[0].AssetID = &assets[5]
	shot := directorScreenshot()
	shot.AssetID = assets[6]
	director.Director.Shots[0].Screenshots = []domain.DirectorScreenshot{shot}
	for i, cfg := range []domain.NodeConfig{generation, timeline, director} {
		commands, err := json.Marshal([]domain.Command{{Type: "UpdateNodeConfig", ID: uuid.New(), Config: &cfg}})
		if err != nil {
			t.Fatal(err)
		}
		if err := db.Exec(`INSERT INTO canvas.command_log(id,document_id,revision,commands,is_delete) VALUES(?,?,?,?::jsonb,true)`, uuid.New(), doc.ID, i+2, string(commands)).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Exec(`UPDATE canvas.document SET is_delete=true WHERE id=?`, doc.ID).Error; err != nil {
		t.Fatal(err)
	}
	for _, asset := range assets {
		found, err := canvasReferenceCheck(t, db, actor, project, asset)
		if err != nil || !found {
			t.Fatal("lost retained own reference", asset, found, err)
		}
	}
	if found, err := canvasReferenceCheck(t, db, actor, project, uuid.New()); err != nil || found {
		t.Fatal("unreferenced asset", found, err)
	}
}
func TestCanvasMediaReferenceGuardRequiresCallerTransactionAndCurrentScope(t *testing.T) {
	db := canvasDB(t, "LV_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, db)
	if _, err := pgcanvas.NewMediaReferenceGuard(db).HasMediaReferences(context.Background(), actor, project, uuid.New()); err == nil {
		t.Fatal("pool accepted as retained transaction")
	}
	foreign, _ := seedCanvasActor(t, db)
	if _, err := canvasReferenceCheck(t, db, foreign, project, uuid.New()); !errors.Is(err, application.ErrNotFound) {
		t.Fatal("foreign scope", err)
	}
	if err := db.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := canvasReferenceCheck(t, db, actor, project, uuid.New()); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatal("disabled caller", err)
	}
}
func TestCanvasMediaReferenceGuardCorruptHistoricalConfigFailsClosed(t *testing.T) {
	db := canvasDB(t, "LV_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, db)
	doc, err := application.NewService(canvasStore(db)).Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "损坏历史"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO canvas.command_log(id,document_id,revision,commands) VALUES(?,?,2,'[{"type":"UpdateNodeConfig","config":{"undeclared_asset":"not-a-proof"}}]'::jsonb)`, uuid.New(), doc.ID).Error; err != nil {
		t.Fatal(err)
	}
	if found, err := canvasReferenceCheck(t, db, actor, project, uuid.New()); err == nil {
		t.Fatal("corrupt history became absence", found)
	}
}

func TestCanvasMediaReferenceGuardBoundsCombinedRetainedHistory(t *testing.T) {
	db := canvasDB(t, "LV_TEST_CANVAS_DB_DSN")
	var name string
	if err := db.Raw(`SELECT current_database()`).Scan(&name).Error; err != nil || name != "lanverse_reference" {
		t.Fatal("budget guard requires isolated lanverse_reference", name, err)
	}
	for _, test := range []struct {
		name                 string
		nodes, logs, padding int
	}{
		{"node_body", 1, 0, 1 << 20},
		{"command_body", 0, 1, 1 << 20},
		{"combined_bytes", 60, 60, 600 << 10},
		{"combined_facts", 25000, 25001, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			actor, project := seedCanvasActor(t, db)
			doc, err := application.NewService(canvasStore(db)).Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "历史预算"})
			if err != nil {
				t.Fatal(err)
			}
			asset := uuid.New()
			if found, err := canvasReferenceCheck(t, db, actor, project, asset); err != nil || found {
				t.Fatal("valid empty retained canvas", found, err)
			}
			config, err := json.Marshal(domain.NodeConfig{Text: ptr(strings.Repeat("a", test.padding))})
			if err != nil {
				t.Fatal(err)
			}
			commands, err := json.Marshal([]map[string]any{{"type": "rename", "name": strings.Repeat("a", test.padding)}})
			if err != nil {
				t.Fatal(err)
			}
			tx := db.WithContext(t.Context()).Begin()
			if tx.Error != nil {
				t.Fatal(tx.Error)
			}
			defer func() {
				if err := tx.Rollback().Error; err != nil {
					t.Error(err)
				}
			}()
			if test.nodes != 0 {
				read := tx.Exec(`INSERT INTO canvas.node(id,document_id,title,node_type,node_action,ref_type,config,x,y,is_delete)
 SELECT gen_random_uuid(),?,'历史文本','text','resource','',?::jsonb,0,0,true FROM generate_series(1,?)`, doc.ID, string(config), test.nodes)
				if read.Error != nil || read.RowsAffected != int64(test.nodes) {
					t.Fatal("owned retained node metadata", read.RowsAffected, read.Error)
				}
			}
			if test.logs != 0 {
				read := tx.Exec(`INSERT INTO canvas.command_log(id,document_id,revision,commands,is_delete)
 SELECT gen_random_uuid(),?,position+1,?::jsonb,true FROM generate_series(1,?) AS position`, doc.ID, string(commands), test.logs)
				if read.Error != nil || read.RowsAffected != int64(test.logs) {
					t.Fatal("owned retained command metadata", read.RowsAffected, read.Error)
				}
			}
			if err := tx.Exec(`SET LOCAL ROLE lanverse_app`).Error; err != nil {
				t.Fatal(err)
			}
			var role string
			if err := tx.Raw(`SELECT current_user`).Scan(&role).Error; err != nil || role != "lanverse_app" {
				t.Fatal("history guard lacks actual nonowner", role, err)
			}
			found, err := pgcanvas.NewMediaReferenceGuard(tx).HasMediaReferences(t.Context(), actor, project, asset)
			if err == nil || found {
				t.Fatal("oversized combined retained history became no reference", found, err)
			}
		})
	}
}

func TestCanvasMediaReferenceGuardRetainsHistoryBudgetLock(t *testing.T) {
	db := canvasDB(t, "LV_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, db)
	err := db.WithContext(t.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE lanverse_app`).Error; err != nil {
			return err
		}
		if found, err := pgcanvas.NewMediaReferenceGuard(tx).HasMediaReferences(t.Context(), actor, project, uuid.New()); err != nil || found {
			t.Fatal("authorized empty retained history", found, err)
		}
		// Current canvas writers take SHARE before any graph/history mutation.
		// They must remain blocked until the caller's complete admission ends.
		ctx, cancel := context.WithTimeout(t.Context(), 250*time.Millisecond)
		defer cancel()
		writerErr := db.WithContext(ctx).Transaction(func(writer *gorm.DB) error {
			if err := writer.Exec(`SET LOCAL ROLE lanverse_app`).Error; err != nil {
				return err
			}
			var role string
			if err := writer.Raw(`SELECT current_user`).Scan(&role).Error; err != nil || role != "lanverse_app" {
				t.Fatal("history writer lacks actual nonowner", role, err)
			}
			var locked int
			return writer.Raw(`SELECT 1 FROM workspace.project WHERE id=? AND org_id=? FOR SHARE`, project, actor.OrgID).Scan(&locked).Error
		})
		if !errors.Is(writerErr, context.DeadlineExceeded) {
			t.Fatal("writer can commit history after preflight before caller finishes", writerErr)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if found, err := canvasReferenceCheck(t, db, actor, project, uuid.New()); err != nil || found {
		t.Fatal("owning history lock was not released with caller transaction", found, err)
	}
}
