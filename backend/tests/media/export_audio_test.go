package media_test

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	canvasdomain "github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
	mediapg "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/lanverse/backend/internal/media/domain"
	exportff "github.com/StephenQiu30/lanverse/backend/internal/mediatool/adapter/ffmpeg"
	exportpg "github.com/StephenQiu30/lanverse/backend/internal/mediatool/adapter/postgres"
	exportapp "github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
	exportdomain "github.com/StephenQiu30/lanverse/backend/internal/mediatool/domain"
)

func TestExportAudioExtractsTrimmedSoundWithGapsMutedTracksAndFades(t *testing.T) {
	renderer, err := exportff.NewRenderer(mediaflow.FFProber{})
	if err != nil {
		t.Fatal(err)
	}
	video, muted := uuid.New(), uuid.New()
	videoTrack, mutedTrack := uuid.New(), uuid.New()
	config := canvasdomain.TimelineConfig{Version: 1, AspectRatio: "16:9", FPS: 25, SubtitleStyle: canvasdomain.SubtitleStyle{FontSize: 24, Color: "#ffffff", Position: "bottom"}, Tracks: []canvasdomain.TimelineTrack{
		{ID: videoTrack, Kind: "video", Label: "Video sound", Visible: true}, {ID: mutedTrack, Kind: "audio", Label: "Muted", Visible: true, Muted: true},
	}, Clips: []canvasdomain.TimelineClip{
		{ID: uuid.New(), TrackID: videoTrack, Kind: "video", AssetID: &video, Title: "Extract", StartMS: 200, SourceStartMS: 500, DurationMS: 600, Volume: 0.5, FadeInMS: 120, FadeOutMS: 120},
		{ID: uuid.New(), TrackID: mutedTrack, Kind: "audio", AssetID: &muted, Title: "Muted", DurationMS: 1000, Volume: 1},
	}}
	root := t.TempDir()
	videoPath, mutedPath := filepath.Join(root, "source.mp4"), filepath.Join(root, "muted.wav")
	// The first half-second is silent. The trim must select the later real sound.
	if output, err := exec.CommandContext(t.Context(), "ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "color=c=red:s=64x64:r=25:d=1.5", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=1", "-filter_complex", "[1:a]adelay=500|500[a]", "-map", "0:v", "-map", "[a]", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-t", "1.5", videoPath).CombinedOutput(); err != nil {
		t.Fatalf("create actual video sound: %v %s", err, output)
	}
	if output, err := exec.CommandContext(t.Context(), "ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "sine=frequency=2000:sample_rate=48000:duration=1", mutedPath).CombinedOutput(); err != nil {
		t.Fatalf("create muted source: %v %s", err, output)
	}
	result, err := renderer.Render(t.Context(), frozenAudio(t, config), map[uuid.UUID]string{video: videoPath, muted: mutedPath}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = result.Close() }()
	probe, err := (mediaflow.FFProber{}).Probe(t.Context(), result)
	if err != nil || result.MIMEType != "audio/mp4" || probe.Kind != mediadomain.KindAudio || probe.Width != nil || probe.Height != nil || probe.DurationMS == nil || *probe.DurationMS < 975 || *probe.DurationMS > 1040 {
		t.Fatalf("actual M4A export facts: %+v mime=%s err=%v", probe, result.MIMEType, err)
	}
	pcm, err := exec.CommandContext(t.Context(), "ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-i", result.File.Name(), "-map", "0:a", "-ac", "1", "-ar", "48000", "-f", "s16le", "pipe:1").Output()
	if err != nil {
		t.Fatal(err)
	}
	rms := func(from, to int) float64 {
		start, end := from*48, to*48
		if len(pcm) < end*2 {
			t.Fatal("actual audio export was truncated")
		}
		var energy float64
		for i := start; i < end; i++ {
			value := float64(int16(binary.LittleEndian.Uint16(pcm[i*2 : i*2+2])))
			energy += value * value
		}
		return math.Sqrt(energy / float64(end-start))
	}
	middle := rms(450, 500)
	if middle < 100 || rms(30, 100) > 10 || rms(870, 950) > 10 || rms(205, 230) > middle*0.6 || rms(770, 795) > middle*0.6 {
		t.Fatalf("trim/gaps/mute/fades failed: pre=%f in=%f mid=%f out=%f post=%f", rms(30, 100), rms(205, 230), middle, rms(770, 795), rms(870, 950))
	}
}

