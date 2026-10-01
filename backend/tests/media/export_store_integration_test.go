package media_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	canvasdomain "github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/lanverse/backend/internal/media/domain"
	toolpg "github.com/StephenQiu30/lanverse/backend/internal/mediatool/adapter/postgres"
	toolapp "github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
	tooldomain "github.com/StephenQiu30/lanverse/backend/internal/mediatool/domain"
)

func TestExportDurableCancelGateRejectsChangedReplayAndForeignRead(t *testing.T) {
	db := mediaStoreDB(t)
	actor, project := mediaStoreProject(t, db)
	_, foreign := mediaStoreProject(t, db)
	timeline := exportTimeline(t)
	source := timeline.Clips[0].AssetID
	width, height := int32(64), int32(64)
	asset := imageAsset(project, *source)
	asset.Width, asset.Height = &width, &height
	sha := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	asset.SHA256 = &sha
	asset.Status, asset.ModerationStatus = mediadomain.StatusProcessing, mediadomain.ModerationPending
	if err := pgmedia.NewStore(db).CreateAsset(t.Context(), actor, asset); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`UPDATE media.media_asset SET status='ready',moderation_status='passed' WHERE id=?`, asset.ID).Error; err != nil {
		t.Fatal(err)
	}
	freeze := &fixedTimeline{timeline: timeline}
	store := toolpg.NewStore(db, func(*gorm.DB) toolapp.TimelineReader { return freeze }, func(tx *gorm.DB) toolapp.DerivedMedia { return mediaapp.NewDerivedService(pgmedia.NewStore(tx)) })
	input := toolapp.CreateInput{CanvasID: uuid.New(), NodeID: uuid.New(), Revision: 1}
	key := uuid.New()
	job, err := store.Create(t.Context(), actor, project, key, input)
	if err != nil || job.Status != tooldomain.Queued {
		t.Fatalf("create: %+v %v", job, err)
	}
	repeated, err := store.Create(t.Context(), actor, project, key, input)
	if err != nil || repeated.ID != job.ID {
		t.Fatalf("replay: %v", err)
	}
	changed := input
	changed.Revision++
	if _, err = store.Create(t.Context(), actor, project, key, changed); !errors.Is(err, toolapp.ErrConflict) {
		t.Fatalf("changed body: %v", err)
	}
	if _, err = store.Get(t.Context(), actor, foreign, job.ID); !errors.Is(err, toolapp.ErrNotFound) {
		t.Fatalf("foreign read: %v", err)
	}
	cancelled, err := store.Control(t.Context(), actor, project, job.ID, uuid.New(), job.Revision, "cancel")
	if err != nil || cancelled.Status != tooldomain.CancelRequested {
		t.Fatalf("cancel accepted: %+v %v", cancelled, err)
	}
	work, err := store.Claim(t.Context(), toolapp.WorkID{JobID: job.ID, Attempt: 1})
	if !errors.Is(err, toolapp.ErrCancelled) || work.Job.ID != uuid.Nil {
		t.Fatalf("dispatch gate: %+v %v", work, err)
	}
	final, err := store.Get(t.Context(), actor, project, job.ID)
	if err != nil || final.Status != tooldomain.Cancelled {
		t.Fatalf("cancel evidence: %+v %v", final, err)
	}
}

// The HTTP/private-render test below uses the real canvas reader; this fixture
// isolates the job cancellation gate from canvas editing behavior.
func exportTimeline(t *testing.T) canvasdomain.TimelineConfig {
	t.Helper()
	lane := uuid.New()
	asset := uuid.New()
	return canvasdomain.TimelineConfig{Version: 1, AspectRatio: "1:1", FPS: 25, Tracks: []canvasdomain.TimelineTrack{{ID: lane, Kind: "image", Label: "Image", Visible: true}}, Clips: []canvasdomain.TimelineClip{{ID: uuid.New(), TrackID: lane, Kind: "image", AssetID: &asset, Title: "Image", DurationMS: 200, Volume: 1}}, SubtitleStyle: canvasdomain.SubtitleStyle{FontSize: 24, Color: "#ffffff", Position: "bottom"}}
}

