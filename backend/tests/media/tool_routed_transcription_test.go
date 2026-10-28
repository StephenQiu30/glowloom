package media_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	flow "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/workflow"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	toolff "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/ffmpeg"
	pgtool "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/postgres"
	toolflow "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/workflow"
	tool "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
	domain "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/domain"
	platform "github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

// The saved-node boundary and speech recognizer are controlled fixtures; media
// authorization, original bytes, real FFmpeg, PostgreSQL and native TOS SDK are actual.
type routedSpeechSource struct{ asset uuid.UUID }

func (s routedSpeechSource) FreezeTranscriptionSource(context.Context, identity.Principal, uuid.UUID, uuid.UUID, uuid.UUID, int64) (uuid.UUID, error) {
	return s.asset, nil
}

type routedSpeechRecognizer struct{ calls int }

func (s *routedSpeechRecognizer) Transcribe(_ context.Context, file *media.Downloaded, language string, duration int64) (domain.Transcript, error) {
	s.calls++
	if file == nil || file.File == nil || language != "auto" || duration != 1000 {
		return domain.Transcript{}, tool.ErrInvalidTranscription
	}
	return domain.Transcript{Version: 1, Language: "chinese", DurationMS: 1000, Segments: []domain.SubtitleSegment{{StartMS: 0, EndMS: 1000, Text: "受控识别夹具"}}}, nil
}

