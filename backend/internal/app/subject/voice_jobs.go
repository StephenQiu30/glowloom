package subject

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	scopewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/scope"
	pgcatalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/postgres"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	pgsubject "github.com/StephenQiu30/glowloom-server/backend/internal/subject/adapter/postgres"
	subject "github.com/StephenQiu30/glowloom-server/backend/internal/subject/application"
	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/postgres"
	workspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

type voiceAccess struct{ tx *gorm.DB }

// NewVoiceAccess composes current request and durable worker ownership.
func NewVoiceAccess(tx *gorm.DB) subject.VoiceAccess { return voiceAccess{tx: tx} }

func (v voiceAccess) Request(ctx context.Context, a identity.Principal, write bool) (subject.VoiceOwner, error) {
	current, err := (scopeAccess{tx: v.tx}).Resolve(ctx, a, write)
	if err != nil {
		return subject.VoiceOwner{}, err
	}
	store := pgworkspace.NewScopeStore(v.tx, func(tx *gorm.DB) workspace.ActorAccess { return pgidentity.NewStore(tx) })
	personalID, err := store.PersonalScopeID(ctx, a.ID)
	if err != nil {
		return subject.VoiceOwner{}, err
	}
	personal, err := scopewire.ProvideRequestScopeAccess(v.tx).Resolve(ctx, a, personalID, write)
	if err != nil {
		return subject.VoiceOwner{}, err
	}
	if personal.Kind != "personal" || personal.OwnerUserID == nil || *personal.OwnerUserID != a.ID {
		return subject.VoiceOwner{}, subject.ErrVoiceJobNotFound
	}
	return subject.VoiceOwner{ScopeID: current.ScopeID, PhysicalScopeID: current.PhysicalScopeID, PersonalScopeID: personal.ID, PersonalOrgID: personal.PhysicalScopeID}, nil
}

func (v voiceAccess) Worker(ctx context.Context, work subject.VoiceWork) (subject.VoiceOwner, error) {
	current, err := scopewire.ProvideScopeAccess(v.tx).Resolve(ctx, work.Job.CurrentActorID, work.Owner.ScopeID, true)
	if err != nil {
		return subject.VoiceOwner{}, err
	}
	personal, err := scopewire.ProvideScopeAccess(v.tx).Resolve(ctx, work.Job.CurrentActorID, work.Owner.PersonalScopeID, true)
	if err != nil {
		return subject.VoiceOwner{}, err
	}
	if current.PhysicalScopeID != work.Owner.PhysicalScopeID || personal.Kind != "personal" || personal.PhysicalScopeID != work.Owner.PersonalOrgID || personal.OwnerUserID == nil || *personal.OwnerUserID != work.Job.CurrentActorID {
		return subject.VoiceOwner{}, subject.ErrVoiceJobNotFound
	}
	return subject.VoiceOwner{ScopeID: current.ID, PhysicalScopeID: current.PhysicalScopeID, PersonalScopeID: personal.ID, PersonalOrgID: personal.PhysicalScopeID}, nil
}

type voiceStorage struct{ routes *pgmedia.StorageRoutes }

func (v voiceStorage) Ready(ctx context.Context) (bool, error) { return v.routes.ReadyForWork(ctx) }

func voiceStorageFact(intent media.StorageWorkIntent) subject.VoiceStorageIntent {
	return subject.VoiceStorageIntent{ID: intent.ID, ProfileID: intent.ProfileID, Bucket: intent.Bucket, ConfigRevision: intent.ConfigRevision, State: intent.State}
}
func (v voiceStorage) Pin(ctx context.Context, actor, id uuid.UUID) (subject.VoiceStorageIntent, error) {
	intent, err := v.routes.PinStorageWork(ctx, media.StorageWorkOwner{Kind: "subject-voice", OwnerID: actor, WorkID: id})
	return voiceStorageFact(intent), err
}
func (v voiceStorage) Read(ctx context.Context, actor, id uuid.UUID) (subject.VoiceStorageIntent, error) {
	intent, err := v.routes.ReadStorageWork(ctx, media.StorageWorkOwner{Kind: "subject-voice", OwnerID: actor, WorkID: id})
	return voiceStorageFact(intent), err
}

// NewVoiceJobs admits project-free jobs through real existing owners and ledger.
func NewVoiceJobs(db *gorm.DB) *pgsubject.VoiceStore {
	return pgsubject.NewVoiceStore(db, pgsubject.VoiceFactories{Access: NewVoiceAccess, Catalog: func(tx *gorm.DB) subject.VoiceCatalog { return pgcatalog.NewSubjectVoiceCatalog(tx) }, Storage: func(tx *gorm.DB) subject.VoiceStorage { return voiceStorage{routes: pgmedia.NewStorageRoutes(tx)} }})
}