type fixedTimeline struct{ timeline canvasdomain.TimelineConfig }

func (f *fixedTimeline) FreezeTimeline(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID, uuid.UUID, int64) (canvasdomain.TimelineConfig, error) {
	return f.timeline, nil
}
func imageAsset(project, id uuid.UUID) mediadomain.MediaAsset {
	now := time.Now().UTC()
	return mediadomain.MediaAsset{ID: id, ProjectID: project, Kind: mediadomain.KindImage, Origin: mediadomain.OriginUpload, Status: mediadomain.StatusReady, ObjectKey: "projects/" + project.String() + "/image/2026/10/" + id.String() + ".png", MimeType: "image/png", FileName: "image.png", ByteSize: 64, ModerationStatus: mediadomain.ModerationPassed, Revision: 1, CreateTime: now, UpdateTime: now}
}

func TestExportCancellationCannotBorrowAnotherWorkersCessation(t *testing.T) {
	db := mediaStoreDB(t)
	actor, project := mediaStoreProject(t, db)
	// Caption-only export avoids irrelevant source probing in the worker lock test.
	timeline := exportTimeline(t)
	timeline.Tracks[0].Kind = "subtitle"
	timeline.Clips[0].Kind = "subtitle"
	timeline.Clips[0].AssetID = nil
	timeline.Clips[0].Text = "caption"
	store := toolpg.NewStore(db, func(*gorm.DB) toolapp.TimelineReader { return &fixedTimeline{timeline: timeline} }, func(tx *gorm.DB) toolapp.DerivedMedia { return mediaapp.NewDerivedService(pgmedia.NewStore(tx)) })
	job, err := store.Create(t.Context(), actor, project, uuid.New(), toolapp.CreateInput{CanvasID: uuid.New(), NodeID: uuid.New(), Revision: 1})
	if err != nil {
		t.Fatal(err)
	}
	owner := toolapp.WorkID{JobID: job.ID, Attempt: 1, ExecutionID: uuid.New()}
	running, err := store.Claim(t.Context(), owner)
	if err != nil {
		t.Fatal(err)
	}
	stranger := owner
	stranger.ExecutionID = uuid.New()
	if _, err := store.Claim(t.Context(), stranger); !errors.Is(err, toolapp.ErrWorkerBusy) {
		t.Fatalf("overlapping active execution admitted: %v", err)
	}
	cancel, err := store.Control(t.Context(), actor, project, job.ID, uuid.New(), running.Job.Revision, "cancel")
	if err != nil || cancel.Status != tooldomain.CancelRequested {
		t.Fatal(err)
	}
	if _, err := store.Claim(t.Context(), stranger); !errors.Is(err, toolapp.ErrCancelled) {
		t.Fatal(err)
	}
	if err := store.Finish(t.Context(), stranger, true, ""); !errors.Is(err, toolapp.ErrConflict) {
		t.Fatalf("another worker claimed cessation: %v", err)
	}
	if err := store.FailWorkflow(t.Context(), owner); err != nil {
		t.Fatal(err)
	}
	pending, err := store.Get(t.Context(), actor, project, job.ID)
	if err != nil || pending.Status != tooldomain.CancelRequested {
		t.Fatal("orchestration timeout fabricated cancelled")
	}
	if err := store.Progress(t.Context(), owner, 20, "rendering"); !errors.Is(err, toolapp.ErrCancelled) {
		t.Fatal(err)
	}
	// Only the owner returning from its real subprocess has this right.
	if err := store.Finish(t.Context(), owner, true, ""); err != nil {
		t.Fatal(err)
	}
	stopped, err := store.Get(t.Context(), actor, project, job.ID)
	if err != nil || stopped.Status != tooldomain.Cancelled {
		t.Fatal(err)
	}
	retry, err := store.Control(t.Context(), actor, project, job.ID, uuid.New(), stopped.Revision, "retry")
	if err != nil || retry.Attempt != 2 || retry.Status != tooldomain.Queued {
		t.Fatal(err)
	}
	if err := store.Progress(t.Context(), owner, 90, "previews"); !errors.Is(err, toolapp.ErrConflict) {
		t.Fatal("old attempt mutated retry")
	}
}

