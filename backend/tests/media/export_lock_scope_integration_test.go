package media_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	toolpg "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/postgres"
	toolapp "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
)

type exportAuthorityTrace struct {
	toolapp.DerivedMedia
	events *[]string
}

func (a exportAuthorityTrace) AuthorizeProject(ctx context.Context, actor identityapp.Principal, project uuid.UUID, write bool) error {
	if err := a.DerivedMedia.AuthorizeProject(ctx, actor, project, write); err != nil {
		return err
	}
	*a.events = append(*a.events, "authority")
	return nil
}

// Real authority and SQL locks are observed without rendering or object I/O.
// Every synthetic caption job, account and command is rolled back together.
func TestExportWorkerLocksAuthorityBeforeJob(t *testing.T) {
	database := mediaStoreDB(t)
	tx := database.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer func() {
		if err := tx.Rollback().Error; err != nil {
			t.Error("roll back synthetic export locks", err)
		}
	}()
	actor, project := mediaStoreProject(t, tx)
	timeline := exportTimeline(t)
	timeline.Tracks[0].Kind, timeline.Clips[0].Kind = "subtitle", "subtitle"
	timeline.Clips[0].AssetID, timeline.Clips[0].Text = nil, "Caption fixture"
	reader := func(*gorm.DB) toolapp.TimelineReader { return &fixedTimeline{timeline: timeline} }
	requests := toolpg.NewStore(tx, reader, func(db *gorm.DB) toolapp.DerivedMedia {
		return mediaapp.NewDerivedService(mediaTestStore(db))
	})
	job, err := requests.Create(t.Context(), actor, project, uuid.New(), toolapp.CreateInput{CanvasID: uuid.New(), NodeID: uuid.New(), Revision: 1})
	if err != nil {
		t.Fatal(err)
	}
	var events []string
	callback := "export-lock-order-" + uuid.NewString()
	if err := tx.Callback().Row().After("gorm:row").Register(callback, func(query *gorm.DB) {
		sql := query.Statement.SQL.String()
		if strings.Contains(sql, "FROM mediatool.export_job") && strings.Contains(sql, "FOR UPDATE") {
			events = append(events, "job-lock")
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := tx.Callback().Row().Remove(callback); err != nil {
			t.Error(err)
		}
	})
	workers := toolpg.NewStore(tx, reader, func(db *gorm.DB) toolapp.DerivedMedia {
		return exportAuthorityTrace{DerivedMedia: mediaapp.NewDerivedService(mediaWorkerStore(db)), events: &events}
	})
	check := func(name string) {
		t.Helper()
		if len(events) < 2 || events[0] != "authority" || !slices.Contains(events, "job-lock") {
			t.Fatal("worker acquired a job lock before current authority", name, events)
		}
		events = nil
	}
	work := toolapp.WorkID{JobID: job.ID, Attempt: 1, ExecutionID: uuid.New()}
	if _, err := workers.Claim(t.Context(), work); err != nil {
		t.Fatal(err)
	}
	check("claim")
	if err := workers.Progress(t.Context(), work, 20, "rendering"); err != nil {
		t.Fatal(err)
	}
	check("progress")
	if err := workers.AuthorizeObjectWrite(t.Context(), work); err != nil {
		t.Fatal(err)
	}
	check("object write")
	if _, err := workers.Commit(t.Context(), work, mediadomain.MediaAsset{ProjectID: project, Kind: mediadomain.KindVideo}, nil); !errors.Is(err, mediadomain.ErrInvalidMediaAsset) {
		t.Fatal("malformed output entered owning metadata", err)
	}
	check("commit")
	current, err := requests.Get(t.Context(), actor, project, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := requests.Control(t.Context(), actor, project, job.ID, uuid.New(), current.Revision, "cancel"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	events = nil
	if err := workers.Finish(t.Context(), work, true, ""); err != nil {
		t.Fatal("revocation prevented the real owner recording cessation", err)
	}
	if slices.Contains(events, "authority") {
		t.Fatal("cessation gained a current-permission prerequisite", events)
	}
	var stopped struct {
		Status       string
		ActiveWorker *uuid.UUID
	}
	if err := tx.Raw(`SELECT status,active_worker FROM mediatool.export_job WHERE id=?`, job.ID).Scan(&stopped).Error; err != nil || stopped.Status != "cancelled" || stopped.ActiveWorker != nil {
		t.Fatal("owning cessation was not durably recorded", stopped, err)
	}
}
