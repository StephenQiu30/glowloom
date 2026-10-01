package media_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
)

type replayNormalizationSpy struct{ calls int }

func (s *replayNormalizationSpy) Normalize(context.Context, *mediaapp.Downloaded) (mediaapp.NormalizedUpload, error) {
	s.calls++
	return mediaapp.NormalizedUpload{}, mediaapp.ErrUnavailable
}

func TestNormalizedUploadStoreKeepsOriginalReceiptAndDeduplicatesSource(t *testing.T) {
	database := mediaStoreDB(t)
	actor, project := mediaStoreProject(t, database)
	store, objects := pgmedia.NewStore(database), &uploadObjectsFake{}
	service := mediaapp.NewUploadService(store, mediaflow.FFUploadProber{}, mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, objects, time.Now)
	in := uploadInput(t)
	in.Actor, in.Request.ProjectID = actor, project
	in.File, in.Request.FileName = readUploadWebM(t, uploadWebMFixture(t, "libvpx-vp9", "0.6")), "白膜镜头.webm"
	first, err := service.Upload(t.Context(), in)
	if err != nil {
		t.Fatal(err)
	}
	asset, err := store.FindAsset(t.Context(), actor, project, first.Asset.ID)
	if err != nil || !asset.CanReference() || asset.MimeType != "video/mp4" || asset.Codec == nil || *asset.Codec != "h264" || asset.SHA256 == nil || *asset.SHA256 == in.File.SHA256 {
		t.Fatalf("real normalized asset=%+v err=%v", asset, err)
	}
	var receipt struct {
		SHA256   string
		FileName string
		ByteSize int64
		AssetID  uuid.UUID
	}
	if err := database.Raw(`SELECT sha256,file_name,byte_size,asset_id FROM media.upload_request WHERE project_id=? AND request_key=?`, project, in.Request.Key).Scan(&receipt).Error; err != nil || receipt.SHA256 != in.File.SHA256 || receipt.FileName != in.Request.FileName || receipt.ByteSize != in.File.Size || receipt.AssetID != first.Asset.ID {
		t.Fatalf("original input receipt=%+v err=%v", receipt, err)
	}
	spy := &replayNormalizationSpy{}
	replayService := mediaapp.NewUploadService(store, mediaflow.FFUploadProber{}, spy, mediaflow.FFUploadRenderer{}, objects, time.Now)
	replayed, err := replayService.Upload(t.Context(), in)
	if err != nil || !reflect.DeepEqual(replayed, first) || spy.calls != 0 {
		t.Fatalf("receipt replay transcoded again: result=%+v err=%v calls=%d", replayed, err, spy.calls)
	}
	in.Request.FileName = "新名.webm"
	if _, err := replayService.Upload(t.Context(), in); !errors.Is(err, mediaapp.ErrUploadConflict) || spy.calls != 0 {
		t.Fatalf("changed body key reached normalizer: %v calls=%d", err, spy.calls)
	}
	in.Request.Key = uuid.New()
	reused, err := service.Upload(t.Context(), in)
	if err != nil || reused.Asset.ID != first.Asset.ID || reused.DuplicateOf == nil || *reused.DuplicateOf != first.Asset.ID || len(objects.items) != 3 {
		t.Fatalf("same source cross-key reuse=%+v err=%v objects=%d", reused, err, len(objects.items))
	}
	var counts struct{ Assets, Receipts int64 }
	if err := database.Raw(`SELECT (SELECT count(*) FROM media.media_asset WHERE project_id=?) AS assets,(SELECT count(*) FROM media.upload_request WHERE project_id=?) AS receipts`, project, project).Scan(&counts).Error; err != nil || counts.Assets != 1 || counts.Receipts != 2 {
		t.Fatalf("normalized durable counts=%+v err=%v", counts, err)
	}
}

func TestNormalizedUploadStoreRejectsAbsentForgedAndUnknownConversionProof(t *testing.T) {
	database := mediaStoreDB(t)
	actor, project := mediaStoreProject(t, database)
	store, objects := pgmedia.NewStore(database), &uploadObjectsFake{}
	record := &uploadRepoFake{}
	in := uploadInput(t)
	in.Actor, in.Request.ProjectID = actor, project
	in.File, in.Request.FileName = readUploadWebM(t, uploadWebMFixture(t, "libvpx", "0.3")), "白膜镜头.webm"
	service := mediaapp.NewUploadService(record, mediaflow.FFUploadProber{}, mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, objects, time.Now)
	if _, err := service.Upload(t.Context(), in); err != nil {
		t.Fatal(err)
	}
	request := in.Request
	request.SHA256, request.ByteSize = in.File.SHA256, in.File.Size
	for _, tc := range []struct {
		name string
		edit func(map[string]any)
	}{
		{"absent proof still requires original sha equality", func(detail map[string]any) { delete(detail, "normalization") }},
		{"changed source sha", func(detail map[string]any) {
			detail["normalization"].(map[string]any)["source"].(map[string]any)["sha256"] = *record.asset.SHA256
		}},
		{"changed canonical size", func(detail map[string]any) {
			detail["normalization"].(map[string]any)["canonical"].(map[string]any)["byte_size"] = 1
		}},
		{"unknown conversion url", func(detail map[string]any) {
			detail["normalization"].(map[string]any)["url"] = "http://example.invalid/canonical.mp4"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			asset := record.asset
			var detail map[string]any
			if err := json.Unmarshal(asset.ModerationDetail, &detail); err != nil {
				t.Fatal(err)
			}
			tc.edit(detail)
			asset.ModerationDetail, _ = json.Marshal(detail)
			if _, err := store.CommitUpload(t.Context(), actor, request, asset, record.rends); !errors.Is(err, mediaapp.ErrInvalidUpload) {
				t.Fatalf("forged proof reached persistence: %v", err)
			}
		})
	}
}

func TestNormalizedUploadUnknownCommitPreservesOwnedCanonicalObjects(t *testing.T) {
	for _, tc := range []struct {
		name      string
		exists    bool
		existsErr error
		objects   int
	}{
		{"known rollback", false, nil, 0},
		{"durable commit", true, nil, 3},
		{"unknown durable owner", false, mediaapp.ErrUnavailable, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &uploadRepoFake{commitErr: context.Canceled, exists: tc.exists, existsErr: tc.existsErr}
			objects := &uploadObjectsFake{}
			in := uploadInput(t)
			in.File, in.Request.FileName = readUploadWebM(t, uploadWebMFixture(t, "libvpx", "0.3")), "白膜镜头.webm"
			service := mediaapp.NewUploadService(repo, mediaflow.FFUploadProber{}, mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, objects, time.Now)
			if _, err := service.Upload(t.Context(), in); !errors.Is(err, context.Canceled) || len(objects.items) != tc.objects {
				t.Fatalf("canonical owner cleanup err=%v objects=%d want=%d", err, len(objects.items), tc.objects)
			}
		})
	}
}
