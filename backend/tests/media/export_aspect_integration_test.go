package media_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	canvasdomain "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	exportapp "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
)

func TestTimelineAutoVideoExportRejectsBeforeJobCreation(t *testing.T) {
	database := mediatoolFixtureDB(t)
	actor, project := mediaStoreProject(t, database)
	timeline := exportTimeline(t)
	timeline.AspectRatio = "auto"
	reader := &fixedTimeline{timeline: timeline}
	store := mediatoolExportStore(database, func(*gorm.DB) exportapp.TimelineReader { return reader }, func(tx *gorm.DB) exportapp.DerivedMedia { return mediaapp.NewDerivedService(mediaTestStore(tx)) })
	_, err := store.Create(t.Context(), actor, project, uuid.New(), exportapp.CreateInput{CanvasID: uuid.New(), NodeID: uuid.New(), Revision: 1})
	if !errors.Is(err, canvasdomain.ErrTimelineAspectUnresolved) || !errors.Is(err, exportapp.ErrInvalidExport) {
		t.Fatal("unresolved automatic video accepted", err)
	}
	var jobs int64
	if err := database.Raw(`SELECT count(*) FROM mediatool.export_job WHERE project_id=?`, project).Scan(&jobs).Error; err != nil || jobs != 0 {
		t.Fatal("unresolved automatic intent queued a job", jobs, err)
	}
}
