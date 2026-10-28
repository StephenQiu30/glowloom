package subject_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	subjectwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/subject"
	pgcatalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/postgres"
	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	catalogdomain "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/workflow"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	pgsubject "github.com/StephenQiu30/glowloom-server/backend/internal/subject/adapter/postgres"
	subject "github.com/StephenQiu30/glowloom-server/backend/internal/subject/application"
	"github.com/StephenQiu30/glowloom-server/backend/tests/identityfixture"
)

// The fixture release/credential is deliberately owner-seeded and its catalog
// port is fixture-proven. It is not evidence of a paid provider validation.
type voiceCatalogFixture struct{ release subject.VoiceRelease }

func (f voiceCatalogFixture) List(context.Context) ([]subject.VoiceRelease, error) {
	return []subject.VoiceRelease{f.release}, nil
}
func (f voiceCatalogFixture) Select(_ context.Context, id uuid.UUID, key string) (subject.VoiceRelease, error) {
	if id != f.release.ReleaseID || key != "fixture-speaker" {
		return subject.VoiceRelease{}, subject.ErrVoiceUnavailable
	}
	return f.release, nil
}
func (f voiceCatalogFixture) Credential(context.Context, subject.VoiceWork) (subject.VoiceCredential, error) {
	return subject.VoiceCredential{}, nil
}

type voiceStorageFixture struct {
	routes  *pgmedia.StorageRoutes
	profile string
	db      *gorm.DB
}

func (f voiceStorageFixture) Ready(context.Context) (bool, error) { return true, nil }

func (f voiceStorageFixture) Pin(ctx context.Context, scope, job uuid.UUID) (subject.VoiceStorageIntent, error) {
	id := uuid.New()
	err := f.routesFixtureInsert(ctx, id, scope, job)
	return subject.VoiceStorageIntent{ID: id, ProfileID: f.profile, Bucket: f.profile, ConfigRevision: "frozen-test", State: "open"}, err
}
func (f voiceStorageFixture) routesFixtureInsert(ctx context.Context, id, scope, job uuid.UUID) error {
	return f.db.WithContext(ctx).Exec(`INSERT INTO media.storage_work_intent(id,kind,owner_id,work_id,profile_id,bucket,config_revision) VALUES(?,'subject-voice',?,?,?,?,'frozen-test')`, id, scope, job, f.profile, f.profile).Error
}
func (f voiceStorageFixture) Read(ctx context.Context, scope, job uuid.UUID) (subject.VoiceStorageIntent, error) {
	r, err := f.routes.ReadStorageWork(ctx, media.StorageWorkOwner{Kind: "subject-voice", OwnerID: scope, WorkID: job})
	return subject.VoiceStorageIntent{ID: r.ID, ProfileID: r.ProfileID, Bucket: r.Bucket, ConfigRevision: r.ConfigRevision, State: r.State}, err
}

type voicePGFixture struct {
	store          *pgsubject.VoiceStore
	runtime, owner *gorm.DB
	actor          identity.Principal
	input          subject.CreateVoiceJobInput
	audio          []byte
	sends          int
	failImport     bool
	unknown        bool
}

