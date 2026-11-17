package aigateway_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	subject "github.com/StephenQiu30/glowloom-server/backend/creation/subject/application"
	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/application"
	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
	operation "github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/app/aigateway"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/media/adapter/workflow"
	media "github.com/StephenQiu30/glowloom-server/backend/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/media/domain"
)

type fixtureVoiceAccess struct{ owner subject.VoiceOwner }

func (f fixtureVoiceAccess) Request(context.Context, identity.Principal, bool) (subject.VoiceOwner, error) {
	return f.owner, nil
}
func (f fixtureVoiceAccess) Worker(context.Context, subject.VoiceWork) (subject.VoiceOwner, error) {
	return f.owner, nil
}
func actualMP3(t *testing.T) []byte {
	t.Helper()
	p := filepath.Join(t.TempDir(), "sample.mp3")
	out, err := exec.CommandContext(t.Context(), "ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "sine=frequency=440:duration=0.2", "-c:a", "libmp3lame", p).CombinedOutput()
	require.NoError(t, err, string(out))
	b, err := os.ReadFile(p)
	require.NoError(t, err)
	return b
}
func TestGatewayVoiceQueuePrivateRecoveryActualPersonalMedia(t *testing.T) {
	db, store, _, a := fixture(t)
	require.NoError(t, db.Exec(`UPDATE identity."user" SET email_key=?,email_verified_at=clock_timestamp() WHERE id=?`, a.ID.String()+"@example.invalid", a.ID).Error)
	m := mediaModel(t, store, "audio.generate", "provider_success_v1")
	registry := fixtureMediaStorage(t, db)
	body := actualMP3(t)
	var sends atomic.Int64
	r := &aigateway.Runtime{DB: db, Objects: registry, Provider: providerFunc(func(_ context.Context, d application.Dispatch, _ func(json.RawMessage) error) (domain.Receipt, error) {
		sends.Add(1)
		require.Equal(t, "subject.voice", d.Call.Origin)
		require.Equal(t, uuid.Nil, d.Call.Project)
		var in domain.MediaInput
		require.NoError(t, json.Unmarshal(d.Call.Raw, &in))
		require.Equal(t, "fixture-speaker", in.Params["speaker"])
		require.Equal(t, "hello audio", in.Prompt)
		result := operation.GenerationResult{Outcome: operation.ProviderOutcomeCompleted, State: operation.ProviderTaskSucceeded, ProviderLogID: "original-voice-log", Audio: []operation.GeneratedAudio{{Bytes: body, MediaType: "audio/mpeg"}}, Usage: json.RawMessage(`{"native_character_count":11}`)}
		raw, err := json.Marshal(result)
		require.NoError(t, err)
		cost, err := d.Route.Purchase.Cost(domain.Usage{})
		require.NoError(t, err)
		return domain.Receipt{Body: raw, UpstreamID: "original-voice-log", PurchaseMicros: cost}, nil
	})}
	voice := aigateway.NewVoiceJobs(r)
	voice.Authorize = fixtureAuthority
	voice.Access = func(*gorm.DB) subject.VoiceAccess {
		return fixtureVoiceAccess{owner: subject.VoiceOwner{ScopeID: a.ScopeID, PhysicalScopeID: a.OrgID, PersonalScopeID: a.ScopeID, PersonalOrgID: a.OrgID}}
	}
	options, err := voice.Options(t.Context(), a)
	require.NoError(t, err)
	require.True(t, options.Available)
	found := false
	for _, option := range options.Models {
		found = found || option.ReleaseID == m.ID
	}
	require.True(t, found)
	key := uuid.New()
	in := subject.CreateVoiceJobInput{DraftIntentID: uuid.New(), ReleaseID: m.ID, VoiceKey: "fixture-speaker", Name: "示例配音", Text: "hello audio"}
	job, err := voice.Create(t.Context(), a, key, in)
	require.NoError(t, err)
	require.Equal(t, "queued", job.Status)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			again, err := voice.Create(t.Context(), a, key, in)
			require.NoError(t, err)
			require.Equal(t, job.ID, again.ID)
		})
	}
	wg.Wait()
	var count int64
	require.NoError(t, db.Raw("SELECT count(*) FROM ai.hold WHERE request_id=?", job.ID).Scan(&count).Error)
	require.EqualValues(t, 1, count)
	require.Zero(t, sends.Load())
	bad := in
	bad.Name = "不同配音"
	_, err = voice.Create(t.Context(), a, key, bad)
	require.ErrorIs(t, err, subject.ErrVoiceJobKeyConflict)
	recovered, err := voice.Recover(t.Context(), a, key)
	require.NoError(t, err)
	require.Equal(t, "recorded", recovered.State)
	require.Equal(t, job.ID, recovered.Job.ID)
	_, err = voice.Get(t.Context(), identity.Principal{ID: uuid.New(), ScopeID: a.ScopeID, OrgID: a.OrgID}, job.ID)
	require.Error(t, err)
	missing, err := voice.Recover(t.Context(), a, uuid.New())
	require.NoError(t, err)
	require.Equal(t, "unrecorded", missing.State)
	creation := aigateway.Creation{Runtime: r, Authorize: fixtureAuthority}
	for range 2 {
		next, err := creation.Execute(t.Context(), taskruntime.Job{BusinessID: job.ID.String()})
		require.NoError(t, err)
		require.True(t, next.IsZero())
	}
	require.EqualValues(t, 1, sends.Load())
	objects := &durableBytes{t: t, db: db, routes: pgmedia.NewStorageRoutes(db), files: map[string][]byte{}, failOnce: true}
	r.VoiceMedia = media.NewPersonalGeneratedAudioService(mediaflow.FFProber{}, mediaflow.FFUploadRenderer{}, objects, time.Now)
	require.ErrorIs(t, r.Deliver(t.Context(), job.ID), media.ErrUnavailable)
	require.NoError(t, r.Deliver(t.Context(), job.ID))
	require.NoError(t, r.Deliver(t.Context(), job.ID))
	require.Equal(t, 2, objects.writes)
	require.EqualValues(t, 1, sends.Load())
	finished, err := voice.Get(t.Context(), a, job.ID)
	require.NoError(t, err)
	require.Equal(t, "succeeded", finished.Status)
	require.NotNil(t, finished.Result)
	require.Equal(t, "subject", finished.Result.LibrarySource)
	require.Equal(t, finished.Result.AssetID, finished.Result.LibraryItemID)
	var asset struct {
		PersonalOrgID, PersonalActorID    uuid.UUID
		SourceOperationID                 *uuid.UUID
		Status, Kind, MimeType, ObjectKey string
		ModerationDetail                  []byte
	}
	require.NoError(t, db.Raw("SELECT * FROM media.media_asset WHERE id=?", finished.Result.AssetID).Scan(&asset).Error)
	require.Equal(t, a.ID, asset.PersonalActorID)
	require.Equal(t, a.OrgID, asset.PersonalOrgID)
	require.Equal(t, "ready", asset.Status)
	require.Equal(t, "audio/mpeg", asset.MimeType)
	require.Nil(t, asset.SourceOperationID)
	require.Equal(t, body, objects.files[asset.ObjectKey])
	require.Contains(t, string(asset.ModerationDetail), "ai_gateway_provider_receipt")
	require.NotContains(t, string(asset.ModerationDetail), "20000000")
	require.NotContains(t, string(asset.ModerationDetail), "local_workspace_owner_review")
	bills, err := store.Bills(t.Context(), a.ScopeID)
	require.NoError(t, err)
	require.Len(t, bills, 1)
	require.EqualValues(t, 1, bills[0].Charged)
	require.NoError(t, db.Raw("SELECT count(*) FROM subject.voice_job WHERE id=?", job.ID).Scan(&count).Error)
	require.Zero(t, count)
}

