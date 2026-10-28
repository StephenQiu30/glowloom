package subject

import (
	"bytes"
	"context"
	"io"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	scopewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/scope"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	mediaobjects "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/objectstorage"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	platform "github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
	pgsubject "github.com/StephenQiu30/glowloom-server/backend/internal/subject/adapter/postgres"
	subject "github.com/StephenQiu30/glowloom-server/backend/internal/subject/application"
)

// NewVoiceExecution composes actual staged bytes, existing sealed credentials and
// same-transaction personal media acceptance. It makes no supplier calls itself.
func NewVoiceExecution(db *gorm.DB, registry *platform.Registry, prober media.Prober, renderer media.Renderer, keys subject.VoiceUnsealer, speech subject.VoiceSynthesizer) *subject.VoiceWorker {
	if db == nil || registry == nil || prober == nil || renderer == nil || keys == nil || speech == nil {
		return nil
	}
	store := NewVoiceJobs(db)
	objects := mediaobjects.NewPersonalGeneratedAudioObjects(registry, pgmedia.NewStorageRoutes(db))
	bridge := &voiceOutputs{store: store, registry: registry, objects: objects, prepare: media.NewPersonalGeneratedAudioService(prober, renderer, objects, time.Now)}
	return subject.NewVoiceWorker(store, keys, speech, bridge, bridge)
}

type voiceOutputs struct {
	store    *pgsubject.VoiceStore
	registry *platform.Registry
	objects  media.PersonalGeneratedAudioObjects
	prepare  *media.PersonalGeneratedAudioService
}

func voiceMediaIntent(work subject.VoiceWork) media.StorageWorkIntent {
	return media.StorageWorkIntent{ID: work.Storage.ID, Owner: media.StorageWorkOwner{Kind: "subject-voice", OwnerID: work.Owner.ScopeID, WorkID: work.Job.ID}, ProfileID: work.Storage.ProfileID, Bucket: work.Storage.Bucket, ConfigRevision: work.Storage.ConfigRevision, State: work.Storage.State}
}
func (o *voiceOutputs) PrepareSubmission(ctx context.Context, work subject.VoiceWork) error {
	if o == nil || o.registry == nil || o.prepare == nil {
		return subject.ErrVoiceUnavailable
	}
	return o.registry.CheckWrite(ctx, platform.Ref{ProfileID: work.Storage.ProfileID, Bucket: work.Storage.Bucket, Key: subject.VoiceAudioKey(work)})
}
func (o *voiceOutputs) Save(ctx context.Context, work subject.VoiceWork, result subject.VoiceSynthesisResult) (subject.VoiceReceipt, error) {
	if result.TerminalCode != 20000000 || result.MIMEType != "audio/mpeg" || len(result.Audio) < 1 || len(result.Audio) > 8<<20 {
		return subject.VoiceReceipt{}, subject.ErrVoiceExecutionUnknown
	}
	file, err := media.ReadUpload(ctx, bytes.NewReader(result.Audio), "voice.mp3")
	if err != nil {
		return subject.VoiceReceipt{}, err
	}
	defer func() { _ = file.Close() }()
	if file.MIMEType != "audio/mpeg" {
		return subject.VoiceReceipt{}, subject.ErrVoiceExecutionUnknown
	}
	location, err := o.objects.CreateGeneratedAudioObject(ctx, voiceMediaIntent(work), subject.VoiceAudioKey(work), file)
	if err != nil {
		return subject.VoiceReceipt{}, err
	}
	receipt := subject.VoiceReceipt{SchemaVersion: 1, JobID: work.Job.ID, ActorID: work.Job.CurrentActorID, OwnerScopeID: work.Owner.ScopeID, PersonalOrgID: work.Owner.PersonalOrgID, AssetID: work.AssetID, ReleaseID: work.Job.ReleaseID, ModelVersionID: work.Job.ModelVersionID, ProviderRequestID: work.Job.ID, RequestSHA256: work.RequestSHA256, ProviderLogID: result.ProviderLogID, AudioLocationID: location.ID, AudioKey: subject.VoiceAudioKey(work), AudioSHA256: file.SHA256, AudioByteSize: file.Size, MIMEType: "audio/mpeg", TerminalCode: result.TerminalCode, Usage: result.Usage}
	if len(receipt.Usage) == 0 {
		receipt.Usage = []byte(`{}`)
	}
	return receipt, receipt.Validate(work)
}

