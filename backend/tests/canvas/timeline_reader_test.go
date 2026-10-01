package canvas_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/canvas/application"
	"github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
)

func TestTimelineReaderFreezesOwnedMediaAndRejectsStaleRevision(t *testing.T) {
	database := canvasDB(t, "LV_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, database)
	store := canvasStore(database)
	service := application.NewService(store)
	doc, err := service.Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "导出冻结输入"})
	if err != nil {
		t.Fatal(err)
	}
	image, timeline := resourceNode("image"), timelineNode()
	asset := seedCanvasMedia(t, database, project, "image")
	image.RefID = &asset
	timeline.Config.Timeline.Tracks[0].Kind = "image"
	timeline.Config.Timeline.Clips[0].Kind = "image"
	timeline.Config.Timeline.Clips[0].NodeID = &image.ID
	result, err := service.Execute(t.Context(), actor, doc.ID, uuid.NewString(), application.CommandsInput{ExpectedRevision: doc.Revision, Commands: []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{image, timeline}}}})
	if err != nil {
		t.Fatal(err)
	}
	reader := application.NewTimelineReader(store)
	frozen, err := reader.FreezeTimeline(t.Context(), actor, project, doc.ID, timeline.ID, result.Revision)
	if err != nil || frozen.Clips[0].AssetID == nil || *frozen.Clips[0].AssetID != asset {
		t.Fatalf("asset not frozen: %v", err)
	}
	loaded, err := service.Get(t.Context(), actor, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range loaded.Nodes {
		if node.ID == timeline.ID && node.Config.Timeline.Clips[0].AssetID != nil {
			t.Fatal("freeze changed the saved canvas")
		}
	}
	_, err = reader.FreezeTimeline(t.Context(), actor, project, doc.ID, timeline.ID, result.Revision-1)
	var conflict *application.RevisionConflict
	if !errors.As(err, &conflict) {
		t.Fatalf("stale revision accepted: %v", err)
	}
	_, foreign := seedCanvasActor(t, database)
	_, err = reader.FreezeTimeline(t.Context(), actor, foreign, doc.ID, timeline.ID, result.Revision)
	if !errors.Is(err, application.ErrNotFound) {
		t.Fatalf("different project accepted: %v", err)
	}
	if err := database.Exec(`UPDATE media.media_asset SET width=256,height=192 WHERE id=?`, asset).Error; err != nil {
		t.Fatal(err)
	}
	timeline.Config.Timeline.Clips[0].Crop = &domain.TimelineCrop{X: 200, Y: 0, Width: 100, Height: 100}
	result, err = service.Execute(t.Context(), actor, doc.ID, uuid.NewString(), application.CommandsInput{ExpectedRevision: result.Revision, Commands: []domain.Command{{Type: "UpdateNodeConfig", ID: timeline.ID, Config: &timeline.Config}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reader.FreezeTimeline(t.Context(), actor, project, doc.ID, timeline.ID, result.Revision); !errors.Is(err, domain.ErrInvalidCommand) {
		t.Fatalf("crop outside actual media dimensions accepted: %v", err)
	}
	timeline.Config.Timeline.Clips[0].Crop.X = 100
	result, err = service.Execute(t.Context(), actor, doc.ID, uuid.NewString(), application.CommandsInput{ExpectedRevision: result.Revision, Commands: []domain.Command{{Type: "UpdateNodeConfig", ID: timeline.ID, Config: &timeline.Config}}})
	if err != nil {
		t.Fatal(err)
	}
	// Nested GORM transactions must keep the input locks until the export's
	// surrounding transaction commits, including after the reader returns.
	if err := database.Transaction(func(tx *gorm.DB) error {
		frozen, err := application.NewTimelineReader(canvasStore(tx)).FreezeTimeline(t.Context(), actor, project, doc.ID, timeline.ID, result.Revision)
		if err != nil {
			return err
		}
		if frozen.Clips[0].Crop == nil || frozen.Clips[0].Crop.X != 100 {
			t.Fatal("valid crop not frozen")
		}
		for _, probe := range []struct {
			query string
			id    uuid.UUID
		}{
			{`SELECT 1 FROM canvas.document WHERE id=? FOR UPDATE NOWAIT`, doc.ID},
			{`SELECT 1 FROM media.media_asset WHERE id=? FOR UPDATE NOWAIT`, asset},
		} {
			var one int
			err := database.Transaction(func(other *gorm.DB) error { return other.Raw(probe.query, probe.id).Scan(&one).Error })
			var locked *pgconn.PgError
			if !errors.As(err, &locked) || locked.Code != "55P03" {
				t.Fatalf("frozen input lock released before outer commit: %v", err)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err = database.Exec(`UPDATE media.media_asset SET moderation_status='pending' WHERE id=?`, asset).Error; err != nil {
		t.Fatal(err)
	}
	_, err = reader.FreezeTimeline(t.Context(), actor, project, doc.ID, timeline.ID, result.Revision)
	if err == nil {
		t.Fatal("unreviewed media accepted for export")
	}
}
