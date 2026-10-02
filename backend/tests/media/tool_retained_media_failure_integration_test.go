package media_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/lanverse/backend/internal/media/domain"
	toolpg "github.com/StephenQiu30/lanverse/backend/internal/mediatool/adapter/postgres"
	toolapp "github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
	tooldomain "github.com/StephenQiu30/lanverse/backend/internal/mediatool/domain"
)

type retainedExportFixture struct {
	database *gorm.DB
	actor    identityapp.Principal
	project  uuid.UUID
	source   mediadomain.MediaAsset
	jobs     []tooldomain.ExportJob
}

func newRetainedExportFixture(t *testing.T, count int) retainedExportFixture {
	t.Helper()
	database, actor, project := retainedToolDatabase(t)
	timeline := exportTimeline(t)
	source := imageAsset(project, *timeline.Clips[0].AssetID)
	digest := strings.Repeat("a", 64)
	source.SHA256 = &digest
	frozen, err := toolapp.FreezeInputs(timeline, []mediadomain.MediaAsset{source})
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(frozen)
	if err != nil {
		t.Fatal(err)
	}
	// Only owning retained metadata is needed here. Its source may no longer
	// exist in the mutable catalog; this fixture never claims private byte IO.
	fixture := retainedExportFixture{database: database, actor: actor, project: project, source: source}
	for range count {
		job := tooldomain.ExportJob{ID: uuid.New(), ProjectID: project, Attempt: 1, CreatedAt: time.Now().UTC()}
		if err := database.Exec(`INSERT INTO mediatool.export_job(id,project_id,org_id,actor_id,actor_role,canvas_id,node_id,source_revision,frozen,status,stage,progress,attempt,revision,created_at,updated_at)VALUES(?,?,?,?,'producer',?,?,1,?::jsonb,'cancelled','cancelled',0,1,1,?,?)`, job.ID, project, actor.OrgID, actor.ID, uuid.New(), uuid.New(), string(body), job.CreatedAt, job.CreatedAt).Error; err != nil {
			t.Fatal(err)
		}
		fixture.jobs = append(fixture.jobs, job)
	}
	// The reader orders jobs by UUID. The cross-job regression must encounter
	// the legitimate output before the corrupt job tries to claim that identity.
	slices.SortFunc(fixture.jobs, func(a, b tooldomain.ExportJob) int { return strings.Compare(a.ID.String(), b.ID.String()) })
	return fixture
}