func TestExportAudioRejectsVideoWithoutAnActualSoundStream(t *testing.T) {
	renderer, err := exportff.NewRenderer(mediaflow.FFProber{})
	if err != nil {
		t.Fatal(err)
	}
	asset, track := uuid.New(), uuid.New()
	config := canvasdomain.TimelineConfig{Version: 1, AspectRatio: "1:1", FPS: 25, SubtitleStyle: canvasdomain.SubtitleStyle{FontSize: 24, Color: "#ffffff", Position: "bottom"}, Tracks: []canvasdomain.TimelineTrack{{ID: track, Kind: "video", Label: "Silent video", Visible: true}}, Clips: []canvasdomain.TimelineClip{{ID: uuid.New(), TrackID: track, Kind: "video", AssetID: &asset, Title: "Silent", DurationMS: 200, Volume: 1}}}
	input := filepath.Join(t.TempDir(), "silent.mp4")
	if output, err := exec.CommandContext(t.Context(), "ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "color=c=red:s=64x64:r=25:d=0.2", "-an", "-c:v", "libx264", "-pix_fmt", "yuv420p", input).CombinedOutput(); err != nil {
		t.Fatalf("create actual silent video: %v %s", err, output)
	}
	result, err := renderer.Render(t.Context(), frozenAudio(t, config), map[uuid.UUID]string{asset: input}, nil)
	if result != nil {
		_ = result.Close()
	}
	if !errors.Is(err, exportapp.ErrInvalidExport) {
		t.Fatalf("missing audio stream was silently synthesized: %v", err)
	}
}

func frozenAudio(t *testing.T, config canvasdomain.TimelineConfig) exportdomain.FrozenExport {
	t.Helper()
	// Use the closed JSON contract in the Red test before the new field exists.
	raw, err := json.Marshal(map[string]any{"timeline": config, "output_kind": "audio"})
	if err != nil {
		t.Fatal(err)
	}
	var frozen exportdomain.FrozenExport
	if err := json.Unmarshal(raw, &frozen); err != nil {
		t.Fatal(err)
	}
	return frozen
}

func TestExportAudioKindIsFrozenImmutableAndBoundToIdempotentRequest(t *testing.T) {
	db := mediaStoreDB(t)
	actor, project := mediaStoreProject(t, db)
	config := exportTimeline(t)
	config.Tracks[0].Kind, config.Clips[0].Kind = "audio", "audio"
	asset := imageAsset(project, *config.Clips[0].AssetID)
	asset.Kind, asset.MimeType, asset.FileName = mediadomain.KindAudio, "audio/wave", "source.wav"
	asset.ObjectKey = "projects/" + project.String() + "/audio/2026/10/" + asset.ID.String() + ".wav"
	duration := int32(500)
	sha := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	asset.DurationMS, asset.SHA256 = &duration, &sha
	asset.Status, asset.ModerationStatus = mediadomain.StatusProcessing, mediadomain.ModerationPending
	if err := mediapg.NewStore(db).CreateAsset(t.Context(), actor, asset); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`UPDATE media.media_asset SET status='ready',moderation_status='passed' WHERE id=?`, asset.ID).Error; err != nil {
		t.Fatal(err)
	}
	store := exportpg.NewStore(db, func(*gorm.DB) exportapp.TimelineReader { return &fixedTimeline{timeline: config} }, func(tx *gorm.DB) exportapp.DerivedMedia { return mediaapp.NewDerivedService(mediapg.NewStore(tx)) })
	canvas, node, key := uuid.New(), uuid.New(), uuid.New()
	createInput := func(kind string) exportapp.CreateInput {
		var input exportapp.CreateInput
		raw, err := json.Marshal(map[string]any{"canvas_id": canvas, "node_id": node, "revision": 1, "output_kind": kind})
		if err != nil || json.Unmarshal(raw, &input) != nil {
			t.Fatal("closed audio creation input")
		}
		return input
	}
	job, err := store.Create(t.Context(), actor, project, key, createInput("audio"))
	if err != nil {
		t.Fatal(err)
	}
	var projection struct {
		OutputKind string `json:"output_kind"`
	}
	raw, err := json.Marshal(job)
	if err != nil || json.Unmarshal(raw, &projection) != nil || projection.OutputKind != "audio" {
		t.Fatalf("output kind not restored in safe job projection: %s %v", raw, err)
	}
	frozen, err := store.Frozen(t.Context(), actor, project, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	raw, err = json.Marshal(frozen)
	if err != nil || json.Unmarshal(raw, &projection) != nil || projection.OutputKind != "audio" {
		t.Fatal("audio output kind was not frozen")
	}
	if _, err := store.Create(t.Context(), actor, project, key, createInput("video")); !errors.Is(err, exportapp.ErrConflict) {
		t.Fatalf("changed output reused another artifact: %v", err)
	}
	if _, err := store.Create(t.Context(), actor, project, uuid.New(), createInput("image")); !errors.Is(err, exportapp.ErrInvalidExport) {
		t.Fatalf("unknown output kind queued: %v", err)
	}
	var canMutate bool
	if err := db.Raw(`SELECT has_column_privilege('lanverse_app','mediatool.export_job','output_kind','UPDATE')`).Scan(&canMutate).Error; err != nil || canMutate {
		t.Fatalf("runtime can alter frozen output kind: %v %v", canMutate, err)
	}
	if os.Getenv("LV_TEST_MEDIA_EXPORT_REQUIRE_ROLE") == "1" {
		err := db.Exec(`UPDATE mediatool.export_job SET output_kind='video' WHERE id=?`, job.ID).Error
		var denied *pgconn.PgError
		if !errors.As(err, &denied) || denied.Code != "42501" {
			t.Fatalf("non-owner output mutation was not denied: %v", err)
		}
		current, err := store.Get(t.Context(), actor, project, job.ID)
		if err != nil || current.OutputKind != exportdomain.OutputAudio {
			t.Fatalf("denied mutation changed frozen type: %+v %v", current, err)
		}
	}
}

