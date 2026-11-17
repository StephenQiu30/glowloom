package aigateway

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	subject "github.com/StephenQiu30/glowloom-server/backend/creation/subject/application"
	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
	scopewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/scope"
	subjectwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/subject"
	mediaobjects "github.com/StephenQiu30/glowloom-server/backend/media/adapter/objectstorage"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/media/adapter/workflow"
	media "github.com/StephenQiu30/glowloom-server/backend/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/media/domain"
)

func (r *Runtime) deliverVoice(ctx context.Context, source deliverySource, row voiceIntentRow, manifest outputManifest, token uuid.UUID, file *media.Downloaded) error {
	var in subject.CreateVoiceJobInput
	var owner subject.VoiceOwner
	if json.Unmarshal(row.Input, &in) != nil || json.Unmarshal(row.Owner, &owner) != nil || row.ActorID != source.Call.Actor || row.ScopeID != source.Call.Scope || row.RequestID != source.Call.ID || row.InputHash != domain.Digest(canonical(in)) || manifest.Policy != "provider_success_v1" || manifest.Kind != "audio" || manifest.MIMEType != "audio/mpeg" || in.ReleaseID != source.Call.Model.ID {
		return domain.ErrInvalid
	}
	work := subject.VoiceWork{Job: subject.VoiceJob{ID: row.RequestID, CurrentActorID: row.ActorID, CurrentScopeID: row.ScopeID}, Owner: owner}
	if err := r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error { _, err := subjectwire.NewVoiceAccess(tx).Worker(ctx, work); return err }); err != nil {
		return err
	}
	intent, err := pgmedia.NewStorageRoutes(r.DB).ReadStorageWork(ctx, media.StorageWorkOwner{Kind: media.StorageWorkKindSubjectVoice, OwnerID: row.ScopeID, WorkID: row.RequestID})
	if err != nil {
		return err
	}
	prepare := r.VoiceMedia
	if prepare == nil {
		if r.Objects == nil {
			return domain.ErrUnavailable
		}
		objects := mediaobjects.NewPersonalGeneratedAudioObjects(r.Objects, pgmedia.NewStorageRoutes(r.DB))
		prepare = media.NewPersonalGeneratedAudioService(mediaflow.FFProber{}, mediaflow.FFUploadRenderer{}, objects, time.Now)
	}
	req := media.PersonalGeneratedAudioRequest{AssetID: manifest.AssetID, OwnerScopeID: row.ScopeID, SourceJobID: row.RequestID, Personal: mediadomain.PersonalOwnership{OrgID: owner.PersonalOrgID, ActorID: row.ActorID}, FileName: "智能配音.mp3", CreatedAt: source.Created, SHA256: manifest.SHA256, ByteSize: manifest.ByteSize, MIMEType: manifest.MIMEType, Intent: intent,
		Provenance: media.PersonalGeneratedAudioProvenance{SchemaVersion: 2, GatewayRequestID: row.RequestID, OutputPolicy: manifest.Policy, ReceiptSHA256: manifest.ReceiptSHA, ReleaseID: source.Call.Model.ID, ModelVersionID: source.Call.Model.ID, ProviderRequestID: row.RequestID, ProviderLogID: source.Receipt.UpstreamID}}
	prepared, err := prepare.Prepare(ctx, req, file)
	if err != nil {
		return err
	}
	return r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := r.verifyDeliveryClaim(tx, row.RequestID, token, manifest); err != nil {
			return err
		}
		if _, err := subjectwire.NewVoiceAccess(tx).Worker(ctx, work); err != nil {
			return err
		}
		actor := media.Actor{ID: row.ActorID, ScopeID: owner.PersonalScopeID, OrgID: owner.PersonalOrgID}
		accepted, err := pgmedia.NewScopedStore(tx, func(tx *gorm.DB) media.ScopeAccess { return gatewayVoicePersonalAuthority{tx: tx} }).AcceptPersonalGeneratedAudio(ctx, actor, prepared)
		if err != nil {
			return err
		}
		voice := &subject.VoiceJobResult{AssetID: accepted.ID, AssetRevision: accepted.Revision, Kind: "audio", MIMEType: accepted.MIMEType, FileName: accepted.FileName, LibraryItemID: accepted.ID, LibrarySource: "subject"}
		return finishDelivery(tx, row.RequestID, token, "ready", OutputSummary{RequestID: row.RequestID, AssetID: accepted.ID, Kind: "audio", Status: "ready", SHA256: manifest.SHA256, Voice: voice})
	})
}

type gatewayVoicePersonalAuthority struct{ tx *gorm.DB }

func (a gatewayVoicePersonalAuthority) CheckActor(ctx context.Context, p media.Actor) error {
	return a.AuthorizePersonal(ctx, p, false)
}
func (a gatewayVoicePersonalAuthority) AuthorizePersonal(ctx context.Context, p media.Actor, write bool) error {
	scope, err := scopewire.ProvideScopeAccess(a.tx).Resolve(ctx, p.ID, p.ScopeID, write)
	if err != nil {
		return err
	}
	if scope.Kind != "personal" || scope.PhysicalScopeID != p.OrgID || scope.OwnerUserID == nil || *scope.OwnerUserID != p.ID {
		return media.ErrNotFound
	}
	return nil
}
func (a gatewayVoicePersonalAuthority) AuthorizeProject(context.Context, media.Actor, uuid.UUID, bool) error {
	return media.ErrNotFound
}
func (a gatewayVoicePersonalAuthority) AuthorizeCopyTarget(context.Context, media.Actor, uuid.UUID) error {
	return media.ErrNotFound
}