func TestGatewayPersonalAudioProvenanceRejectsForgedLegacyTerminal(t *testing.T) {
	// The new provenance needs its own frozen provider policy. It cannot claim an
	// old 20000000 speech terminal or a fabricated model version for acceptance.
	b := actualMP3(t)
	file, err := media.ReadUpload(t.Context(), bytes.NewReader(b), "sample.mp3")
	require.NoError(t, err)
	defer func() { _ = file.Close() }()
	id, scope, model := uuid.New(), uuid.New(), uuid.New()
	req := media.PersonalGeneratedAudioRequest{AssetID: uuid.New(), OwnerScopeID: scope, SourceJobID: id, Personal: mediadomain.PersonalOwnership{OrgID: uuid.New(), ActorID: uuid.New()}, FileName: "智能配音.mp3", CreatedAt: time.Now(), SHA256: file.SHA256, ByteSize: file.Size, MIMEType: file.MIMEType, Intent: media.StorageWorkIntent{ID: uuid.New(), Owner: media.StorageWorkOwner{Kind: media.StorageWorkKindSubjectVoice, OwnerID: scope, WorkID: id}, ProfileID: "fixture", Bucket: "fixture-123", ConfigRevision: "1", State: media.StorageWorkOpen}, Provenance: media.PersonalGeneratedAudioProvenance{SchemaVersion: 2, ReceiptSHA256: domain.Digest([]byte("saved-receipt")), GatewayRequestID: id, OutputPolicy: "provider_success_v1", ReleaseID: model, ModelVersionID: model, ProviderRequestID: id}}
	require.NoError(t, req.Validate())
	req.Provenance.TerminalCode = 20000000
	require.ErrorIs(t, req.Validate(), media.ErrInvalidGeneratedAudio)
	req.Provenance.TerminalCode = 0
	req.Provenance.OutputPolicy = ""
	require.ErrorIs(t, req.Validate(), media.ErrInvalidGeneratedAudio)
}
