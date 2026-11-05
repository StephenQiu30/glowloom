package media_test

import (
	"context"
	"errors"
	"io"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	mediaflow "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	toolff "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/ffmpeg"
	toolpg "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/postgres"
	tooltask "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/task"
	toolapp "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
	tooldomain "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	"github.com/StephenQiu30/glowloom-server/backend/tests/taskfixture"
)

var errTakeoverStorage = errors.New("isolated storage outage after recovered claim")

type unavailableTaskObjects struct{}

func (unavailableTaskObjects) Get(context.Context, string) (io.ReadCloser, error) {
	return nil, errTakeoverStorage
}
func (unavailableTaskObjects) Stat(context.Context, string) (mediaapp.ObjectInfo, error) {
	return mediaapp.ObjectInfo{}, errTakeoverStorage
}
func (unavailableTaskObjects) PutIfAbsent(context.Context, string, io.Reader, int64, string, string) error {
	return errTakeoverStorage
}

// A process may disappear after the domain claim commits. The next SQL epoch
// must recover that same attempt; a dead active_worker cannot block it forever.
func TestExportTaskRecoversAbandonedDomainOwner(t *testing.T) {
	db := mediaStoreDB(t)
	actor, project := mediaStoreProject(t, db)
	timeline := exportTimeline(t)
	timeline.Tracks[0].Kind = "subtitle"
	timeline.Clips[0].Kind = "subtitle"
	timeline.Clips[0].AssetID = nil
	timeline.Clips[0].Text = "recovery"
	create := toolpg.NewStore(db, func(*gorm.DB) toolapp.TimelineReader { return &fixedTimeline{timeline: timeline} }, func(tx *gorm.DB) toolapp.DerivedMedia { return mediaapp.NewDerivedService(mediaTestStore(tx)) })
	store := toolpg.NewStore(db, func(*gorm.DB) toolapp.TimelineReader { return &fixedTimeline{timeline: timeline} }, func(tx *gorm.DB) toolapp.DerivedMedia { return mediaapp.NewDerivedService(mediaWorkerStore(tx)) })
	job, err := create.Create(t.Context(), actor, project, uuid.New(), toolapp.CreateInput{CanvasID: uuid.New(), NodeID: uuid.New(), Revision: 1})
	if err != nil {
		t.Fatal(err)
	}
	var businessID string
	if err := db.Raw("SELECT business_id FROM infra.task_work WHERE kind=? AND reference @> ?::jsonb", tooltask.Export, `{"job_id":"`+job.ID.String()+`"}`).Scan(&businessID).Error; err != nil || businessID == "" {
		t.Fatal(businessID, err)
	}
	lease, err := taskruntime.NewStore(db).ForBusiness(businessID).Claim(t.Context(), []string{tooltask.Export}, "lost-process", time.Minute)
	if err != nil || lease == nil {
		t.Fatal(lease, err)
	}
	oldID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(lease.Kind+"/"+lease.BusinessID+"/"+strconv.FormatInt(lease.Epoch, 10)))
	if _, err := store.Claim(t.Context(), toolapp.WorkID{JobID: job.ID, Attempt: 1, ExecutionID: oldID}); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE infra.task_work SET lease_until=clock_timestamp()-interval '1 second' WHERE kind=? AND business_id=?", tooltask.Export, businessID).Error; err != nil {
		t.Fatal(err)
	}
	renderer, err := toolff.NewRenderer(mediaflow.FFProber{})
	if err != nil {
		t.Fatal(err)
	}
	worker := toolapp.NewWorker(store, unavailableTaskObjects{}, renderer, mediaflow.FFProber{}, mediaflow.FFUploadRenderer{})
	handle := tooltask.ExportHandler(worker, store)
	observed := make(chan error, 1)
	taskfixture.StartWork(t, db, tooltask.Export, map[string]any{"job_id": job.ID}, func(ctx context.Context, j taskruntime.Job) (time.Time, error) {
		next, err := handle(ctx, j)
		select {
		case observed <- err:
		default:
		}
		return next, err
	})
	select {
	case err := <-observed:
		if !errors.Is(err, errTakeoverStorage) {
			t.Fatalf("recovered attempt did not reach its original storage phase: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("takeover failed to execute")
	}
}

type noTaskTranscription struct{ calls int }

func (n *noTaskTranscription) Transcribe(context.Context, *mediaapp.Downloaded, string, int64) (tooldomain.Transcript, error) {
	n.calls++
	return tooldomain.Transcript{}, errors.New("unexpected inference")
}

func TestTranscriptionTaskRecoversPreparationButNeverRepeatsSubmittedInference(t *testing.T) {
	for _, submitted := range []bool{false, true} {
		t.Run(strconv.FormatBool(submitted), func(t *testing.T) {
			db, create, actor, project, input := transcriptionPGFixture(t)
			store := transcriptionWorkerStore(db)
			job, err := create.Create(t.Context(), actor, project, uuid.New(), input)
			if err != nil {
				t.Fatal(err)
			}
			var businessID string
			if err := db.Raw("SELECT business_id FROM infra.task_work WHERE kind=? AND reference @> ?::jsonb", tooltask.Transcription, `{"job_id":"`+job.ID.String()+`"}`).Scan(&businessID).Error; err != nil || businessID == "" {
				t.Fatal(businessID, err)
			}
			lease, err := taskruntime.NewStore(db).ForBusiness(businessID).Claim(t.Context(), []string{tooltask.Transcription}, "lost-process", time.Minute)
			if err != nil || lease == nil {
				t.Fatal(lease, err)
			}
			oldID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(lease.Kind+"/"+lease.BusinessID+"/"+strconv.FormatInt(lease.Epoch, 10)))
			work := toolapp.TranscriptionWorkID{JobID: job.ID, Attempt: 1, ExecutionID: oldID}
			if _, err := store.Claim(t.Context(), work); err != nil {
				t.Fatal(err)
			}
			if submitted {
				if err := store.StartInference(t.Context(), work); err != nil {
					t.Fatal(err)
				}
			}
			if err := db.Exec("UPDATE infra.task_work SET lease_until=clock_timestamp()-interval '1 second' WHERE kind=? AND business_id=?", tooltask.Transcription, businessID).Error; err != nil {
				t.Fatal(err)
			}
			preprocessor, err := toolff.NewAudioPreprocessor(mediaflow.FFProber{})
			if err != nil {
				t.Fatal(err)
			}
			transcriber := &noTaskTranscription{}
			worker := toolapp.NewTranscriptionWorker(store, unavailableTaskObjects{}, preprocessor, transcriber)
			handle := tooltask.TranscriptionHandler(worker, store)
			observed := make(chan error, 1)
			taskfixture.StartWork(t, db, tooltask.Transcription, map[string]any{"job_id": job.ID}, func(ctx context.Context, j taskruntime.Job) (time.Time, error) {
				next, err := handle(ctx, j)
				select {
				case observed <- err:
				default:
				}
				return next, err
			})
			select {
			case err := <-observed:
				if submitted && err != nil {
					t.Fatal("unknown inference failed to park safely", err)
				}
				if !submitted && !errors.Is(err, errTakeoverStorage) {
					t.Fatalf("preparation did not resume: %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("takeover failed to execute")
			}
			if transcriber.calls != 0 {
				t.Fatal("abandoned attempt dispatched inference", transcriber.calls)
			}
			if submitted {
				current, err := create.Get(t.Context(), actor, project, job.ID)
				if err != nil || current.Status != tooldomain.TranscriptionRunning || current.Stage != "awaiting_reconciliation" {
					t.Fatalf("original submitted identity lost: %+v %v", current, err)
				}
			}
		})
	}
}

// The renderer, decoder, SQL leases, routed provider bytes and publication are real.
func TestExportTaskRendersDecodableVideoThroughPostgresWorker(t *testing.T) {
	db := mediatoolFixtureDB(t)
	actor, project := mediaStoreProject(t, db)
	timeline := exportTimeline(t)
	timeline.Tracks[0].Kind = "subtitle"
	timeline.Clips[0].Kind = "subtitle"
	timeline.Clips[0].AssetID = nil
	timeline.Clips[0].Text = "Actual worker export"
	timeline.Clips[0].DurationMS = 800
	create := mediatoolExportStore(db, func(*gorm.DB) toolapp.TimelineReader { return &fixedTimeline{timeline: timeline} }, func(tx *gorm.DB) toolapp.DerivedMedia { return mediaapp.NewDerivedService(mediaTestStore(tx)) })
	store := mediatoolExportStore(db, func(*gorm.DB) toolapp.TimelineReader { return &fixedTimeline{timeline: timeline} }, func(tx *gorm.DB) toolapp.DerivedMedia { return mediaapp.NewDerivedService(mediaWorkerStore(tx)) })
	job, err := create.Create(t.Context(), actor, project, uuid.New(), toolapp.CreateInput{CanvasID: uuid.New(), NodeID: uuid.New(), Revision: 1})
	if err != nil {
		t.Fatal(err)
	}
	objects := mediatoolRoutedObjects(t, db)
	renderer, err := toolff.NewRenderer(mediaflow.FFProber{})
	if err != nil {
		t.Fatal(err)
	}
	worker := toolapp.NewRoutedWorker(store, objects, renderer, mediaflow.FFProber{}, mediaflow.FFUploadRenderer{})
	taskfixture.RunWork(t, db, tooltask.Export, map[string]any{"job_id": job.ID}, tooltask.ExportHandler(worker, store))
	final, err := create.Get(t.Context(), actor, project, job.ID)
	if err != nil || final.Status != tooldomain.ReviewRequired || final.AssetID == nil {
		t.Fatalf("actual render publication: %+v %v", final, err)
	}
	var key string
	if err := db.Raw("SELECT object_key FROM media.media_asset WHERE id=?", *final.AssetID).Scan(&key).Error; err != nil || key == "" {
		t.Fatal(key, err)
	}
	var saved struct{ ProfileID, Bucket string }
	if err := db.Raw(`SELECT l.profile_id,l.bucket FROM media.media_asset a JOIN media.object_location l ON l.id=a.object_location_id AND l.object_key=a.object_key WHERE a.id=?`, *final.AssetID).Scan(&saved).Error; err != nil {
		t.Fatal(err)
	}
	body, err := mediaFixtureReadObjects(t).Get(t.Context(), mediadomain.ObjectRef{ProfileID: saved.ProfileID, Bucket: saved.Bucket, Key: key})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := body.Close(); err != nil {
			t.Error(err)
		}
	}()
	file, err := mediaapp.ReadUpload(t.Context(), body, "worker.mp4")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			t.Error(err)
		}
	}()
	probe, err := (mediaflow.FFProber{}).Probe(t.Context(), file)
	if err != nil || probe.Kind != mediadomain.KindVideo || probe.DurationMS == nil || *probe.DurationMS < 700 {
		t.Fatalf("real ffprobe output: %+v %v", probe, err)
	}
}