func TestRoutedTranscriptionActualPostgresTOSAndFFmpegAcceptsOneFrozenArtifact(t *testing.T) {
	owner, runtime := routedGeneratedDatabase(t)
	actor, project := mediaStoreProject(t, owner)
	registry, writes, cfg := controlledGeneratedTOS(t)
	profiles := []media.StorageProfile{}
	for id, p := range cfg.Profiles {
		profiles = append(profiles, media.StorageProfile{ID: id, Provider: p.Provider, Endpoint: p.Endpoint, SigningEndpoint: p.SigningEndpoint, Bucket: p.Bucket, Region: p.Region})
	}
	routes := pgmedia.NewStorageRoutes(owner)
	if err := routes.RegisterProfiles(t.Context(), profiles); err != nil {
		t.Fatal(err)
	}
	if err := owner.Exec(`INSERT INTO media.storage_route(singleton,profile_id,revision) VALUES(1,'tos-frozen','controlled-tool-v1') ON CONFLICT(singleton) DO UPDATE SET profile_id=excluded.profile_id,revision=excluded.revision`).Error; err != nil {
		t.Fatal(err)
	}
	pcm := transcriptionPCM(t)
	defer func() { _ = pcm.Close() }()
	raw, err := io.ReadAll(pcm.File)
	if err != nil {
		t.Fatal(err)
	}
	asset := uuid.New()
	key := "projects/" + project.String() + "/audio/2026/10/" + asset.String() + ".wav"
	digest := sha256.Sum256(raw)
	sha := hex.EncodeToString(digest[:])
	if err = registry.PutIfAbsent(t.Context(), platform.Ref{ProfileID: "tos-frozen", Bucket: "controlled-fixture", Key: key}, bytes.NewReader(raw), int64(len(raw)), "audio/wav", sha); err != nil {
		t.Fatal(err)
	}
	location := uuid.New()
	if err = owner.Exec(`INSERT INTO media.object_location(id,profile_id,bucket,object_key,state,byte_size,sha256,mime_type) VALUES(?,'tos-frozen','controlled-fixture',?,'accepted',?,?,'audio/wav')`, location, key, len(raw), sha).Error; err != nil {
		t.Fatal(err)
	}
	if err = owner.Exec(`INSERT INTO media.media_asset(id,project_id,kind,origin,status,object_key,object_location_id,file_name,mime_type,byte_size,sha256,duration_ms,moderation_status) VALUES(?,?,'audio','upload','ready',?,?,'synthetic.wav','audio/wav',?,?,1000,'passed')`, asset, project, key, location, len(raw), sha).Error; err != nil {
		t.Fatal(err)
	}
	makeStore := func(worker bool) *pgtool.TranscriptionStore {
		scopes := app.NewMediaScopeAccess
		if worker {
			scopes = app.NewMediaWorkerScopeAccess
		}
		return pgtool.NewTranscriptionStore(runtime, func(*gorm.DB) tool.TranscriptionSourceReader { return routedSpeechSource{asset} }, func(tx *gorm.DB) tool.DerivedMedia {
			return media.NewDerivedService(pgmedia.NewScopedStore(tx, scopes))
		}).WithStorageAdmission(func(tx *gorm.DB) tool.ToolStorageLedger { return pgmedia.NewStorageRoutes(tx) })
	}
	store, workerStore := makeStore(false), makeStore(true)
	input := tool.TranscriptionCreateInput{CanvasID: uuid.New(), NodeID: uuid.New(), Revision: 1, Language: "auto"}
	command := uuid.New()
	job, err := store.Create(t.Context(), actor, project, command, input)
	if err != nil {
		t.Fatal("pin before outbox", err)
	}
	if err = owner.Exec(`UPDATE media.storage_route SET profile_id='tos-new',revision='switched-after-admission' WHERE singleton=1`).Error; err != nil {
		t.Fatal(err)
	}
	prep, err := toolff.NewAudioPreprocessor(flow.FFProber{})
	if err != nil {
		t.Fatal(err)
	}
	recognizer := new(routedSpeechRecognizer)
	worker := tool.NewRoutedTranscriptionWorker(workerStore, toolflow.NewRoutedObjects(registry, pgmedia.NewStorageRoutes(runtime), nil), prep, recognizer)
	work := tool.TranscriptionWorkID{JobID: job.ID, Attempt: 1, ExecutionID: uuid.New()}
	result, err := worker.Execute(t.Context(), work)
	if err != nil || result.Status != domain.TranscriptionSucceeded {
		t.Fatal("native artifact acceptance", result.Status, err)
	}
	if _, err = worker.Execute(t.Context(), work); err != nil || recognizer.calls != 1 || *writes != 2 {
		t.Fatal("permanent attempt repeated inference/write", err, recognizer.calls, *writes)
	}
	var fact struct {
		State, ProfileID string
		Total            int
	}
	if err = owner.Raw(`SELECT w.state,w.profile_id,(SELECT count(*) FROM media.object_location l WHERE l.owner_intent_id=w.id AND l.state='accepted') total FROM media.storage_work_intent w WHERE w.kind='transcription' AND w.owner_id=? AND w.work_id=?`, project, tool.StorageOwner("transcription", project, job.ID, 1).WorkID).Scan(&fact).Error; err != nil || fact.State != "accepted" || fact.ProfileID != "tos-frozen" || fact.Total != 1 {
		t.Fatal("exact receipt/frozen profile", fact, err)
	}
	if replay, e := store.Create(t.Context(), actor, project, command, input); e != nil || replay.ID != job.ID {
		t.Fatal("same command not recovered", e)
	}
	if err = owner.Exec(`UPDATE media.storage_route SET profile_id='tos-frozen' WHERE singleton=1`).Error; err != nil {
		t.Fatal(err)
	}
	next, e := store.Create(t.Context(), actor, project, uuid.New(), input)
	if e != nil {
		t.Fatal(e)
	}
	if err = owner.Exec(`UPDATE media.media_asset SET sha256=repeat('f',64) WHERE id=?`, asset).Error; err != nil {
		t.Fatal(err)
	}
	if _, e = worker.Execute(t.Context(), tool.TranscriptionWorkID{JobID: next.ID, Attempt: 1, ExecutionID: uuid.New()}); e == nil {
		t.Fatal("changed source was consumed", e)
	}
	if recognizer.calls != 1 || *writes != 2 {
		t.Fatal("invalid source reached inference/SDK write")
	}
}
