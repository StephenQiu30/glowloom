package media_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	mediaflow "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

type generatedAudioObjects struct {
	files map[string][]byte
	wrong bool
}

func (o *generatedAudioObjects) CreateGeneratedAudioObject(_ context.Context, intent mediaapp.StorageWorkIntent, key string, file *mediaapp.Downloaded) (domain.ObjectLocation, error) {
	if o.files == nil {
		o.files = map[string][]byte{}
	}
	if _, err := file.File.Seek(0, io.SeekStart); err != nil {
		return domain.ObjectLocation{}, err
	}
	data, err := io.ReadAll(file.File)
	if err != nil {
		return domain.ObjectLocation{}, err
	}
	if existing, ok := o.files[key]; ok && !bytes.Equal(existing, data) {
		return domain.ObjectLocation{}, mediaapp.ErrObjectMismatch
	}
	o.files[key] = data
	location := domain.ObjectLocation{ID: uuid.NewSHA1(uuid.NameSpaceOID, []byte(key)), Ref: domain.ObjectRef{ProfileID: intent.ProfileID, Bucket: intent.Bucket, Key: key}, State: "present", ByteSize: &file.Size, SHA256: &file.SHA256, MIMEType: &file.MIMEType}
	if o.wrong {
		location.Ref.ProfileID = "different-profile"
	}
	return location, nil
}

func generatedAudioInput(t *testing.T) (mediaapp.PersonalGeneratedAudioRequest, *mediaapp.Downloaded) {
	t.Helper()
	name := filepath.Join(t.TempDir(), "sample.mp3")
	cmd := exec.CommandContext(t.Context(), "ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "sine=frequency=440:duration=0.2", "-c:a", "libmp3lame", name)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("make real generated audio fixture: %v: %s", err, output)
	}
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	file, err := mediaapp.ReadUpload(t.Context(), bytes.NewReader(data), "sample.mp3")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	job, owner := uuid.New(), uuid.New()
	r := mediaapp.PersonalGeneratedAudioRequest{
		AssetID: uuid.New(), OwnerScopeID: owner,
		Personal:    domain.PersonalOwnership{OrgID: uuid.New(), ActorID: uuid.New()},
		SourceJobID: job, FileName: "智能配音1.mp3", CreatedAt: time.Date(2026, 10, 28, 1, 2, 3, 0, time.UTC),
		SHA256: file.SHA256, ByteSize: file.Size, MIMEType: "audio/mpeg",
		Intent:     mediaapp.StorageWorkIntent{ID: uuid.New(), Owner: mediaapp.StorageWorkOwner{Kind: "subject-voice", OwnerID: owner, WorkID: job}, ProfileID: "minio-main", Bucket: "test-media", ConfigRevision: "frozen-1", State: "open"},
		Provenance: mediaapp.PersonalGeneratedAudioProvenance{SchemaVersion: 1, ReceiptSHA256: strings.Repeat("a", 64), ReleaseID: uuid.New(), ModelVersionID: uuid.New(), ProviderRequestID: job, ProviderLogID: "actual-log-id", TerminalCode: 20000000},
	}
	return r, file
}

func generatedAudioService(objects *generatedAudioObjects) *mediaapp.PersonalGeneratedAudioService {
	return mediaapp.NewPersonalGeneratedAudioService(mediaflow.FFProber{}, mediaflow.FFUploadRenderer{}, objects, func() time.Time { return time.Date(2026, 10, 28, 2, 0, 0, 0, time.UTC) })
}

