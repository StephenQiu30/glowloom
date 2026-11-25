package application

import (
	"context"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/media/domain"
)

// GeneratedOutputArchiver owns actual original/rendition bytes on a Media-local
// frozen route. It cannot read an operation table or call a paid provider.
type GeneratedOutputArchiver struct {
	routes   StorageWorkAdmission
	objects  RoutedIngestObjects
	prober   Prober
	renderer Renderer
}

// NewGeneratedOutputArchiver uses the same object ledger and real FFmpeg pipeline
// as the existing generated-media path, with source facts supplied by the owner.
func NewGeneratedOutputArchiver(routes StorageWorkAdmission, objects RoutedIngestObjects, prober Prober, renderer Renderer) *GeneratedOutputArchiver {
	return &GeneratedOutputArchiver{routes: routes, objects: objects, prober: prober, renderer: renderer}
}

// ArchiveGatewaySource archives saved AI receipt bytes without creating an old operation or write-fence command.
// SourceOperation is used only as immutable source metadata; its ID is the new AI request.
func (a *GeneratedOutputArchiver) ArchiveGatewaySource(ctx context.Context, source SourceOperation, assetID uuid.UUID, file *Downloaded) (domain.MediaAsset, []domain.Rendition, error) {
	if a == nil || a.routes == nil || a.objects == nil || a.prober == nil || a.renderer == nil || source.ID == uuid.Nil || source.ProjectID == uuid.Nil || source.CreateTime.IsZero() || source.ProviderKey == "" || source.ModelKey == "" || (source.Region != "domestic" && source.Region != "overseas") || assetID == uuid.Nil || file == nil || file.File == nil {
		return domain.MediaAsset{}, nil, ErrInvalidIngest
	}
	return a.archiveSource(ctx, source, assetID, file, true)
}

func (a *GeneratedOutputArchiver) archiveSource(ctx context.Context, source SourceOperation, assetID uuid.UUID, file *Downloaded, gateway bool) (domain.MediaAsset, []domain.Rendition, error) {
	var asset domain.MediaAsset
	owner := StorageWorkOwner{Kind: StorageWorkKindOperation, OwnerID: source.ProjectID, WorkID: source.ID}
	intent, err := a.routes.ReadStorageWork(ctx, owner)
	if err != nil {
		return asset, nil, err
	}
	if intent.Owner != owner || intent.ID == uuid.Nil || intent.State != StorageWorkOpen {
		return asset, nil, ErrStorageRouteConflict
	}
	if _, err := file.File.Seek(0, io.SeekStart); err != nil {
		return asset, nil, err
	}
	probe, err := a.prober.Probe(ctx, file)
	if err != nil {
		return asset, nil, err
	}
	if probe.Kind != source.OutputKind || probe.Extension == "" || strings.ContainsAny(probe.Extension, "./\\") {
		return asset, nil, ErrObjectMismatch
	}
	rendered, err := a.renderer.Render(ctx, file, probe, source.AspectRatio)
	if err != nil {
		return asset, nil, err
	}
	defer func() {
		for _, r := range rendered {
			if r.Result != nil {
				_ = r.Result.Close()
			}
		}
	}()
	if !completeRendered(probe.Kind, rendered) {
		return asset, nil, ErrInvalidIngest
	}
	key := path.Join("projects", source.ProjectID.String(), string(probe.Kind), source.CreateTime.UTC().Format("2006"), source.CreateTime.UTC().Format("01"), assetID.String()+"."+probe.Extension)
	if _, err := file.File.Seek(0, io.SeekStart); err != nil {
		return asset, nil, err
	}
	original, err := a.objects.CreateOperationObject(ctx, intent, key, file)
	if err != nil {
		return asset, nil, err
	}
	if !validIngestLocation(&intent, &original, key, file.Size, file.MIMEType, file.SHA256) {
		return asset, nil, ErrObjectMismatch
	}
	now := time.Now().UTC()
	sha := file.SHA256
	provider, model, region := source.ProviderKey, source.ModelKey, source.Region
	asset = domain.MediaAsset{ID: assetID, ProjectID: source.ProjectID, Kind: probe.Kind, Origin: domain.OriginGenerated, Status: domain.StatusProcessing, ObjectKey: key, ObjectLocation: &original, MimeType: file.MIMEType, ByteSize: file.Size, SHA256: &sha, Width: probe.Width, Height: probe.Height, DurationMS: probe.DurationMS, FPS: probe.FPS, AudioChannels: probe.AudioChannels, Codec: probe.Codec, SourceOperationID: &source.ID, ProviderKey: &provider, ModelKey: &model, Region: &region, ModerationStatus: domain.ModerationPending, Revision: 1, CreateTime: now, UpdateTime: now}
	if gateway {
		asset.SourceOperationID = nil
		asset.SourceAIRequestID = &source.ID
		asset.AIGCMarked = true
	}
	if err := asset.Validate(); err != nil {
		return asset, nil, err
	}
	renditions := make([]domain.Rendition, 0, len(rendered))
	for _, r := range rendered {
		if r.Result == nil || r.Result.File == nil || r.Width < 1 || r.Height < 1 || r.Ext == "" || strings.ContainsAny(r.Ext, "./\\") {
			return asset, nil, ErrInvalidIngest
		}
		renditionKey := path.Join(strings.TrimSuffix(key, path.Ext(key)), string(r.Kind)+"."+r.Ext)
		if _, err := r.Result.File.Seek(0, io.SeekStart); err != nil {
			return asset, nil, err
		}
		location, err := a.objects.CreateOperationObject(ctx, intent, renditionKey, r.Result)
		if err != nil {
			return asset, nil, err
		}
		if !validIngestLocation(&intent, &location, renditionKey, r.Result.Size, r.Result.MIMEType, r.Result.SHA256) {
			return asset, nil, ErrObjectMismatch
		}
		size, width, height := r.Result.Size, r.Width, r.Height
		rendition := domain.Rendition{ID: uuid.NewSHA1(uuid.NameSpaceOID, []byte("media-rendition/"+asset.ID.String()+"/"+string(r.Kind))), MediaAssetID: asset.ID, Kind: r.Kind, ObjectKey: renditionKey, ObjectLocation: &location, Width: &width, Height: &height, ByteSize: &size, CreateTime: now, UpdateTime: now}
		if err := rendition.Validate(); err != nil {
			return asset, nil, fmt.Errorf("validate output rendition: %w", err)
		}
		renditions = append(renditions, rendition)
	}
	return asset, renditions, nil
}