func retainedRuntimeRead(t *testing.T, database *gorm.DB, use func(*toolpg.RetainedMediaReader) error) {
	t.Helper()
	if err := database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE lanverse_app`).Error; err != nil {
			return err
		}
		var role string
		if err := tx.Raw(`SELECT current_user`).Scan(&role).Error; err != nil {
			return err
		}
		if role != "lanverse_app" {
			t.Fatal("retained reader requires the actual non-owner role")
		}
		return use(toolpg.NewRetainedMediaReader(tx, mediaapp.NewDerivedService(pgmedia.NewStore(tx))))
	}); err != nil {
		t.Fatal(err)
	}
}

func requireRetainedExportRejection(t *testing.T, fixture retainedExportFixture, id uuid.UUID) {
	t.Helper()
	retainedRuntimeRead(t, fixture.database, func(reader *toolpg.RetainedMediaReader) error {
		found, err := reader.HasMediaReferences(t.Context(), fixture.actor, fixture.project, id)
		if err == nil || found {
			t.Errorf("foreign export result binding became a valid reference: found=%t error=%v", found, err)
		}
		keys, err := reader.StorageUsageKeys(t.Context(), fixture.actor, fixture.project)
		if err == nil || keys != nil {
			t.Errorf("foreign export result binding returned a usage subtotal: keys=%d error=%v", len(keys), err)
		}
		return nil
	})
}

func TestMediaToolRetainedFailureRejectsExportInputAsItsOutput(t *testing.T) {
	fixture := newRetainedExportFixture(t, 1)
	if err := fixture.database.Exec(`UPDATE mediatool.export_job SET asset_id=?,sha256=? WHERE id=?`, fixture.source.ID, *fixture.source.SHA256, fixture.jobs[0].ID).Error; err != nil {
		t.Fatal(err)
	}
	requireRetainedExportRejection(t, fixture, fixture.source.ID)
}

func TestMediaToolRetainedFailureRejectsAnotherExportsOutput(t *testing.T) {
	fixture := newRetainedExportFixture(t, 2)
	foreign := uuid.NewSHA1(fixture.jobs[0].ID, []byte("export-output/1"))
	if err := fixture.database.Exec(`UPDATE mediatool.export_job SET asset_id=?,sha256=? WHERE id=?`, foreign, *fixture.source.SHA256, fixture.jobs[1].ID).Error; err != nil {
		t.Fatal(err)
	}
	requireRetainedExportRejection(t, fixture, foreign)
}

func TestMediaToolRetainedFailurePreservesOwnOldAttemptOutput(t *testing.T) {
	fixture := newRetainedExportFixture(t, 1)
	job := fixture.jobs[0]
	old := uuid.NewSHA1(job.ID, []byte("export-output/1"))
	if err := fixture.database.Exec(`UPDATE mediatool.export_job SET attempt=2,asset_id=?,sha256=? WHERE id=?`, old, *fixture.source.SHA256, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	retainedRuntimeRead(t, fixture.database, func(reader *toolpg.RetainedMediaReader) error {
		for _, id := range []uuid.UUID{fixture.source.ID, old, uuid.NewSHA1(job.ID, []byte("export-output/2"))} {
			found, err := reader.HasMediaReferences(t.Context(), fixture.actor, fixture.project, id)
			if err != nil || !found {
				t.Fatal("own retained source or historical output lost", found, err)
			}
		}
		keys, err := reader.StorageUsageKeys(t.Context(), fixture.actor, fixture.project)
		if err != nil || len(keys) != 7 || !slices.Contains(keys, fixture.source.ObjectKey) {
			t.Fatal("own retained history did not remain complete", len(keys), err)
		}
		for attempt := 1; attempt <= 2; attempt++ {
			id := uuid.NewSHA1(job.ID, []byte("export-output/"+strconv.Itoa(attempt)))
			key := path.Join("projects", fixture.project.String(), "video", job.CreatedAt.UTC().Format("2006/01"), id.String()+".mp4")
			for _, expected := range []string{key, strings.TrimSuffix(key, ".mp4") + "/poster.png", strings.TrimSuffix(key, ".mp4") + "/proxy_720p.mp4"} {
				if !slices.Contains(keys, expected) {
					t.Fatal("own historical attempt lost an exact private object key")
				}
			}
		}
		return nil
	})
}

func TestMediaToolRetainedFailureDoesNotAllocateUnrelatedTranscriptResult(t *testing.T) {
	// Deliberately serial: TotalAlloc belongs to the process, so run this test
	// alone when using its allocation evidence. No private object IO is claimed.
	database, actor, project := retainedToolDatabase(t)
	duration := int32(1000)
	input := tooldomain.FrozenSource{AssetID: uuid.New(), Revision: 1, Kind: "audio", ObjectKey: "projects/" + project.String() + "/audio/fixture/source.m4a", MIMEType: "audio/mp4", ByteSize: 100, SHA256: strings.Repeat("a", 64), DurationMS: &duration}
	frozen, err := json.Marshal(tooldomain.FrozenTranscription{Language: "zh", Input: input})
	if err != nil {
		t.Fatal(err)
	}
	job := uuid.New()
	now := time.Now().UTC()
	if err := database.Exec(`INSERT INTO mediatool.transcription_job(id,project_id,org_id,actor_id,actor_role,canvas_id,node_id,source_revision,language,frozen,status,stage,progress,attempt,revision,inference_state,created_at,updated_at)VALUES(?,?,?,?,'producer',?,?,1,'zh',?::jsonb,'failed','completed',0,1,1,'terminal',?,?)`, job, project, actor.OrgID, actor.ID, uuid.New(), uuid.New(), string(frozen), now, now).Error; err != nil {
		t.Fatal(err)
	}
	read := func() {
		retainedRuntimeRead(t, database, func(reader *toolpg.RetainedMediaReader) error {
			keys, err := reader.StorageUsageKeys(t.Context(), actor, project)
			if err != nil || len(keys) != 1 || keys[0] != input.ObjectKey {
				t.Fatal("complete transcript history source pin lost", len(keys), err)
			}
			return nil
		})
	}
	read() // Warm the connection, SQL metadata and owning JSON paths.
	allocation := func() uint64 {
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		read()
		runtime.ReadMemStats(&after)
		return after.TotalAlloc - before.TotalAlloc
	}
	baseline := allocation()
	transcript := tooldomain.Transcript{Version: 1, Language: "zh", DurationMS: 1000}
	for index := range 255 {
		transcript.Segments = append(transcript.Segments, tooldomain.SubtitleSegment{StartMS: int64(index), EndMS: int64(index + 1), Text: strings.Repeat("x", 32700)})
	}
	if transcript.Validate() != nil {
		t.Fatal("large result must remain a valid formal transcript")
	}
	result, err := json.Marshal(transcript)
	if err != nil || len(result) < 7<<20 || len(result) >= 8<<20 {
		t.Fatal("expected a valid result near the 8 MiB contract boundary", len(result), err)
	}
	digest := sha256.Sum256(result)
	if err := database.Exec(`UPDATE mediatool.transcription_job SET status='succeeded',progress=100,result=?::jsonb,result_sha256=? WHERE id=?`, string(result), hex.EncodeToString(digest[:]), job).Error; err != nil {
		t.Fatal(err)
	}
	withResult := allocation()
	t.Logf("retained source allocation: baseline=%d with_unrelated_result=%d result_bytes=%d", baseline, withResult, len(result))
	// Four MiB of headroom covers ordinary driver/runtime noise while remaining
	// below even one copy of the unrelated near-eight-MiB result.
	if withResult > baseline+4<<20 {
		t.Fatalf("retained reader allocated unrelated transcript result: baseline=%d with_result=%d", baseline, withResult)
	}
	retainedRuntimeRead(t, database, func(reader *toolpg.RetainedMediaReader) error {
		found, err := reader.HasMediaReferences(t.Context(), actor, project, input.AssetID)
		if err != nil || !found {
			t.Fatal("successful transcript lost its full historical source identity", found, err)
		}
		return nil
	})
}
