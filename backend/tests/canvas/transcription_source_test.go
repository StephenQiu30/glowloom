package canvas_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/canvas/application"
	"github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
)

func TestTranscriptionSourceFreezesFormalNodeAndLocksUntilOuterCommit(t *testing.T) {
	database := canvasDB(t, "LV_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, database)
	service := application.NewService(canvasStore(database))
	doc, err := service.Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "真实转写输入"})
	if err != nil {
		t.Fatal(err)
	}
	node := resourceNode("video")
	asset := seedCanvasMedia(t, database, project, "video")
	node.RefID = &asset
	saved, err := service.Execute(t.Context(), actor, doc.ID, uuid.NewString(), application.CommandsInput{ExpectedRevision: doc.Revision, Commands: []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{node}}}})
	if err != nil {
		t.Fatal(err)
	}
	before, err := service.Get(t.Context(), actor, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	reader := application.NewTranscriptionSourceReader(canvasStore(database))
	frozen, err := reader.FreezeTranscriptionSource(t.Context(), actor, project, doc.ID, node.ID, saved.Revision)
	if err != nil || frozen != asset {
		t.Fatalf("formal source: %v %v", frozen, err)
	}
	after, err := service.Get(t.Context(), actor, doc.ID)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("reader changed canvas: %v", err)
	}
	if err := database.Transaction(func(tx *gorm.DB) error {
		_, err := application.NewTranscriptionSourceReader(canvasStore(tx)).FreezeTranscriptionSource(t.Context(), actor, project, doc.ID, node.ID, saved.Revision)
		if err != nil {
			return err
		}
		for _, probe := range []struct {
			sql string
			id  uuid.UUID
		}{
			{`SELECT 1 FROM workspace.project WHERE id=? FOR UPDATE NOWAIT`, project},
			{`SELECT 1 FROM canvas.document WHERE id=? FOR UPDATE NOWAIT`, doc.ID},
			{`SELECT 1 FROM media.media_asset WHERE id=? FOR UPDATE NOWAIT`, asset},
		} {
			var one int
			err := database.Transaction(func(other *gorm.DB) error { return other.Raw(probe.sql, probe.id).Scan(&one).Error })
			var locked *pgconn.PgError
			if !errors.As(err, &locked) || locked.Code != "55P03" {
				t.Fatalf("source lock released: %v", err)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	_, err = reader.FreezeTranscriptionSource(t.Context(), actor, project, doc.ID, node.ID, saved.Revision-1)
	var stale *application.RevisionConflict
	if !errors.As(err, &stale) {
		t.Fatalf("stale source accepted: %v", err)
	}
	_, foreign := seedCanvasActor(t, database)
	_, err = reader.FreezeTranscriptionSource(t.Context(), actor, foreign, doc.ID, node.ID, saved.Revision)
	if !errors.Is(err, application.ErrNotFound) {
		t.Fatalf("foreign source accepted: %v", err)
	}
	if err := database.Exec(`UPDATE media.media_asset SET status='processing',moderation_status='pending' WHERE id=?`, asset).Error; err != nil {
		t.Fatal(err)
	}
	_, err = reader.FreezeTranscriptionSource(t.Context(), actor, project, doc.ID, node.ID, saved.Revision)
	if !errors.Is(err, mediaapp.ErrNotFound) {
		t.Fatalf("unreviewed source accepted: %v", err)
	}
}

func TestTranscriptionSourceRejectsImageAndKindChangedAfterSave(t *testing.T) {
	database := canvasDB(t, "LV_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, database)
	service := application.NewService(canvasStore(database))
	doc, err := service.Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "转写类型守卫"})
	if err != nil {
		t.Fatal(err)
	}
	image, audio := resourceNode("image"), resourceNode("audio")
	imageID, audioID := seedCanvasMedia(t, database, project, "image"), seedCanvasMedia(t, database, project, "audio")
	image.RefID, audio.RefID = &imageID, &audioID
	saved, err := service.Execute(t.Context(), actor, doc.ID, uuid.NewString(), application.CommandsInput{ExpectedRevision: doc.Revision, Commands: []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{image, audio}}}})
	if err != nil {
		t.Fatal(err)
	}
	reader := application.NewTranscriptionSourceReader(canvasStore(database))
	_, err = reader.FreezeTranscriptionSource(t.Context(), actor, project, doc.ID, image.ID, saved.Revision)
	if !errors.Is(err, domain.ErrInvalidCommand) {
		t.Fatalf("image source accepted: %v", err)
	}
	id, err := reader.FreezeTranscriptionSource(t.Context(), actor, project, doc.ID, audio.ID, saved.Revision)
	if err != nil || id != audioID {
		t.Fatalf("audio source: %v %v", id, err)
	}
	if err := database.Exec(`UPDATE media.media_asset SET kind='video',object_key=replace(object_key,'/audio/','/video/') WHERE id=?`, audioID).Error; err != nil {
		t.Fatal(err)
	}
	_, err = reader.FreezeTranscriptionSource(t.Context(), actor, project, doc.ID, audio.ID, saved.Revision)
	if !errors.Is(err, domain.ErrInvalidCommand) {
		t.Fatalf("changed source kind accepted: %v", err)
	}
}