func newVoicePG(t *testing.T) *voicePGFixture {
	t.Helper()
	runtime, owner := subjectPostgres(t)
	actor := subjectSession(t, owner)
	project := subjectProject(t, owner, actor)
	version, releaseID := seedActualSubjectCatalog(t, owner, project)
	var ids struct{ ProviderID, ConfigVersionID, CredentialID, PriceVersionID uuid.UUID }
	if err := owner.Raw(`SELECT r.config_version_id,r.price_version_id,m.provider_id,c.id AS credential_id FROM catalog.model_release r JOIN catalog.model_profile m ON m.id=r.model_id JOIN catalog.provider_credential c ON c.provider_id=m.provider_id WHERE r.id=?`, releaseID).Scan(&ids).Error; err != nil {
		t.Fatal(err)
	}
	release := subject.VoiceRelease{VoiceModelOption: subject.VoiceModelOption{ReleaseID: releaseID, ModelVersionID: version, ModelKey: "fixture", DisplayName: "Fixture only", Voices: []subject.VoiceOption{{Key: "fixture-speaker", Name: "Fixture"}}, DefaultVoiceKey: "fixture-speaker"}, ProviderID: ids.ProviderID, ConfigVersionID: ids.ConfigVersionID, CredentialID: ids.CredentialID, PriceVersionID: ids.PriceVersionID, ResourceID: "seed-tts-2.0", Moderation: "provider", UpstreamCost: json.RawMessage(`{"currency":"CNY","max_micros":1,"source":"fixture only","reviewed_at":"2026-10-28T00:00:00Z"}`)}
	profile := "voice-pg-" + uuid.NewString()
	if err := pgmedia.NewStorageRoutes(owner).RegisterProfiles(t.Context(), []media.StorageProfile{{ID: profile, Provider: "minio", Endpoint: "http://127.0.0.1:1", SigningEndpoint: "http://127.0.0.1:1", Region: "us-east-1", Bucket: profile, AddressingStyle: "path"}}); err != nil {
		t.Fatal(err)
	}
	store := pgsubject.NewVoiceStore(runtime, pgsubject.VoiceFactories{Access: subjectwire.NewVoiceAccess, Catalog: func(*gorm.DB) subject.VoiceCatalog { return voiceCatalogFixture{release} }, Storage: func(tx *gorm.DB) subject.VoiceStorage {
		return voiceStorageFixture{routes: pgmedia.NewStorageRoutes(tx), profile: profile, db: tx}
	}})
	name := filepath.Join(t.TempDir(), "voice.mp3")
	if out, err := exec.CommandContext(t.Context(), "ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "sine=frequency=550:duration=0.2", "-c:a", "libmp3lame", name).CombinedOutput(); err != nil {
		t.Fatalf("actual audio fixture: %v %s", err, out)
	}
	audio, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return &voicePGFixture{store: store, runtime: runtime, owner: owner, actor: actor, input: subject.CreateVoiceJobInput{DraftIntentID: uuid.New(), ReleaseID: releaseID, VoiceKey: "fixture-speaker", Name: "智能配音样本", Text: "原始字节来自真实MP3测试"}, audio: audio}
}
func (f *voicePGFixture) Synthesize(context.Context, subject.VoiceSynthesisInput, string) (subject.VoiceSynthesisResult, error) {
	f.sends++
	if f.unknown {
		return subject.VoiceSynthesisResult{}, subject.ErrVoiceExecutionUnknown
	}
	return subject.VoiceSynthesisResult{Audio: bytes.Clone(f.audio), MIMEType: "audio/mpeg", TerminalCode: 20000000, Usage: json.RawMessage(`{"text_words":12}`)}, nil
}
func (f *voicePGFixture) Read(_ context.Context, w subject.VoiceWork) (subject.VoiceReceipt, bool, error) {
	if w.Receipt == nil {
		return subject.VoiceReceipt{}, false, nil
	}
	return *w.Receipt, true, nil
}
func (f *voicePGFixture) Save(ctx context.Context, w subject.VoiceWork, r subject.VoiceSynthesisResult) (subject.VoiceReceipt, error) {
	file, err := media.ReadUpload(ctx, bytes.NewReader(r.Audio), "voice.mp3")
	if err != nil {
		return subject.VoiceReceipt{}, err
	}
	defer func() { _ = file.Close() }()
	intent := media.StorageWorkIntent{ID: w.Storage.ID, Owner: media.StorageWorkOwner{Kind: "subject-voice", OwnerID: w.Owner.ScopeID, WorkID: w.Job.ID}, ProfileID: w.Storage.ProfileID, Bucket: w.Storage.Bucket, ConfigRevision: w.Storage.ConfigRevision, State: w.Storage.State}
	location, err := (voiceLedgerObjects{pgmedia.NewStorageRoutes(f.runtime)}).CreateGeneratedAudioObject(ctx, intent, subject.VoiceAudioKey(w), file)
	if err != nil {
		return subject.VoiceReceipt{}, err
	}
	sum := sha256.Sum256(r.Audio)
	return subject.VoiceReceipt{SchemaVersion: 1, JobID: w.Job.ID, ActorID: w.Job.CurrentActorID, OwnerScopeID: w.Owner.ScopeID, PersonalOrgID: w.Owner.PersonalOrgID, AssetID: w.AssetID, ReleaseID: w.Job.ReleaseID, ModelVersionID: w.Job.ModelVersionID, ProviderRequestID: w.Job.ID, RequestSHA256: w.RequestSHA256, AudioLocationID: location.ID, AudioKey: subject.VoiceAudioKey(w), AudioSHA256: hex.EncodeToString(sum[:]), AudioByteSize: int64(len(r.Audio)), MIMEType: r.MIMEType, TerminalCode: r.TerminalCode, Usage: r.Usage}, nil
}
func (f *voicePGFixture) Publish(ctx context.Context, w subject.VoiceWork, r subject.VoiceReceipt) (subject.VoiceJob, error) {
	if f.failImport {
		return w.Job, errors.New("temporary ingest fixture failure")
	}
	file, err := media.ReadUpload(ctx, bytes.NewReader(f.audio), "voice.mp3")
	if err != nil {
		return w.Job, err
	}
	defer func() { _ = file.Close() }()
	request := media.PersonalGeneratedAudioRequest{AssetID: w.AssetID, OwnerScopeID: w.Owner.ScopeID, SourceJobID: w.Job.ID, Personal: mediadomain.PersonalOwnership{OrgID: w.Owner.PersonalOrgID, ActorID: w.Job.CurrentActorID}, FileName: w.Job.Name + ".mp3", CreatedAt: w.Job.CreatedAt, SHA256: r.AudioSHA256, ByteSize: r.AudioByteSize, MIMEType: r.MIMEType, Intent: media.StorageWorkIntent{ID: w.Storage.ID, Owner: media.StorageWorkOwner{Kind: "subject-voice", OwnerID: w.Owner.ScopeID, WorkID: w.Job.ID}, ProfileID: w.Storage.ProfileID, Bucket: w.Storage.Bucket, ConfigRevision: w.Storage.ConfigRevision, State: w.Storage.State}, Provenance: media.PersonalGeneratedAudioProvenance{SchemaVersion: 1, ReceiptSHA256: r.Digest(), ReleaseID: w.Job.ReleaseID, ModelVersionID: w.Job.ModelVersionID, ProviderRequestID: w.Job.ID, TerminalCode: 20000000}}
	prepared, err := media.NewPersonalGeneratedAudioService(mediaflow.FFProber{}, mediaflow.FFUploadRenderer{}, voiceLedgerObjects{pgmedia.NewStorageRoutes(f.runtime)}, time.Now).Prepare(ctx, request, file)
	if err != nil {
		return w.Job, err
	}
	return f.store.CompleteWith(ctx, w, r, func(tx *gorm.DB) (subject.VoiceJobResult, error) {
		summary, err := pgmedia.NewScopedStore(tx, app.NewMediaWorkerScopeAccess).AcceptPersonalGeneratedAudio(ctx, identity.Principal{ID: w.Job.CurrentActorID, OrgID: w.Owner.PersonalOrgID, ScopeID: w.Owner.PersonalScopeID}, prepared)
		if err != nil {
			return subject.VoiceJobResult{}, err
		}
		return subject.VoiceJobResult{AssetID: summary.ID, AssetRevision: summary.Revision, Kind: summary.Kind, MIMEType: summary.MIMEType, FileName: summary.FileName, LibraryItemID: summary.ID, LibrarySource: "subject"}, nil
	})
}
func TestVoicePostgresPermanentIntentRecoveryAndAtomicRealAudio(t *testing.T) {
	f := newVoicePG(t)
	key := uuid.New()
	job, err := f.store.Create(t.Context(), f.actor, key, f.input)
	if err != nil {
		t.Fatal("admit voice", err)
	}
	replayed, err := f.store.Create(t.Context(), f.actor, key, f.input)
	if err != nil || !reflect.DeepEqual(job, replayed) {
		t.Fatal("permanent admission changed", err)
	}
	changed := f.input
	changed.DraftIntentID = uuid.New()
	if _, err := f.store.Create(t.Context(), f.actor, key, changed); !errors.Is(err, subject.ErrVoiceJobKeyConflict) {
		t.Fatal("different draft reused key", err)
	}
	work, err := f.store.Work(t.Context(), job.ID)
	if err != nil {
		t.Fatal(err)
	}
	d := subject.VoiceDelivery{EventID: work.EventID, JobID: job.ID, ActorID: f.actor.ID, ScopeID: f.actor.ScopeID}
	if valid, err := f.store.VerifyDelivery(t.Context(), d); err != nil || !valid {
		t.Fatal("real outbox proof", err)
	}
	d.RecoveryOnly = true
	if _, err := f.store.VerifyDelivery(t.Context(), d); err == nil {
		t.Fatal("tampered recovery proof accepted")
	}
	worker := subject.NewVoiceWorker(f.store, voiceTestKeys{}, f, f, f)
	f.failImport = true
	if _, err := worker.Run(t.Context(), subject.VoiceWorkID{JobID: job.ID}); err == nil {
		t.Fatal("expected temporary import failure")
	}
	stored, err := f.store.Get(t.Context(), f.actor, job.ID)
	if err != nil || stored.Status != "ingesting" || f.sends != 1 {
		t.Fatal("receipt not durable", stored.Status, err)
	}
	var count int64
	if err := f.owner.Raw(`SELECT count(*) FROM media.media_asset WHERE id=?`, work.AssetID).Scan(&count).Error; err != nil || count != 0 {
		t.Fatal("failed import published media", err)
	}
	if _, err := f.store.Create(t.Context(), f.actor, key, f.input); err != nil {
		t.Fatal("same-key recovery admission", err)
	}
	f.failImport = false
	done, err := worker.Run(t.Context(), subject.VoiceWorkID{JobID: job.ID, RecoveryOnly: true})
	if err != nil || done.Status != "succeeded" || done.Result == nil || done.Result.AssetID != work.AssetID || f.sends != 1 {
		t.Fatal("durable local recovery", done.Status, err)
	}
	if err := f.owner.Raw(`SELECT count(*) FROM media.media_asset WHERE id=? AND project_id IS NULL AND personal_actor_id=? AND sha256=? AND origin='system' AND aigc_marked`, work.AssetID, f.actor.ID, hexHash(f.audio)).Scan(&count).Error; err != nil || count != 1 {
		t.Fatal("real personal generated audio missing", err)
	}
	if _, err := worker.Run(t.Context(), subject.VoiceWorkID{JobID: job.ID}); err != nil || f.sends != 1 {
		t.Fatal("success replay submitted", err)
	}
	if err := f.owner.Raw(`SELECT count(*) FROM audit.audit_log WHERE object_type='subject_voice_job' AND object_id=? AND action IN ('subject.voice_created','subject.voice_dispatch_started','subject.voice_receipt_recorded','subject.voice_succeeded')`, job.ID.String()).Scan(&count).Error; err != nil || count != 4 {
		t.Fatal("durable voice lifecycle audit missing or duplicated", count, err)
	}
	other := subjectSession(t, f.owner)
	if _, err := f.store.Get(t.Context(), other, job.ID); err == nil {
		t.Fatal("foreign actor read job")
	}
	if err := f.runtime.Exec(`UPDATE subject.voice_job SET text='mutated' WHERE id=?`, job.ID).Error; err == nil {
		t.Fatal("runtime mutated frozen paid request")
	}
}
func hexHash(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
func TestVoicePostgresUnknownNeverReopensDispatch(t *testing.T) {
	f := newVoicePG(t)
	f.unknown = true
	key := uuid.New()
	job, err := f.store.Create(t.Context(), f.actor, key, f.input)
	if err != nil {
		t.Fatal(err)
	}
	worker := subject.NewVoiceWorker(f.store, voiceTestKeys{}, f, f, f)
	got, err := worker.Run(t.Context(), subject.VoiceWorkID{JobID: job.ID})
	if err != nil || got.Status != "unknown" {
		t.Fatal("unknown result", err)
	}
	for range 2 {
		if _, err := f.store.Create(t.Context(), f.actor, key, f.input); err != nil {
			t.Fatal(err)
		}
		if _, err := worker.Run(t.Context(), subject.VoiceWorkID{JobID: job.ID, RecoveryOnly: true}); err != nil {
			t.Fatal(err)
		}
	}
	if f.sends != 1 {
		t.Fatal("unknown paid send repeated")
	}
	work, claimed, err := f.store.Claim(t.Context(), job.ID, uuid.New())
	if err != nil || claimed || work.Job.Status != "unknown" {
		t.Fatal("permanent claim reopened", err)
	}
	if err := f.owner.Exec(`UPDATE catalog.provider SET concurrency_limit=1 WHERE id=?`, work.ProviderID).Error; err != nil {
		t.Fatal(err)
	}
	another := f.input
	another.DraftIntentID = uuid.New()
	next, err := f.store.Create(t.Context(), f.actor, uuid.New(), another)
	if err != nil {
		t.Fatal(err)
	}
	if _, claimed, err := f.store.Claim(t.Context(), next.ID, uuid.New()); !errors.Is(err, subject.ErrVoiceUnavailable) || claimed {
		t.Fatal("unknown supplier execution must retain conservative provider capacity", err)
	}
}

// A real release can be drafted for the speech protocol, while unverified
// connections remain unavailable. No validation result is fabricated here.
func TestSubjectSpeechReleaseDraftUsesExistingPublicationGate(t *testing.T) {
	runtime, owner := subjectPostgres(t)
	actor := subjectSession(t, owner)
	if err := owner.Exec(`UPDATE identity."user" SET platform_role='platform_admin' WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	actor = identityfixture.Session(t, owner, actor)
	provider, model, version, price, config := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	if err := owner.Exec(`INSERT INTO catalog.capability(id,key,output_type,modes,input_roles) VALUES(?,'audio.generate','audio',ARRAY['text2audio'],ARRAY['prompt']) ON CONFLICT(key) DO NOTHING`, uuid.New()).Error; err != nil {
		t.Fatal(err)
	}
	preset, presetVersion := uuid.New(), uuid.New()
	var speechConfig catalogdomain.ProviderConfig
	for _, p := range catalogdomain.BuiltinPresets() {
		if p.Slug == "volcengine-speech" {
			speechConfig = p.Config
		}
	}
	payload, err := json.Marshal(map[string]any{"schema_version": 1, "name": "Speech fixture", "config": speechConfig})
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Exec(`INSERT INTO catalog.provider_preset(id,slug,origin) VALUES(?,?,'custom')`, preset, "voice-fixture-"+preset.String()).Error; err != nil {
		t.Fatal(err)
	}
	if err := owner.Exec(`INSERT INTO catalog.provider_preset_version(id,preset_id,version_no,payload,content_hash) VALUES(?,?,1,?::jsonb,?)`, presetVersion, preset, string(payload), hexHash(payload)).Error; err != nil {
		t.Fatal(err)
	}
	queries := []struct {
		q string
		a []any
	}{{`INSERT INTO catalog.provider(id,key,name,adapter_key,region,status) VALUES(?,?,'Speech fixture','volcengine_speech','domestic','disabled')`, []any{provider, "speech-" + provider.String()}}, {`INSERT INTO catalog.model_profile(id,model_key,provider_id,capability,display_name,status) VALUES(?,?,?,'audio.generate','Speech fixture','disabled')`, []any{model, "speech-" + model.String(), provider}}, {`INSERT INTO catalog.model_profile_version(id,model_profile_id,version_no,provider_model_id,modes,limits,param_schema,moderation,queue,supports_query,supports_cancel,supports_callback,expected_max_ms) VALUES(?,?,1,'seed-tts-2.0',ARRAY['text2audio'],'{"max_outputs":1}','[{"field":"speaker","type":"string","component":"input","default":"zh_female_vv_uranus_bigtts"}]','provider','provider',false,false,false,30000)`, []any{version, model}}, {`INSERT INTO catalog.price_rule_version(id,model_profile_id,version_no,unit,rule,effective_from) VALUES(?,?,1,'per_1k_chars','{"base_micros":1}',now())`, []any{price, model}}, {`INSERT INTO catalog.provider_config_version(id,provider_id,preset_version_id,version_no,config,content_hash) SELECT ?,?,id,1,payload->'config',content_hash FROM catalog.provider_preset_version WHERE id=?`, []any{config, provider, presetVersion}}}
	for _, q := range queries {
		r := owner.Exec(q.q, q.a...)
		if r.Error != nil || r.RowsAffected != 1 {
			t.Fatal("speech release fixture", r.Error)
		}
	}
	release, err := app.NewCatalogStore(runtime).CreateModelRelease(t.Context(), actor, model, catalog.CreateReleaseInput{ModelVersionID: version, PriceVersionID: price, ConfigVersionID: config, RequestID: uuid.New(), UpstreamCost: catalog.UpstreamCost{Currency: "CNY", MaxMicros: 100, Source: "official fixture", ReviewedAt: time.Now().UTC()}})
	if err != nil || release.Status != "draft" {
		t.Fatal("speech draft release denied", err)
	}
	options, err := pgcatalog.NewSubjectVoiceCatalog(runtime).List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range options {
		if o.ReleaseID == release.ID {
			t.Fatal("unverified speech release executable")
		}
	}
}

func TestVoicePostgresTeamTargetAndLiveRevocation(t *testing.T) {
	f := newVoicePG(t)
	peer := subjectSession(t, f.owner)
	_, team := subjectTeam(t, f.owner, f.actor, peer)
	key := uuid.New()
	job, err := f.store.Create(t.Context(), team, key, f.input)
	if err != nil {
		t.Fatal("team admission must independently derive creator personal output", err)
	}
	work, err := f.store.Work(t.Context(), job.ID)
	if err != nil || work.Owner.ScopeID != team.ScopeID || work.Owner.PersonalScopeID != peer.ScopeID || work.Owner.PersonalOrgID != peer.OrgID {
		t.Fatal("target and output ownership were conflated", err)
	}
	if err := f.owner.Exec(`UPDATE workspace.team_member SET status='removed' WHERE team_id=(SELECT team_id FROM workspace.creation_scope WHERE id=?) AND user_id=?`, team.ScopeID, team.ID).Error; err != nil {
		t.Fatal(err)
	}
	worker := subject.NewVoiceWorker(f.store, voiceTestKeys{}, f, f, f)
	if _, err := worker.Run(t.Context(), subject.VoiceWorkID{JobID: job.ID}); err == nil || f.sends != 0 {
		t.Fatal("revoked target submitted paid voice", err)
	}
	if _, err := f.store.Create(t.Context(), team, key, f.input); err == nil {
		t.Fatal("permanent key bypassed live target revocation")
	}
}
