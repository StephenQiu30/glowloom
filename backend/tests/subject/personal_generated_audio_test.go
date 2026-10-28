package subject_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/workflow"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// This SQL fixture uses the real work ledger and real FF tools; the storage
// transport is bounded memory. Official SDK I/O is tested separately in media.
type voiceLedgerObjects struct{ routes *pgmedia.StorageRoutes }

func (o voiceLedgerObjects) CreateGeneratedAudioObject(ctx context.Context, intent media.StorageWorkIntent, key string, file *media.Downloaded) (mediadomain.ObjectLocation, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	l, err := o.routes.ReserveWorkLocation(ctx, intent.ID, key)
	if err != nil {
		return l, err
	}
	if err := o.routes.BeginWorkWrite(ctx, intent.ID, l); err != nil {
		return l, err
	}
	if _, err := file.File.Seek(0, io.SeekStart); err != nil {
		return l, err
	}
	data, err := io.ReadAll(file.File)
	if err != nil || int64(len(data)) != file.Size {
		return l, errors.Join(media.ErrObjectMismatch, err)
	}
	if err := o.routes.ConfirmWorkLocation(ctx, intent.ID, l, media.ObjectInfo{Size: file.Size, ContentType: file.MIMEType, SHA256: file.SHA256}); err != nil {
		return l, err
	}
	return o.routes.ReadLocation(ctx, l.ID)
}

func preparedVoiceFixture(t *testing.T, runtime, owner *gorm.DB, actor identity.Principal) media.PreparedPersonalGeneratedAudio {
	t.Helper()
	name := filepath.Join(t.TempDir(), "voice.mp3")
	if out, err := exec.CommandContext(t.Context(), "ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "sine=frequency=440:duration=0.2", "-c:a", "libmp3lame", name).CombinedOutput(); err != nil {
		t.Fatalf("make actual voice fixture: %v: %s", err, out)
	}
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	file, err := media.ReadUpload(t.Context(), bytes.NewReader(data), "voice.mp3")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	jobID, profileID := uuid.New(), "voice-"+uuid.NewString()
	r := media.PersonalGeneratedAudioRequest{AssetID: uuid.New(), OwnerScopeID: actor.ScopeID, SourceJobID: jobID,
		Personal: mediadomain.PersonalOwnership{OrgID: actor.OrgID, ActorID: actor.ID}, FileName: "智能配音1.mp3", CreatedAt: time.Now().UTC().Truncate(time.Microsecond),
		SHA256: file.SHA256, ByteSize: file.Size, MIMEType: "audio/mpeg",
		Intent:     media.StorageWorkIntent{ID: uuid.New(), Owner: media.StorageWorkOwner{Kind: "subject-voice", OwnerID: actor.ScopeID, WorkID: jobID}, ProfileID: profileID, Bucket: profileID, ConfigRevision: "frozen-voice-test", State: "open"},
		Provenance: media.PersonalGeneratedAudioProvenance{SchemaVersion: 1, ReceiptSHA256: strings.Repeat("a", 64), ReleaseID: uuid.New(), ModelVersionID: uuid.New(), ProviderRequestID: jobID, TerminalCode: 20000000}}
	routes := pgmedia.NewStorageRoutes(runtime)
	if err := pgmedia.NewStorageRoutes(owner).RegisterProfiles(t.Context(), []media.StorageProfile{{ID: profileID, Provider: "minio", Endpoint: "http://127.0.0.1:1", SigningEndpoint: "http://127.0.0.1:1", Region: "us-east-1", Bucket: r.Intent.Bucket, AddressingStyle: "path"}}); err != nil {
		t.Fatal(err)
	}
	if err := owner.Exec(`INSERT INTO media.storage_work_intent(id,kind,owner_id,work_id,profile_id,bucket,config_revision) VALUES(?,'subject-voice',?,?,?,?,?)`, r.Intent.ID, actor.ScopeID, jobID, profileID, r.Intent.Bucket, r.Intent.ConfigRevision).Error; err != nil {
		t.Fatal(err)
	}
	prepared, err := media.NewPersonalGeneratedAudioService(mediaflow.FFProber{}, mediaflow.FFUploadRenderer{}, voiceLedgerObjects{routes}, time.Now).Prepare(t.Context(), r, file)
	if err != nil {
		t.Fatal(err)
	}
	return prepared
}