// Read trusts only the committed complete receipt, never mere object existence.
func (o *voiceOutputs) Read(_ context.Context, work subject.VoiceWork) (subject.VoiceReceipt, bool, error) {
	if work.Receipt == nil {
		return subject.VoiceReceipt{}, false, nil
	}
	if err := work.Receipt.Validate(work); err != nil {
		return subject.VoiceReceipt{}, false, err
	}
	return *work.Receipt, true, nil
}
func (o *voiceOutputs) Publish(ctx context.Context, work subject.VoiceWork, receipt subject.VoiceReceipt) (subject.VoiceJob, error) {
	if receipt.Validate(work) != nil || o.registry == nil || o.prepare == nil {
		return work.Job, subject.ErrVoiceExecutionUnknown
	}
	source, err := o.registry.Get(ctx, platform.Ref{ProfileID: work.Storage.ProfileID, Bucket: work.Storage.Bucket, Key: receipt.AudioKey})
	if err != nil {
		return work.Job, err
	}
	defer func() { _ = source.Close() }()
	file, err := media.ReadUpload(ctx, io.LimitReader(source, receipt.AudioByteSize+1), "voice.mp3")
	if err != nil {
		return work.Job, err
	}
	defer func() { _ = file.Close() }()
	if file.Size != receipt.AudioByteSize || file.SHA256 != receipt.AudioSHA256 || file.MIMEType != receipt.MIMEType {
		return work.Job, subject.ErrVoiceExecutionUnknown
	}
	req := media.PersonalGeneratedAudioRequest{AssetID: work.AssetID, OwnerScopeID: work.Owner.ScopeID, SourceJobID: work.Job.ID, Personal: mediadomain.PersonalOwnership{OrgID: work.Owner.PersonalOrgID, ActorID: work.Job.CurrentActorID}, FileName: "智能配音.mp3", CreatedAt: work.Job.CreatedAt, SHA256: receipt.AudioSHA256, ByteSize: receipt.AudioByteSize, MIMEType: receipt.MIMEType, Intent: voiceMediaIntent(work), Provenance: media.PersonalGeneratedAudioProvenance{SchemaVersion: 1, ReceiptSHA256: receipt.Digest(), ReleaseID: work.Job.ReleaseID, ModelVersionID: work.Job.ModelVersionID, ProviderRequestID: work.Job.ID, ProviderLogID: receipt.ProviderLogID, TerminalCode: receipt.TerminalCode}}
	prepared, err := o.prepare.Prepare(ctx, req, file)
	if err != nil {
		return work.Job, err
	}
	return o.store.CompleteWith(ctx, work, receipt, func(tx *gorm.DB) (subject.VoiceJobResult, error) {
		actor := identity.Principal{ID: work.Job.CurrentActorID, ScopeID: work.Owner.PersonalScopeID, OrgID: work.Owner.PersonalOrgID}
		summary, err := pgmedia.NewScopedStore(tx, func(tx *gorm.DB) media.ScopeAccess { return voicePersonalAuthority{tx: tx} }).AcceptPersonalGeneratedAudio(ctx, actor, prepared)
		if err != nil {
			return subject.VoiceJobResult{}, err
		}
		return subject.VoiceJobResult{AssetID: summary.ID, AssetRevision: summary.Revision, Kind: "audio", MIMEType: summary.MIMEType, FileName: summary.FileName, LibraryItemID: summary.ID, LibrarySource: "subject"}, nil
	})
}

type voicePersonalAuthority struct{ tx *gorm.DB }

func (a voicePersonalAuthority) CheckActor(ctx context.Context, p identity.Principal) error {
	return a.AuthorizePersonal(ctx, p, false)
}
func (a voicePersonalAuthority) AuthorizePersonal(ctx context.Context, p identity.Principal, write bool) error {
	scope, err := scopewire.ProvideScopeAccess(a.tx).Resolve(ctx, p.ID, p.ScopeID, write)
	if err != nil {
		return err
	}
	if scope.Kind != "personal" || scope.PhysicalScopeID != p.OrgID || scope.OwnerUserID == nil || *scope.OwnerUserID != p.ID {
		return media.ErrNotFound
	}
	return nil
}
func (a voicePersonalAuthority) AuthorizeProject(context.Context, identity.Principal, uuid.UUID, bool) error {
	return media.ErrNotFound
}
func (a voicePersonalAuthority) AuthorizeCopyTarget(context.Context, identity.Principal, uuid.UUID) error {
	return media.ErrNotFound
}

// NewVoiceExecutionWithObjects reuses the profile registry owned by subject input transport.
func NewVoiceExecutionWithObjects(db *gorm.DB, objects Objects, prober media.Prober, renderer media.Renderer, keys subject.VoiceUnsealer, speech subject.VoiceSynthesizer) *subject.VoiceWorker {
	return NewVoiceExecution(db, objects.registry, prober, renderer, keys, speech)
}