func TestExportDefaultVideoPreservesPersistedFingerprintAndExplicitReplay(t *testing.T) {
	db := mediaStoreDB(t)
	actor, project := mediaStoreProject(t, db)
	config := exportTimeline(t)
	config.Tracks[0].Kind, config.Clips[0].Kind = "subtitle", "subtitle"
	config.Clips[0].AssetID, config.Clips[0].Text = nil, "中文"
	store := exportpg.NewStore(db, func(*gorm.DB) exportapp.TimelineReader { return &fixedTimeline{timeline: config} }, func(tx *gorm.DB) exportapp.DerivedMedia { return mediaapp.NewDerivedService(mediapg.NewStore(tx)) })
	input := exportapp.CreateInput{CanvasID: uuid.New(), NodeID: uuid.New(), Revision: 1}
	key := uuid.New()
	job, err := store.Create(t.Context(), actor, project, key, input)
	if err != nil || job.OutputKind != exportdomain.OutputVideo {
		t.Fatalf("default-video job: %+v %v", job, err)
	}
	// Reproduce the original pre-audio command's exact marshaled field order.
	legacyInput := struct {
		CanvasID uuid.UUID `json:"canvas_id"`
		NodeID   uuid.UUID `json:"node_id"`
		Revision int64     `json:"revision"`
	}{input.CanvasID, input.NodeID, input.Revision}
	raw, err := json.Marshal(struct {
		Action      string
		Project, ID uuid.UUID
		Input       any
	}{"create", project, uuid.Nil, legacyInput})
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	wantHash := hex.EncodeToString(digest[:])
	var hash string
	if err := db.Raw(`SELECT request_hash FROM mediatool.export_command WHERE actor_id=? AND request_id=?`, actor.ID, key).Scan(&hash).Error; err != nil || hash != wantHash {
		t.Fatalf("preexisting durable fingerprint drifted: got=%s want=%s err=%v", hash, wantHash, err)
	}
	explicit := input
	explicit.OutputKind = exportdomain.OutputVideo
	if repeated, err := store.Create(t.Context(), actor, project, key, explicit); err != nil || repeated.ID != job.ID {
		t.Fatalf("default and explicit video differ: %+v %v", repeated, err)
	}
	// An actual historical receipt lacks the new output field. Its permanent
	// request hash and original response remain intact; projection applies default.
	response, err := json.Marshal(job)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(response, &fields); err != nil {
		t.Fatal(err)
	}
	delete(fields, "output_kind")
	response, err = json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	oldKey := uuid.New()
	if err := db.Exec(`INSERT INTO mediatool.export_command(actor_id,request_id,job_id,request_hash,response,created_at) VALUES(?,?,?,?,?::jsonb,?)`, actor.ID, oldKey, job.ID, wantHash, string(response), time.Now().UTC()).Error; err != nil {
		t.Fatal(err)
	}
	if historical, err := store.Create(t.Context(), actor, project, oldKey, input); err != nil || historical.ID != job.ID || historical.OutputKind != exportdomain.OutputVideo {
		t.Fatalf("historical video replay changed: %+v %v", historical, err)
	}
}