func TestExportRuntimeRoleOwnsOnlyMutableJobFacts(t *testing.T) {
	if os.Getenv("LV_TEST_MEDIA_EXPORT_REQUIRE_ROLE") != "1" {
		t.Skip("set LV_TEST_MEDIA_EXPORT_REQUIRE_ROLE=1 with isolated runtime-role DSN")
	}
	db := mediaStoreDB(t)
	var role struct {
		Role                                                                                            string
		Superuser, OwnsJobs, CanInsert, CanUpdate, CanFreeze, CanCommand, CanOutbox, CanInbox, CanAudit bool
	}
	if err := db.Raw(`SELECT current_role AS role,(SELECT rolsuper FROM pg_roles WHERE rolname=current_role) superuser,(SELECT pg_get_userbyid(relowner)=current_role FROM pg_class WHERE oid='mediatool.export_job'::regclass) owns_jobs,has_table_privilege(current_role,'mediatool.export_job','SELECT,INSERT') can_insert,has_column_privilege(current_role,'mediatool.export_job','active_worker','UPDATE') can_update,has_column_privilege(current_role,'mediatool.export_job','frozen','UPDATE') can_freeze,has_table_privilege(current_role,'mediatool.export_command','SELECT,INSERT') can_command,has_table_privilege(current_role,'infra.outbox','SELECT,INSERT') can_outbox,has_table_privilege(current_role,'infra.processed_event','SELECT,INSERT') can_inbox,has_table_privilege(current_role,'audit.audit_log','SELECT,INSERT') can_audit`).Scan(&role).Error; err != nil {
		t.Fatal(err)
	}
	if role.Role != "lanverse_app" || role.Superuser || role.OwnsJobs || role.CanFreeze || !role.CanInsert || !role.CanUpdate || !role.CanCommand || !role.CanOutbox || !role.CanInbox || !role.CanAudit {
		t.Fatalf("incorrect non-owning runtime ACL: %+v", role)
	}
}

func TestExportRejectsUnverifiableRealPersonConsentBeforeQueueing(t *testing.T) {
	db := mediaStoreDB(t)
	actor, project := mediaStoreProject(t, db)
	timeline := exportTimeline(t)
	asset := imageAsset(project, *timeline.Clips[0].AssetID)
	sha := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	consent := uuid.New()
	asset.SHA256 = &sha
	asset.ContainsRealPerson = true
	asset.ConsentRecordID = &consent
	asset.Status = mediadomain.StatusProcessing
	asset.ModerationStatus = mediadomain.ModerationPending
	if err := pgmedia.NewStore(db).CreateAsset(t.Context(), actor, asset); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`UPDATE media.media_asset SET status='ready',moderation_status='passed' WHERE id=?`, asset.ID).Error; err != nil {
		t.Fatal(err)
	}
	store := toolpg.NewStore(db, func(*gorm.DB) toolapp.TimelineReader { return &fixedTimeline{timeline: timeline} }, func(tx *gorm.DB) toolapp.DerivedMedia { return mediaapp.NewDerivedService(pgmedia.NewStore(tx)) })
	if _, err := store.Create(t.Context(), actor, project, uuid.New(), toolapp.CreateInput{CanvasID: uuid.New(), NodeID: uuid.New(), Revision: 1}); !errors.Is(err, toolapp.ErrNotFound) {
		t.Fatalf("unknown consent registry bypassed: %v", err)
	}
	var count int
	if err := db.Raw(`SELECT count(*) FROM mediatool.export_job WHERE project_id=?`, project).Scan(&count).Error; err != nil || count != 0 {
		t.Fatal("rejected source partially queued export")
	}
}
