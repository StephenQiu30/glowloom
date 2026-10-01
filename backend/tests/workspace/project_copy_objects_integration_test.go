package workspace_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	canvaspg "github.com/StephenQiu30/lanverse/backend/internal/canvas/adapter/postgres"
	canvasapp "github.com/StephenQiu30/lanverse/backend/internal/canvas/application"
	cd "github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/adapter/gltf"
	mo "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/objectstorage"
	mediapg "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mf "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	ma "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstorage"
	wp "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
	wa "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	wd "github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

func projectCopyObjects(t *testing.T) *objectstorage.Client {
	t.Helper()
	path := os.Getenv("LV_TEST_RECEIPT_STORAGE_CONFIG")
	if path == "" {
		t.Skip("set isolated LV_TEST_RECEIPT_STORAGE_CONFIG; no business configuration is read")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("read synthetic copy storage configuration")
	}
	var cfg struct {
		Endpoint  string `json:"endpoint"`
		Bucket    string `json:"bucket"`
		AccessKey string `json:"access_key"`
		SecretKey string `json:"secret_key"`
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&cfg) != nil || d.Decode(new(any)) != io.EOF {
		t.Fatal("invalid synthetic copy storage configuration")
	}
	u, err := url.Parse(cfg.Endpoint)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || cfg.Bucket != "lanverse-receipt-test" {
		t.Fatal("copy storage must be the isolated loopback test bucket")
	}
	objects, err := objectstorage.Open(cfg.Endpoint, cfg.Bucket, cfg.AccessKey, cfg.SecretKey, "")
	if err != nil {
		t.Fatal("open synthetic private copy storage")
	}
	return objects
}
func copyPNG(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 16, 16))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func copyGLB(t *testing.T) []byte {
	t.Helper()
	document := map[string]any{"asset": map[string]any{"version": "2.0"}, "scene": 0, "scenes": []any{map[string]any{"nodes": []int{0}}}, "nodes": []any{map[string]any{"mesh": 0}}, "meshes": []any{map[string]any{"primitives": []any{map[string]any{"attributes": map[string]any{"POSITION": 0}}}}}, "accessors": []any{map[string]any{"bufferView": 0, "componentType": 5126, "count": 3, "type": "VEC3", "min": []int{0, 0, 0}, "max": []int{1, 1, 0}}}, "bufferViews": []any{map[string]any{"buffer": 0, "byteLength": 36}}, "buffers": []any{map[string]any{"byteLength": 36}}}
	js, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	for len(js)%4 != 0 {
		js = append(js, ' ')
	}
	var geometry bytes.Buffer
	for _, v := range []float32{0, 0, 0, 1, 0, 0, 0, 1, 0} {
		if err := binary.Write(&geometry, binary.LittleEndian, v); err != nil {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	for _, v := range []uint32{0x46546c67, 2, uint32(28 + len(js) + geometry.Len()), uint32(len(js)), 0x4e4f534a} {
		if err := binary.Write(&out, binary.LittleEndian, v); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := out.Write(js); err != nil {
		t.Fatal(err)
	}
	for _, v := range []uint32{uint32(geometry.Len()), 0x004e4942} {
		if err := binary.Write(&out, binary.LittleEndian, v); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := out.Write(geometry.Bytes()); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
func copyAV(t *testing.T, kind string) []byte {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("actual copy AV fixtures require ffmpeg")
	}
	path := filepath.Join(t.TempDir(), "source."+map[string]string{"video": "mp4", "audio": "wav"}[kind])
	args := []string{"-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i"}
	if kind == "video" {
		args = append(args, "color=c=blue:s=16x16:r=24:d=1", "-c:v", "libx264", "-pix_fmt", "yuv420p")
	} else {
		args = append(args, "sine=frequency=440:duration=1", "-c:a", "pcm_s16le")
	}
	args = append(args, path)
	if err := exec.CommandContext(t.Context(), "ffmpeg", args...).Run(); err != nil {
		t.Fatal("generate actual synthetic copy AV", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
func seedProjectCopyContent(ctx context.Context, t *testing.T, db *gorm.DB, objects *objectstorage.Client, all bool) (identityapp.Principal, uuid.UUID, map[string]uuid.UUID, uuid.UUID) {
	t.Helper()
	actor, project := lifecycleActorProject(ctx, t, db)
	validator, err := gltf.NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	upload := ma.NewUploadService(mediapg.NewStore(db), mf.NewUploadProber(validator), mf.FFUploadNormalizer{}, mf.FFUploadRenderer{}, mf.NewUploadObjects(objects), time.Now)
	samples := []struct {
		Kind, Name string
		Bytes      []byte
	}{{"image", "导演截图.png", copyPNG(t)}}
	if all {
		samples = append(samples, struct {
			Kind, Name string
			Bytes      []byte
		}{"video", "短片.mp4", copyAV(t, "video")}, struct {
			Kind, Name string
			Bytes      []byte
		}{"audio", "配音.wav", copyAV(t, "audio")}, struct {
			Kind, Name string
			Bytes      []byte
		}{"model", "演员.glb", copyGLB(t)})
	}
	assets := map[string]uuid.UUID{}
	for _, sample := range samples {
		file, err := ma.ReadUpload(ctx, bytes.NewReader(sample.Bytes), sample.Name)
		if err != nil {
			t.Fatal(err)
		}
		result, uploadErr := upload.Upload(ctx, ma.UploadInput{Actor: actor, Request: ma.UploadRequest{ProjectID: project, Key: uuid.New(), SHA256: file.SHA256, ByteSize: file.Size, FileName: sample.Name, RequestID: uuid.New()}, File: file, LocalReviewConfirmed: true})
		if err := errors.Join(uploadErr, file.Close()); err != nil {
			t.Fatal("real source upload", err)
		}
		assets[sample.Kind] = result.Asset.ID
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var keys []string
		if err := db.WithContext(cleanup).Raw(`SELECT a.object_key FROM media.media_asset a WHERE a.project_id=? OR a.project_id IN(SELECT target_project_id FROM workspace.project_copy_job WHERE source_project_id=?) UNION SELECT r.object_key FROM media.rendition r JOIN media.media_asset a ON a.id=r.media_asset_id WHERE a.project_id=? OR a.project_id IN(SELECT target_project_id FROM workspace.project_copy_job WHERE source_project_id=?)`, project, project, project, project).Scan(&keys).Error; err != nil {
			t.Error("collect scoped copy fixture keys")
			return
		}
		for _, key := range keys {
			if err := objects.Remove(cleanup, key); err != nil {
				t.Error("remove exact task copy fixture key")
			}
		}
	})
	canvas := canvasapp.NewService(canvaspg.NewStore(db, func(tx *gorm.DB) canvasapp.MediaReader { return ma.NewAssetQuery(mediapg.NewStore(tx), nil) }))
	doc, err := canvas.Create(ctx, actor, project, uuid.NewString(), canvasapp.CreateInput{Name: "全部媒体与导演"})
	if err != nil {
		t.Fatal(err)
	}
	var nodes []cd.Node
	for kind, asset := range assets {
		id := asset
		nodes = append(nodes, cd.Node{ID: uuid.New(), Title: kind, NodeType: kind, NodeAction: "resource", RefType: "media_asset", RefID: &id})
	}
	camera, shot := uuid.New(), uuid.New()
	transform := cd.DirectorTransform{Position: []float64{0, 0, 0}, Rotation: []float64{0, 0, 0}, Scale: []float64{1, 1, 1}}
	imageAsset := assets["image"]
	director := cd.DirectorConfig{ID: uuid.New(), Version: 1, Title: "完整导演图库", Background: "#ffffff", EnvironmentIntensity: 1, ActiveShotID: shot, Objects: []cd.DirectorObject{}, Cameras: []cd.DirectorCamera{{ID: camera, Name: "摄影机", Transform: transform, Target: []float64{0, 1, 0}, FocalLength: 35, FOV: 54, Aperture: 2.8, FocusDistance: 5, Near: 0.1, Far: 100, Keyframes: []cd.DirectorKeyframe{}}}, Lights: []cd.DirectorLight{}, Shots: []cd.DirectorShot{{ID: shot, Name: "镜头", CameraID: camera, Duration: 1, FPS: 24, ShotSize: "medium", CameraMove: "static", Screenshots: []cd.DirectorScreenshot{{ID: uuid.New(), AssetID: imageAsset, Name: "保留图库", CreatedAt: time.Date(2026, 10, 1, 1, 0, 0, 0, time.UTC)}}}}, Cover: &cd.DirectorCover{AssetID: imageAsset, ShotID: shot}, Panorama: &cd.DirectorPanorama{AssetID: imageAsset}}
	nodes = append(nodes, cd.Node{ID: uuid.New(), Title: "导演台", NodeType: "director", NodeAction: "tool", Config: cd.NodeConfig{Director: &director}})
	if _, err := canvas.Execute(ctx, actor, doc.ID, uuid.NewString(), canvasapp.CommandsInput{ExpectedRevision: 1, Commands: []cd.Command{{Type: "AddNodes", Nodes: nodes}}}); err != nil {
		t.Fatal("save actual complete source graph", err)
	}
	extra, err := canvas.Create(ctx, actor, project, uuid.NewString(), canvasapp.CreateInput{Name: "时间轴"})
	if err != nil {
		t.Fatal(err)
	}
	track := uuid.New()
	timeline := cd.TimelineConfig{Version: 1, AspectRatio: "16:9", FPS: 24, SubtitleStyle: cd.SubtitleStyle{FontSize: 24, Color: "#ffffff", Position: "bottom"}, Tracks: []cd.TimelineTrack{{ID: track, Kind: "image", Label: "画面", Visible: true}}, Clips: []cd.TimelineClip{{ID: uuid.New(), TrackID: track, Kind: "image", AssetID: &imageAsset, Title: "封面", DurationMS: 1000, Volume: 1}}}
	if _, err := canvas.Execute(ctx, actor, extra.ID, uuid.NewString(), canvasapp.CommandsInput{ExpectedRevision: 1, Commands: []cd.Command{{Type: "AddNodes", Nodes: []cd.Node{{ID: uuid.New(), Title: "剪辑", NodeType: "timeline", NodeAction: "tool", Config: cd.NodeConfig{Timeline: &timeline}}}}}}); err != nil {
		t.Fatal(err)
	}
	return actor, project, assets, doc.ID
}
func actualCopyWorker(t *testing.T, db *gorm.DB, objects ma.ProjectCopyObjects) *wa.ProjectCopyWorker {
	t.Helper()
	store := projectCopyStore(db)
	temp := t.TempDir()
	return wa.NewProjectCopyWorker(store, func(job wd.ProjectCopyJob, worker uuid.UUID, cleanup bool) *ma.ProjectCopyTransfer {
		return ma.NewProjectCopyTransfer(wp.NewProjectCopyMediaObjects(store, job.ID, worker, cleanup), objects, temp)
	}, time.Now)
}
func TestProjectCopyRealCompletePrivateMediaAndIndependentRefresh(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	objects := projectCopyObjects(t)
	actor, source, assets, _ := seedProjectCopyContent(ctx, t, db, objects, true)
	store := projectCopyStore(db)
	job, err := store.Create(ctx, actor, copyInput(source), time.Now())
	if err != nil || job.Manifest.Assets != 4 || job.Manifest.Renditions < 4 || job.Manifest.Documents != 2 {
		t.Fatal("whole copy manifest", job.Manifest, err)
	}
	worker := uuid.New()
	finished, err := actualCopyWorker(t, db, mo.NewProjectCopyObjects(objects)).Execute(ctx, wa.ProjectCopyWorkID{OrgID: actor.OrgID, JobID: job.ID, WorkerID: worker})
	if err != nil || finished.Status != "succeeded" {
		t.Fatal("real owning copy execution", finished.Status, finished.FailureCode, err)
	}
	query := ma.NewAssetQuery(mediapg.NewStore(db), objects)
	items, err := query.List(ctx, actor, finished.TargetProjectID, "", "", 100)
	if err != nil || len(items.Items) != 4 {
		t.Fatal("fresh complete media library", items, err)
	}
	var sourceKeys []string
	for _, sourceAsset := range assets {
		a, err := mediapg.NewStore(db).FindAsset(ctx, actor, source, sourceAsset)
		if err != nil {
			t.Fatal(err)
		}
		sourceKeys = append(sourceKeys, a.ObjectKey)
		r, err := mediapg.NewStore(db).FindRenditions(ctx, actor, source, sourceAsset)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range r {
			sourceKeys = append(sourceKeys, item.ObjectKey)
		}
	}
	for _, key := range sourceKeys {
		if err := objects.Remove(ctx, key); err != nil {
			t.Fatal("remove only synthetic source fixture")
		}
	}
	for _, asset := range items.Items {
		for _, old := range assets {
			if asset.ID == old {
				t.Fatal("copied identity shares source")
			}
		}
		actual, err := mediapg.NewStore(db).FindAsset(ctx, actor, finished.TargetProjectID, asset.ID)
		if err != nil {
			t.Fatal(err)
		}
		reader, err := objects.Get(ctx, actual.ObjectKey)
		if err != nil {
			t.Fatal("independent original lost with synthetic source")
		}
		size, readErr := io.Copy(io.Discard, reader)
		if err := errors.Join(readErr, reader.Close()); err != nil || size != actual.ByteSize {
			t.Fatal("real private original readback", err)
		}
		renditions, err := mediapg.NewStore(db).FindRenditions(ctx, actor, finished.TargetProjectID, asset.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range renditions {
			exists, err := objects.Exists(ctx, r.ObjectKey)
			if err != nil || !exists {
				t.Fatal("independent rendition missing", err)
			}
		}
	}
	canvas := canvasapp.NewService(canvaspg.NewStore(db, nil))
	docs, err := canvas.List(ctx, actor, finished.TargetProjectID)
	if err != nil || len(docs) != 2 {
		t.Fatal("fresh complete canvas list", err)
	}
	seenDirector, seenTimeline := false, false
	for _, doc := range docs {
		fresh, err := canvas.Get(ctx, actor, doc.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, node := range fresh.Nodes {
			if node.Config.Director != nil {
				seenDirector = true
				c := node.Config.Director
				if c.Cover == nil || c.Cover.AssetID == assets["image"] || c.Cover.AssetID != c.Shots[0].Screenshots[0].AssetID || c.Cover.ShotID != c.Shots[0].ID {
					t.Fatal("gallery/cover references not complete")
				}
			}
			if node.Config.Timeline != nil {
				seenTimeline = true
				if *node.Config.Timeline.Clips[0].AssetID == assets["image"] {
					t.Fatal("timeline retained source identity")
				}
			}
		}
	}
	if !seenDirector || !seenTimeline {
		t.Fatal("source tools lost")
	}
}

type unknownCopyObjects struct {
	ma.ProjectCopyObjects
	afterWrite bool
	failed     bool
}

func (o *unknownCopyObjects) PutIfAbsent(ctx context.Context, key string, reader io.Reader, size int64, mime, digest string) error {
	if !o.failed {
		o.failed = true
		if o.afterWrite {
			if err := o.ProjectCopyObjects.PutIfAbsent(ctx, key, reader, size, mime, digest); err != nil {
				return err
			}
		}
		return io.ErrUnexpectedEOF
	}
	return o.ProjectCopyObjects.PutIfAbsent(ctx, key, reader, size, mime, digest)
}
func TestProjectCopyRealUnknownRecoveryAndCancellation(t *testing.T) {
	for _, tc := range []struct {
		Name               string
		Cancel, AfterWrite bool
	}{{"same frozen attempt readback", false, true}, {"owned cleanup after actual write", true, true}, {"absent unknown write stays unresolved", true, false}} {
		t.Run(tc.Name, func(t *testing.T) {
			ctx, db := workspaceMigrationDB(t)
			objects := projectCopyObjects(t)
			actor, source, _, _ := seedProjectCopyContent(ctx, t, db, objects, false)
			store := projectCopyStore(db)
			job, err := store.Create(ctx, actor, copyInput(source), time.Now())
			if err != nil {
				t.Fatal(err)
			}
			adapter := mo.NewProjectCopyObjects(objects)
			fault := &unknownCopyObjects{ProjectCopyObjects: adapter, afterWrite: tc.AfterWrite}
			failed, err := actualCopyWorker(t, db, fault).Execute(ctx, wa.ProjectCopyWorkID{OrgID: actor.OrgID, JobID: job.ID, WorkerID: uuid.New()})
			if err != nil || failed.Status != "failed" || !failed.NeedsReconciliation || failed.WorkerID != uuid.Nil || failed.Attempt != 1 {
				t.Fatal("unknown physical write discarded", failed.Status, failed.FailureCode, err)
			}
			if _, err := store.Change(ctx, actor, job.ID, "retry", failed.Revision, uuid.New(), uuid.NewString()); !errors.Is(err, wd.ErrProjectCopyStateConflict) {
				t.Fatal("unknown write automatically retried", err)
			}
			if tc.Cancel {
				failed, err = store.Change(ctx, actor, job.ID, "cancel", failed.Revision, uuid.New(), uuid.NewString())
				if err != nil || failed.Status == "cancelled" || !failed.CancellationRequested {
					t.Fatal("cancel intent falsely completed", err)
				}
			}
			reconciled, err := store.Change(ctx, actor, job.ID, "reconcile", failed.Revision, uuid.New(), uuid.NewString())
			if err != nil || !reconciled.ReconciliationRequested {
				t.Fatal("explicit recovery intent", err)
			}
			result, err := actualCopyWorker(t, db, adapter).Execute(ctx, wa.ProjectCopyWorkID{OrgID: actor.OrgID, JobID: job.ID, WorkerID: uuid.New()})
			if err != nil || result.Manifest != job.Manifest || result.TargetProjectID != job.TargetProjectID || result.Attempt != 1 {
				t.Fatal("recovery changed frozen identity", err)
			}
			expected := "succeeded"
			if tc.Cancel {
				expected = "cancelled"
			}
			if !tc.AfterWrite {
				expected = "failed"
			}
			if result.Status != expected {
				t.Fatal("recovery result", result.Status, result.FailureCode)
			}
			if !tc.AfterWrite && (!result.NeedsReconciliation || result.FailureCode != "object_absence_unknown") {
				t.Fatal("unknown absent object treated as stopped")
			}
			var sourceKeys []string
			if err := db.Raw(`SELECT object_key FROM media.media_asset WHERE project_id=? UNION SELECT r.object_key FROM media.rendition r JOIN media.media_asset a ON a.id=r.media_asset_id WHERE a.project_id=?`, source, source).Scan(&sourceKeys).Error; err != nil {
				t.Fatal(err)
			}
			for _, key := range sourceKeys {
				exists, err := objects.Exists(ctx, key)
				if err != nil || !exists {
					t.Fatal("cancel/recovery touched source", err)
				}
			}
			blocked, err := store.HasInflightWork(ctx, actor, source)
			if err != nil || blocked == (result.Status == "succeeded" || result.Status == "cancelled") {
				t.Fatal("copy pin lifecycle truth", blocked, err)
			}
			if tc.Cancel && tc.AfterWrite {
				var targets []string
				if err := db.Raw(`SELECT target_object_key FROM media.project_copy_object WHERE snapshot_id=?`, result.Manifest.MediaSnapshotID).Scan(&targets).Error; err != nil {
					t.Fatal(err)
				}
				for _, key := range targets {
					exists, err := objects.Exists(ctx, key)
					if err != nil || exists {
						t.Fatal("owned private cleanup incomplete", err)
					}
				}
			}
		})
	}
}
func TestProjectCopyRealNonOwnerPermissionsAndObjectFence(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	objects := projectCopyObjects(t)
	actor, source, _, _ := seedProjectCopyContent(ctx, t, db, objects, false)
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE lanverse_app`).Error; err != nil {
			return err
		}
		var role struct{ Runtime, Superuser, Owner, FrozenUpdate, HistoryDelete bool }
		if err := tx.Raw(`SELECT current_user='lanverse_app' AS runtime,(SELECT rolsuper FROM pg_roles WHERE rolname=current_user) AS superuser,(SELECT tableowner=current_user FROM pg_tables WHERE schemaname='workspace' AND tablename='project_copy_job') AS owner,has_column_privilege(current_user,'workspace.project_copy_job','manifest','UPDATE') AS frozen_update,has_table_privilege(current_user,'workspace.project_copy_job','DELETE') AS history_delete`).Scan(&role).Error; err != nil {
			return err
		}
		if !role.Runtime || role.Superuser || role.Owner || role.FrozenUpdate || role.HistoryDelete {
			return errors.New("runtime copy privilege contract violated")
		}
		store := projectCopyStore(tx)
		job, err := store.Create(ctx, actor, copyInput(source), time.Now())
		if err != nil {
			return err
		}
		worker := uuid.New()
		claimed, err := store.Claim(ctx, actor, job.ID, worker, false, time.Now())
		if err != nil {
			return err
		}
		binding := ma.ProjectCopyBinding{JobID: job.ID, OrgID: actor.OrgID, SourceProjectID: source, TargetProjectID: job.TargetProjectID}
		snapshot := ma.ProjectCopySnapshot{ID: job.Manifest.MediaSnapshotID, ManifestSHA256: job.Manifest.MediaSHA256, Assets: job.Manifest.Assets, Renditions: job.Manifest.Renditions}
		stale := wp.NewProjectCopyMediaObjects(store, job.ID, uuid.New(), false)
		if _, err := stale.Objects(ctx, actor, binding, snapshot); !errors.Is(err, wd.ErrProjectCopyWorkerConflict) {
			return errors.New("stale object writer admitted")
		}
		// Stop this synthetic worker before issuing a new explicit retry; its bytes never started.
		failed, err := store.Fail(ctx, actor, job.ID, worker, "synthetic_stopped", true, false)
		if err != nil {
			return err
		}
		if _, err := store.Change(ctx, actor, job.ID, "retry", failed.Revision, uuid.New(), uuid.NewString()); err != nil {
			return err
		}
		if _, err := wp.NewProjectCopyMediaObjects(store, job.ID, claimed.WorkerID, false).Objects(ctx, actor, binding, snapshot); !errors.Is(err, wd.ErrProjectCopyWorkerConflict) {
			return errors.New("old attempt retains object authority")
		}
		finished, err := actualCopyWorker(t, tx, mo.NewProjectCopyObjects(objects)).Execute(ctx, wa.ProjectCopyWorkID{OrgID: actor.OrgID, JobID: job.ID, WorkerID: uuid.New()})
		if err != nil {
			return err
		}
		if finished.Status != "succeeded" || finished.Attempt != 2 {
			return errors.New("nonowner real copy did not complete")
		}
		return nil
	})
	if err != nil {
		t.Fatal("actual nonowner copy", err)
	}
}