func TestPersonalGeneratedAudioPreparesActualBytesAndStableWaveformWithoutUploadReview(t *testing.T) {
	r, file := generatedAudioInput(t)
	objects := &generatedAudioObjects{}
	service := generatedAudioService(objects)
	first, err := service.Prepare(t.Context(), r, file)
	if err != nil {
		t.Fatal(err)
	}
	if first.Validate() != nil || first.Asset.Validate() != nil || first.Asset.ID != r.AssetID || first.Asset.Personal == nil || *first.Asset.Personal != r.Personal || first.Asset.ProjectID != uuid.Nil || first.Asset.Origin != domain.OriginSystem || !first.Asset.AIGCMarked || first.Asset.CanReference() || first.Asset.SourceOperationID != nil || first.Asset.UploadID != nil || first.Asset.DurationMS == nil || *first.Asset.DurationMS <= 0 || len(first.Renditions) != 1 || first.Renditions[0].Kind != domain.RenditionWaveform || len(objects.files) != 2 {
		t.Fatal("generated audio did not preserve closed personal media facts")
	}
	proof, err := json.Marshal(first.Asset.ModerationDetail)
	if err != nil || bytes.Contains(proof, []byte("local_workspace_owner_review")) || bytes.Contains(proof, []byte("local_review_confirmed")) || !bytes.Contains(proof, []byte(r.Provenance.ReceiptSHA256)) {
		t.Fatal("generated audio invented upload review or lost fixed receipt", err)
	}
	for key, data := range objects.files {
		if !strings.HasPrefix(key, "personal/"+r.Personal.OrgID.String()+"/"+r.Personal.ActorID.String()+"/audio/2026/10/"+r.AssetID.String()) {
			t.Fatal("unstable or unowned generated media key", key)
		}
		hash := sha256.Sum256(data)
		if key == first.Asset.ObjectKey && hex.EncodeToString(hash[:]) != r.SHA256 {
			t.Fatal("original differs from fixed complete provider receipt")
		}
	}
	replay, err := service.Prepare(t.Context(), r, file)
	if err != nil || !reflect.DeepEqual(first, replay) || len(objects.files) != 2 {
		t.Fatal("free ingest replay changed asset identity or output set", err)
	}
	if _, err := file.File.Stat(); err != nil {
		t.Fatal("prepare closed caller-owned verified file", err)
	}
}

func TestPersonalGeneratedAudioRejectsIncompleteOrChangedReceiptBeforeStorage(t *testing.T) {
	r, file := generatedAudioInput(t)
	for _, name := range []string{"terminal", "receipt", "request", "asset", "owner", "route", "work", "mime", "size", "hash", "name", "invalid_log"} {
		t.Run(name, func(t *testing.T) {
			bad := r
			switch name {
			case "terminal":
				bad.Provenance.TerminalCode = 0
			case "receipt":
				bad.Provenance.ReceiptSHA256 = ""
			case "request":
				bad.Provenance.ProviderRequestID = uuid.New()
			case "asset":
				bad.AssetID = uuid.Nil
			case "owner":
				bad.Personal.ActorID = uuid.Nil
			case "route":
				bad.Intent.ConfigRevision = ""
			case "work":
				bad.Intent.Owner.WorkID = uuid.New()
			case "mime":
				bad.MIMEType = "image/png"
			case "size":
				bad.ByteSize++
			case "hash":
				bad.SHA256 = strings.Repeat("b", 64)
			case "name":
				bad.FileName = "../sample.mp3"
			case "invalid_log":
				bad.Provenance.ProviderLogID = string([]byte{0xff})
			}
			objects := &generatedAudioObjects{}
			if _, err := generatedAudioService(objects).Prepare(t.Context(), bad, file); err == nil || len(objects.files) != 0 {
				t.Fatal("incomplete or changed receipt reached object storage", err)
			}
		})
	}
}

func TestPersonalGeneratedAudioRehashesBytesAndRejectsForeignPhysicalLocation(t *testing.T) {
	r, file := generatedAudioInput(t)
	objects := &generatedAudioObjects{wrong: true}
	if _, err := generatedAudioService(objects).Prepare(t.Context(), r, file); !errors.Is(err, mediaapp.ErrObjectMismatch) {
		t.Fatal("foreign fixed profile was accepted", err)
	}
	if err := file.File.Truncate(8); err != nil {
		t.Fatal(err)
	}
	objects = &generatedAudioObjects{}
	if _, err := generatedAudioService(objects).Prepare(t.Context(), r, file); err == nil || len(objects.files) != 0 {
		t.Fatal("changed underlying bytes were accepted from claimed metadata", err)
	}
}
