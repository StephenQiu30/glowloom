package media_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	toolpg "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/postgres"
	toolapp "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
	tooldomain "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/domain"
)

type transcriptionLockSource struct{ asset uuid.UUID }

func (s transcriptionLockSource) FreezeTranscriptionSource(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID, uuid.UUID, int64) (uuid.UUID, error) {
	return s.asset, nil
}

type transcriptionPermissionTrace struct {
	toolapp.DerivedMedia
	events         *[]string
	afterAuthorize func() error
}

func (m transcriptionPermissionTrace) AuthorizeProject(ctx context.Context, actor identityapp.Principal, project uuid.UUID, write bool) error {
	if err := m.DerivedMedia.AuthorizeProject(ctx, actor, project, write); err != nil {
		return err
	}
	*m.events = append(*m.events, "permission")
	if m.afterAuthorize != nil {
		return m.afterAuthorize()
	}
	return nil
}

func transcriptionLockFixture(t *testing.T) (*gorm.DB, identityapp.Principal, uuid.UUID, *toolpg.TranscriptionStore, toolapp.TranscriptionWorkID, *[]string) {
	t.Helper()
	tx := mediaStoreDB(t).Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	t.Cleanup(func() {
		if err := tx.Rollback().Error; err != nil && !errors.Is(err, sql.ErrTxDone) {
			t.Error("roll back synthetic transcription lock fixtures", err)
		}
	})
	actor, project := mediaStoreProject(t, tx)
	asset := imageAsset(project, uuid.New())
	asset.Kind, asset.MimeType, asset.FileName = mediadomain.KindAudio, "audio/mp4", "lock.m4a"
	asset.ObjectKey = "projects/" + project.String() + "/audio/2026/10/" + asset.ID.String() + ".m4a"
	duration := int32(1000)
	sha := strings.Repeat("a", 64)
	asset.DurationMS, asset.SHA256 = &duration, &sha
	asset.Status, asset.ModerationStatus = mediadomain.StatusProcessing, mediadomain.ModerationPending
	if err := mediaTestStore(tx).CreateAsset(t.Context(), actor, asset); err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`UPDATE media.media_asset SET status='ready',moderation_status='passed' WHERE id=?`, asset.ID).Error; err != nil {
		t.Fatal(err)
	}
	requests := toolpg.NewTranscriptionStore(tx, func(*gorm.DB) toolapp.TranscriptionSourceReader {
		return transcriptionLockSource{asset: asset.ID}
	}, func(db *gorm.DB) toolapp.DerivedMedia {
		return mediaapp.NewDerivedService(pgmedia.NewScopedStore(db, app.NewMediaScopeAccess))
	})
	job, err := requests.Create(t.Context(), actor, project, uuid.New(), toolapp.TranscriptionCreateInput{CanvasID: uuid.New(), NodeID: uuid.New(), Revision: 1, Language: "auto"})
	if err != nil {
		t.Fatal(err)
	}
	var events []string
	workers := toolpg.NewTranscriptionStore(tx, nil, func(db *gorm.DB) toolapp.DerivedMedia {
		return transcriptionPermissionTrace{DerivedMedia: mediaapp.NewDerivedService(pgmedia.NewScopedStore(db, app.NewMediaWorkerScopeAccess)), events: &events}
	})
	callback := "test:transcription-job-lock-" + job.ID.String()
	if err := tx.Callback().Row().After("gorm:row").Register(callback, func(query *gorm.DB) {
		sql := query.Statement.SQL.String()
		if !strings.Contains(sql, "mediatool.transcription_job") || !strings.Contains(sql, "FOR UPDATE") {
			return
		}
		for _, value := range query.Statement.Vars {
			if id, ok := value.(uuid.UUID); ok && id == job.ID {
				events = append(events, "job")
				return
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := tx.Callback().Row().Remove(callback); err != nil {
			t.Error("remove transcription lock trace", err)
		}
	})
	return tx, actor, project, workers, toolapp.TranscriptionWorkID{JobID: job.ID, Attempt: 1, ExecutionID: uuid.New()}, &events
}

func transcriptionLockDraft() tooldomain.Transcript {
	return tooldomain.Transcript{Version: 1, Language: "english", DurationMS: 1000, Segments: []tooldomain.SubtitleSegment{{StartMS: 0, EndMS: 1000, Text: "Synthetic terminal response"}}}
}

// Every row, command, outbox event and audit remains inside the outer rollback.
// The trace forwards the real production worker authority; no native or object I/O runs.
func TestTranscriptionWorkerPermissionLocksPrecedeJobLocks(t *testing.T) {
	_, _, _, workers, owner, events := transcriptionLockFixture(t)
	check := func(name string) {
		t.Helper()
		if len(*events) < 2 || (*events)[0] != "permission" || (*events)[1] != "job" {
			t.Fatalf("%s must acquire real worker permission before the job row: %v", name, *events)
		}
		*events = nil
	}
	if _, err := workers.Claim(t.Context(), owner); err != nil {
		t.Fatal(err)
	}
	check("claim")
	if err := workers.StartInference(t.Context(), owner); err != nil {
		t.Fatal(err)
	}
	check("dispatch")
	if err := workers.EndInference(t.Context(), owner); err != nil {
		t.Fatal(err)
	}
	*events = nil
	if err := workers.AuthorizeObjectWrite(t.Context(), owner); err != nil {
		t.Fatal(err)
	}
	check("object write")
	if _, err := workers.Complete(t.Context(), owner, transcriptionLockDraft()); err != nil {
		t.Fatal(err)
	}
	check("complete")
}

func TestTranscriptionRevocationStillAllowsOwnedStopFacts(t *testing.T) {
	tx, actor, _, workers, owner, events := transcriptionLockFixture(t)
	if _, err := workers.Claim(t.Context(), owner); err != nil {
		t.Fatal(err)
	}
	if err := workers.StartInference(t.Context(), owner); err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := workers.AuthorizeObjectWrite(t.Context(), owner); err == nil {
		t.Fatal("revoked worker admitted an object write")
	}
	if err := workers.StartInference(t.Context(), owner); err == nil {
		t.Fatal("revoked worker admitted native dispatch")
	}
	if err := workers.Progress(t.Context(), owner, 50, "transcribing"); err != nil {
		t.Fatal("revocation blocked observed local progress", err)
	}
	if err := tx.Exec(`UPDATE mediatool.transcription_job SET status='cancel_requested' WHERE id=?`, owner.JobID).Error; err != nil {
		t.Fatal(err)
	}
	*events = nil
	if err := workers.EndInference(t.Context(), owner); err != nil {
		t.Fatal("revocation blocked actual native cessation", err)
	}
	job, err := workers.Complete(t.Context(), owner, transcriptionLockDraft())
	if err != nil || job.Status != tooldomain.TranscriptionCancelled || job.ResultSHA256 != nil {
		t.Fatal("revocation blocked cancellation or published discarded cues", job, err)
	}
	if err := workers.Finish(t.Context(), owner, true, ""); err != nil {
		t.Fatal("revocation blocked owned finalization", err)
	}
	if err := workers.Release(t.Context(), owner); err != nil {
		t.Fatal("revocation blocked actual worker release", err)
	}
	for _, event := range *events {
		if event == "permission" {
			t.Fatal("stop facts acquired a new permission gate", *events)
		}
	}
	if err := workers.EndInference(t.Context(), owner); !errors.Is(err, toolapp.ErrConflict) {
		t.Fatal("released worker changed native cessation", err)
	}
}

func TestTranscriptionWorkerRejectsChangedAuthorityBinding(t *testing.T) {
	for _, gate := range []string{"claim", "dispatch", "object write", "complete"} {
		t.Run(gate, func(t *testing.T) {
			tx, _, _, workers, owner, events := transcriptionLockFixture(t)
			if gate != "claim" {
				if _, err := workers.Claim(t.Context(), owner); err != nil {
					t.Fatal(err)
				}
			}
			if gate == "object write" || gate == "complete" {
				if err := workers.StartInference(t.Context(), owner); err != nil {
					t.Fatal(err)
				}
				if err := workers.EndInference(t.Context(), owner); err != nil {
					t.Fatal(err)
				}
			}
			changed := toolpg.NewTranscriptionStore(tx, nil, func(db *gorm.DB) toolapp.DerivedMedia {
				return transcriptionPermissionTrace{
					DerivedMedia: mediaapp.NewDerivedService(pgmedia.NewScopedStore(db, app.NewMediaWorkerScopeAccess)),
					events:       events,
					afterAuthorize: func() error {
						return db.Exec(`UPDATE mediatool.transcription_job SET actor_id=? WHERE id=?`, uuid.New(), owner.JobID).Error
					},
				}
			})
			var err error
			switch gate {
			case "claim":
				_, err = changed.Claim(t.Context(), owner)
			case "dispatch":
				err = changed.StartInference(t.Context(), owner)
			case "object write":
				err = changed.AuthorizeObjectWrite(t.Context(), owner)
			case "complete":
				_, err = changed.Complete(t.Context(), owner, transcriptionLockDraft())
			}
			if !errors.Is(err, toolapp.ErrConflict) {
				t.Fatal("worker used authority from a stale candidate binding", err)
			}
		})
	}
}
