package media_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/config"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstorage"
)

func TestUploadRealMinIOOriginalRenditionsAndPreview(t *testing.T) {
	if os.Getenv("LV_TEST_OBJECT_STORAGE") != "1" || os.Getenv("LV_TEST_MEDIA_STORE_DB_DSN") == "" {
		t.Skip("set isolated LV_TEST_MEDIA_STORE_DB_DSN, LV_TEST_OBJECT_STORAGE=1 and related LV_ENV_FILE")
	}
	database := mediaStoreDB(t)
	actor, project := mediaStoreProject(t, database)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal("related object storage configuration unavailable")
	}
	objects, err := objectstorage.Open(cfg.ObjectStorageEndpoint, cfg.ObjectStorageBucket, cfg.ObjectStorageAccessKey, cfg.ObjectStorageSecretKey, cfg.ObjectStorageRegion)
	if err != nil {
		t.Fatal("private object store configuration invalid")
	}
	store := pgmedia.NewStore(database)
	service := mediaapp.NewUploadService(store, mediaflow.FFUploadProber{}, mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, mediaflow.NewUploadObjects(objects), time.Now)
	for _, tc := range []struct {
		name, kind string
		args       []string
		renditions int
	}{
		{"reference.png", "image", []string{"-f", "lavfi", "-i", "color=size=64x48", "-frames:v", "1"}, 2},
		{"reference.jpg", "image", []string{"-f", "lavfi", "-i", "color=size=64x48", "-frames:v", "1"}, 2},
		{"reference.webp", "image", []string{"-f", "lavfi", "-i", "color=size=64x48", "-frames:v", "1"}, 2},
		{"reference.mp4", "video", []string{"-f", "lavfi", "-i", "color=size=64x48:rate=10", "-t", "0.2", "-c:v", "libx264", "-pix_fmt", "yuv420p"}, 2},
		{"reference.mov", "video", []string{"-f", "lavfi", "-i", "color=size=64x48:rate=10", "-t", "0.2", "-c:v", "libx264", "-pix_fmt", "yuv420p"}, 2},
		{"reference.mp3", "audio", []string{"-f", "lavfi", "-i", "sine=frequency=440:duration=0.2", "-c:a", "libmp3lame"}, 1},
		{"reference.wav", "audio", []string{"-f", "lavfi", "-i", "sine=frequency=440:duration=0.2", "-c:a", "pcm_s16le"}, 1},
		{"reference.m4a", "audio", []string{"-f", "lavfi", "-i", "sine=frequency=440:duration=0.2", "-c:a", "aac"}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := filepath.Join(t.TempDir(), tc.name)
			args := append([]string{"-nostdin", "-hide_banner", "-loglevel", "error", "-y"}, tc.args...)
			if tc.name == "reference.webp" {
				// A locally encoded 4x4 black WebP avoids requiring an optional
				// FFmpeg WebP encoder while still exercising its real decoder.
				data, err := base64.StdEncoding.DecodeString("UklGRiQAAABXRUJQVlA4IBgAAAAwAQCdASoEAAQAAgA0JaQAA3AA/vv9UAA=")
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(fixture, data, 0600); err != nil {
					t.Fatal(err)
				}
			} else if output, err := exec.CommandContext(t.Context(), "ffmpeg", append(args, fixture)...).CombinedOutput(); err != nil {
				t.Fatalf("create media fixture: %v: %s", err, output)
			}
			reader, err := os.Open(fixture)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = reader.Close() }()
			file, err := mediaapp.ReadUpload(t.Context(), reader, tc.name)
			if err != nil {
				t.Fatalf("read real %s: %v", tc.name, err)
			}
			defer func() { _ = file.Close() }()
			in := uploadInput(t)
			in.Actor = actor
			in.Request.ProjectID = project
			in.Request.FileName = tc.name
			in.File = file
			result, err := service.Upload(t.Context(), in)
			if err != nil {
				t.Fatalf("real %s upload failed: %v", tc.name, err)
			}
			asset, err := store.FindAsset(t.Context(), actor, project, result.Asset.ID)
			if err != nil {
				t.Fatal(err)
			}
			rends, err := store.FindRenditions(t.Context(), actor, project, result.Asset.ID)
			if err != nil {
				t.Fatal(err)
			}
			keys := []string{asset.ObjectKey}
			for _, rendition := range rends {
				keys = append(keys, rendition.ObjectKey)
			}
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				for _, key := range keys {
					if err := objects.Remove(ctx, key); err != nil {
						t.Error("exact test object cleanup failed")
					}
				}
			})
			if result.Asset.Kind != tc.kind || !asset.CanReference() || len(rends) != tc.renditions {
				t.Fatalf("upload ready result=%+v renditions=%d", result, len(rends))
			}
			for _, rendition := range rends {
				info, err := objects.Stat(t.Context(), rendition.ObjectKey)
				if err != nil || rendition.ByteSize == nil || *rendition.ByteSize != info.Size || info.SHA256 == "" || rendition.Width == nil || rendition.Height == nil {
					t.Fatalf("real %s rendition verification failed", rendition.Kind)
				}
			}
			preview, err := mediaapp.NewAssetQuery(store, objects).Preview(t.Context(), actor, project, result.Asset.ID)
			if err != nil {
				t.Fatal("authorized preview signing failed")
			}
			req, err := http.NewRequestWithContext(t.Context(), "GET", preview.URL, nil)
			if err != nil {
				t.Fatal("preview request invalid")
			}
			response, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal("private original preview retrieval failed")
			}
			defer func() { _ = response.Body.Close() }()
			hash := sha256.New()
			size, err := io.Copy(hash, io.LimitReader(response.Body, file.Size+1))
			if err != nil || response.StatusCode != 200 || size != file.Size || hex.EncodeToString(hash.Sum(nil)) != file.SHA256 {
				t.Fatalf("retrieved %s differs from uploaded bytes: status=%d size=%d", tc.name, response.StatusCode, size)
			}
		})
	}
}