func TestPersonalGeneratedAudioAcceptJoinsCallerTransactionAndStableReceipt(t *testing.T) {
	runtime, owner := subjectPostgres(t)
	actor := subjectSession(t, owner)
	prepared := preparedVoiceFixture(t, runtime, owner, actor)
	if _, err := pgmedia.NewScopedStore(runtime, app.NewMediaWorkerScopeAccess).AcceptPersonalGeneratedAudio(t.Context(), actor, prepared); !errors.Is(err, media.ErrUnavailable) {
		t.Fatal("accepted generated media without the business transaction", err)
	}
	rollback := errors.New("caller job completion rolled back")
	accept := func(tx *gorm.DB) error {
		got, err := pgmedia.NewScopedStore(tx, app.NewMediaWorkerScopeAccess).AcceptPersonalGeneratedAudio(t.Context(), actor, prepared)
		if err != nil {
			return err
		}
		if got.ID != prepared.Request.AssetID || got.Revision != 1 || got.Kind != "audio" {
			return media.ErrObjectMismatch
		}
		return nil
	}
	if err := runtime.Transaction(func(tx *gorm.DB) error {
		if err := accept(tx); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatal("caller rollback was not preserved", err)
	}
	var count int64
	var state string
	if err := owner.Raw(`SELECT count(*) FROM media.media_asset WHERE id=?`, prepared.Request.AssetID).Scan(&count).Error; err != nil || count != 0 {
		t.Fatal("media publication escaped the caller transaction", count, err)
	}
	if err := owner.Raw(`SELECT state FROM media.storage_work_intent WHERE id=?`, prepared.Request.Intent.ID).Scan(&state).Error; err != nil || state != "open" {
		t.Fatal("object acceptance escaped the caller transaction", state, err)
	}
	if err := owner.Exec(`UPDATE media.object_location SET state='accepted' WHERE id=?`, prepared.Asset.ObjectLocation.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := runtime.Transaction(accept); err == nil {
		t.Fatal("open intent accepted a partially published object set")
	}
	if err := owner.Exec(`UPDATE media.object_location SET state='present' WHERE id=?`, prepared.Asset.ObjectLocation.ID).Error; err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := runtime.Transaction(accept); err != nil {
			t.Fatal("accept or permanent replay failed", err)
		}
	}
	if err := owner.Raw(`SELECT count(*) FROM media.media_asset WHERE id=? AND project_id IS NULL AND origin='system' AND aigc_marked AND status='ready' AND moderation_status='passed' AND source_operation_id IS NULL AND upload_id IS NULL AND moderation_detail->>'method'='subject_voice_provider_receipt'`, prepared.Request.AssetID).Scan(&count).Error; err != nil || count != 1 {
		t.Fatal("generated audio became an upload or fake project operation", count, err)
	}
	if err := owner.Raw(`SELECT count(*) FROM media.object_location WHERE owner_intent_id=? AND state='accepted'`, prepared.Request.Intent.ID).Scan(&count).Error; err != nil || count != 2 {
		t.Fatal("exact original and waveform were not accepted together", count, err)
	}
	bad := prepared
	bad.Request.Provenance.ReceiptSHA256 = strings.Repeat("b", 64)
	if err := runtime.Transaction(func(tx *gorm.DB) error {
		_, err := pgmedia.NewScopedStore(tx, app.NewMediaWorkerScopeAccess).AcceptPersonalGeneratedAudio(t.Context(), actor, bad)
		return err
	}); err == nil {
		t.Fatal("changed fixed receipt replay was accepted")
	}
}
