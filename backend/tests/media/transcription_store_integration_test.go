package media_test

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	canvaspg "github.com/StephenQiu30/lanverse/backend/internal/canvas/adapter/postgres"
	canvasapp "github.com/StephenQiu30/lanverse/backend/internal/canvas/application"
	canvasdomain "github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediapg "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/lanverse/backend/internal/media/domain"
	toolpg "github.com/StephenQiu30/lanverse/backend/internal/mediatool/adapter/postgres"
	toolapp "github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
	tooldomain "github.com/StephenQiu30/lanverse/backend/internal/mediatool/domain"
)

func transcriptionStore(t *testing.T, db *gorm.DB) *toolpg.TranscriptionStore {
	t.Helper()
	mediaFactory := func(tx *gorm.DB) canvasapp.MediaReader { return mediaapp.NewAssetQuery(mediapg.NewStore(tx), nil) }
	return toolpg.NewTranscriptionStore(db, func(tx *gorm.DB) toolapp.TranscriptionSourceReader {
		return canvasapp.NewTranscriptionSourceReader(canvaspg.NewStore(tx, mediaFactory))
	}, func(tx *gorm.DB) toolapp.DerivedMedia { return mediaapp.NewDerivedService(mediapg.NewStore(tx)) })
}

func transcriptionNode(t *testing.T, db *gorm.DB, actor identityapp.Principal, project, asset uuid.UUID, kind string) toolapp.TranscriptionCreateInput {
	t.Helper()
	mediaFactory := func(tx *gorm.DB) canvasapp.MediaReader { return mediaapp.NewAssetQuery(mediapg.NewStore(tx), nil) }
	service := canvasapp.NewService(canvaspg.NewStore(db, mediaFactory))
	doc, err := service.Create(t.Context(), actor, project, uuid.NewString(), canvasapp.CreateInput{Name: "Actual speech source"})
	if err != nil {
		t.Fatal(err)
	}
	node := canvasdomain.Node{ID: uuid.New(), NodeType: kind, NodeAction: "resource", Title: "Speech source", RefType: "media_asset", RefID: &asset}
	saved, err := service.Execute(t.Context(), actor, doc.ID, uuid.NewString(), canvasapp.CommandsInput{ExpectedRevision: doc.Revision, Commands: []canvasdomain.Command{{Type: "AddNodes", Nodes: []canvasdomain.Node{node}}}})
	if err != nil {
		t.Fatal(err)
	}
	return toolapp.TranscriptionCreateInput{CanvasID: doc.ID, NodeID: node.ID, Revision: saved.Revision, Language: "auto"}
}
func transcriptionPGFixture(t *testing.T) (*gorm.DB, *toolpg.TranscriptionStore, identityapp.Principal, uuid.UUID, toolapp.TranscriptionCreateInput) {
	t.Helper()
	db := mediaStoreDB(t)
	actor, project := mediaStoreProject(t, db)
	asset := imageAsset(project, uuid.New())
	asset.Kind = mediadomain.KindAudio
	asset.MimeType = "audio/mp4"
	asset.ObjectKey = "projects/" + project.String() + "/audio/2026/10/" + asset.ID.String() + ".m4a"
	asset.FileName = "speech.m4a"
	duration := int32(1000)
	asset.DurationMS = &duration
	sha := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	asset.SHA256 = &sha
	asset.Status = mediadomain.StatusProcessing
	asset.ModerationStatus = mediadomain.ModerationPending
	if err := mediapg.NewStore(db).CreateAsset(t.Context(), actor, asset); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`UPDATE media.media_asset SET status='ready',moderation_status='passed' WHERE id=?`, asset.ID).Error; err != nil {
		t.Fatal(err)
	}
	input := transcriptionNode(t, db, actor, project, asset.ID, "audio")
	return db, transcriptionStore(t, db), actor, project, input
}

func TestTranscriptionRuntimeRoleOwnsOnlyMutableFacts(t *testing.T) {
	if os.Getenv("LV_TEST_TRANSCRIPTION_REQUIRE_ROLE") != "1" {
		t.Skip("set isolated non-owner runtime-role DSN and LV_TEST_TRANSCRIPTION_REQUIRE_ROLE=1")
	}
	db := mediaStoreDB(t)
	var role struct {
		Role                                                                  string
		Superuser, Owner, Read, Write, Freeze, Commands, Outbox, Inbox, Audit bool
	}
	if err := db.Raw(`SELECT current_role AS role,(SELECT rolsuper FROM pg_roles WHERE rolname=current_role) superuser,(SELECT pg_get_userbyid(relowner)=current_role FROM pg_class WHERE oid='mediatool.transcription_job'::regclass) owner,has_table_privilege(current_role,'mediatool.transcription_job','SELECT,INSERT') read,has_column_privilege(current_role,'mediatool.transcription_job','inference_state','UPDATE') write,has_column_privilege(current_role,'mediatool.transcription_job','frozen','UPDATE') freeze,has_table_privilege(current_role,'mediatool.transcription_command','SELECT,INSERT') commands,has_table_privilege(current_role,'infra.outbox','SELECT,INSERT') outbox,has_table_privilege(current_role,'infra.processed_event','SELECT,INSERT') inbox,has_table_privilege(current_role,'audit.audit_log','SELECT,INSERT') audit`).Scan(&role).Error; err != nil {
		t.Fatal(err)
	}
	if role.Role != "lanverse_app" || role.Superuser || role.Owner || role.Freeze || !role.Read || !role.Write || !role.Commands || !role.Outbox || !role.Inbox || !role.Audit {
		t.Fatalf("invalid non-owner ACL: %+v", role)
	}
}

func TestTranscriptionDurableCancelBeforeDispatchAndIdempotentReplay(t *testing.T) {
	db, store, actor, project, input := transcriptionPGFixture(t)
	key := uuid.New()
	job, err := store.Create(t.Context(), actor, project, key, input)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := store.Create(t.Context(), actor, project, key, input)
	if err != nil || replay.ID != job.ID {
		t.Fatal("create replay", err)
	}
	changed := input
	changed.Language = "zh"
	if _, err := store.Create(t.Context(), actor, project, key, changed); !errors.Is(err, toolapp.ErrConflict) {
		t.Fatal("changed command replay", err)
	}
	_, foreign := mediaStoreProject(t, db)
	if _, err := store.Get(t.Context(), actor, foreign, job.ID); !errors.Is(err, toolapp.ErrNotFound) {
		t.Fatal("foreign job exposed", err)
	}
	cancel, err := store.Control(t.Context(), actor, project, job.ID, uuid.New(), job.Revision, "cancel")
	if err != nil || cancel.Status != tooldomain.TranscriptionCancelRequested {
		t.Fatal(err)
	}
	id := toolapp.TranscriptionWorkID{JobID: job.ID, Attempt: 1, ExecutionID: uuid.New()}
	if _, err := store.Claim(t.Context(), id); !errors.Is(err, toolapp.ErrCancelled) {
		t.Fatal("cancel gate", err)
	}
	actual, err := store.Get(t.Context(), actor, project, job.ID)
	if err != nil || actual.Status != tooldomain.TranscriptionCancelled {
		t.Fatal("not-dispatched cancellation evidence", err)
	}
}

func TestTranscriptionLostSubmittedOwnerCannotDispatchOrRetry(t *testing.T) {
	db, store, actor, project, input := transcriptionPGFixture(t)
	job, err := store.Create(t.Context(), actor, project, uuid.New(), input)
	if err != nil {
		t.Fatal(err)
	}
	owner := toolapp.TranscriptionWorkID{JobID: job.ID, Attempt: 1, ExecutionID: uuid.New()}
	if _, err := store.Claim(t.Context(), owner); err != nil {
		t.Fatal(err)
	}
	if err := store.StartInference(t.Context(), owner); err != nil {
		t.Fatal(err)
	}
	// A hard process loss leaves no Go defer or HTTP termination receipt. The
	// committed submitted state must block a later actual execution owner.
	replacement := owner
	replacement.ExecutionID = uuid.New()
	if _, err := store.Claim(t.Context(), replacement); !errors.Is(err, toolapp.ErrInferenceUncertain) {
		t.Fatal("lost owner admitted inference", err)
	}
	pending, err := store.Get(t.Context(), actor, project, job.ID)
	if err != nil || pending.Stage != "awaiting_reconciliation" || pending.Status != tooldomain.TranscriptionRunning {
		t.Fatalf("missing public uncertainty: %+v %v", pending, err)
	}
	if _, err := store.Control(t.Context(), actor, project, job.ID, uuid.New(), pending.Revision, "retry"); !errors.Is(err, toolapp.ErrConflict) {
		t.Fatal("unknown retry admitted", err)
	}
	if err := db.Exec(`UPDATE mediatool.transcription_job SET active_worker=NULL WHERE id=?`, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := store.Claim(t.Context(), replacement); !errors.Is(err, toolapp.ErrInferenceUncertain) {
		t.Fatal("unowned submitted dispatched", err)
	}
	if err := store.Finish(t.Context(), replacement, true, ""); !errors.Is(err, toolapp.ErrConflict) {
		t.Fatal("borrowed native cessation", err)
	}
}

func TestTranscriptionSoftCancelRequiresActualTerminalResponse(t *testing.T) {
	_, store, actor, project, input := transcriptionPGFixture(t)
	job, err := store.Create(t.Context(), actor, project, uuid.New(), input)
	if err != nil {
		t.Fatal(err)
	}
	owner := toolapp.TranscriptionWorkID{JobID: job.ID, Attempt: 1, ExecutionID: uuid.New()}
	if _, err := store.Claim(t.Context(), owner); err != nil {
		t.Fatal(err)
	}
	if err := store.StartInference(t.Context(), owner); err != nil {
		t.Fatal(err)
	}
	before, err := store.Get(t.Context(), actor, project, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := store.Control(t.Context(), actor, project, job.ID, uuid.New(), before.Revision, "cancel")
	if err != nil || pending.Status != tooldomain.TranscriptionCancelRequested {
		t.Fatal(err)
	}
	if err := store.FailWorkflow(t.Context(), owner); err != nil {
		t.Fatal(err)
	}
	still, err := store.Get(t.Context(), actor, project, job.ID)
	if err != nil || still.Status != tooldomain.TranscriptionCancelRequested {
		t.Fatal("workflow error forged cancellation", err)
	}
	if err := store.EndInference(t.Context(), owner); err != nil {
		t.Fatal(err)
	}
	draft := tooldomain.Transcript{Version: 1, Language: "english", DurationMS: 1000, Segments: []tooldomain.SubtitleSegment{{StartMS: 0, EndMS: 1000, Text: "Actual terminal response"}}}
	completed, err := store.Complete(t.Context(), owner, draft)
	if err != nil || completed.Status != tooldomain.TranscriptionCancelled || completed.ResultSHA256 != nil {
		t.Fatal("soft cancellation published result", err)
	}
	if err := store.Release(t.Context(), owner); err != nil {
		t.Fatal(err)
	}
	retried, err := store.Control(t.Context(), actor, project, job.ID, uuid.New(), completed.Revision, "retry")
	if err != nil || retried.Attempt != 2 {
		t.Fatal("known cessation retry", err)
	}
	if err := store.StartInference(t.Context(), owner); !errors.Is(err, toolapp.ErrConflict) {
		t.Fatal("old attempt changed retry", err)
	}
}

func TestTranscriptionNativeBudgetFitsActivity(t *testing.T) {
	// Include two final cancellation/release database calls and a full minute of
	// scheduling slack; detached inference cannot consume the activity deadline.
	total := toolapp.TranscriptionPreparationTimeout + toolapp.TranscriptionInferenceTimeout + toolapp.TranscriptionFinalizationTimeout + 20*time.Second + time.Minute
	if total >= toolapp.TranscriptionActivityTimeout {
		t.Fatal("native work exceeds enclosing Activity deadline")
	}
}

func TestTranscriptionMissingConfigRejectsNewJobsButKeepsReceiptsAndRead(t *testing.T) {
	db, store, actor, project, input := transcriptionPGFixture(t)
	key := uuid.New()
	job, err := store.Create(t.Context(), actor, project, key, input)
	if err != nil {
		t.Fatal(err)
	}
	disabled := toolpg.NewTranscriptionStore(db, nil, func(tx *gorm.DB) toolapp.DerivedMedia { return mediaapp.NewDerivedService(mediapg.NewStore(tx)) })
	if _, err := disabled.Create(t.Context(), actor, project, uuid.New(), input); !errors.Is(err, toolapp.ErrUnavailable) {
		t.Fatal("unconfigured new speech job admitted", err)
	}
	replay, err := disabled.Create(t.Context(), actor, project, key, input)
	if err != nil || replay.ID != job.ID {
		t.Fatal("accepted permanent receipt lost after config change", err)
	}
	read, err := disabled.Get(t.Context(), actor, project, job.ID)
	if err != nil || read.ID != job.ID {
		t.Fatal("missing optional recognizer blocked safe job read", err)
	}
	if _, err := disabled.Control(t.Context(), actor, project, job.ID, uuid.New(), job.Revision, "cancel"); err != nil {
		t.Fatal("missing optional recognizer blocked cancel", err)
	}
}
