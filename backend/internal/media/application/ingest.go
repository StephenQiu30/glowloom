// Package application coordinates generated media ingestion and its durable facts.
package application

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
	operationapp "github.com/StephenQiu30/lanverse/backend/internal/operation/application"
)

var (
	// ErrInvalidIngest means the workflow supplied an unusable operation/result identity.
	ErrInvalidIngest = errors.New("invalid media ingest request")
	// ErrObjectMismatch means the persisted object differs from the bytes checked locally.
	ErrObjectMismatch = errors.New("stored media object metadata mismatch")
	// ErrObjectAlreadyExists means a concurrent retry created the same key.
	ErrObjectAlreadyExists = errors.New("media object already exists")
)

// IngestInput is the stable Temporal activity payload.
type IngestInput struct {
	OperationID string                        `json:"operation_id"`
	SeqNo       int                           `json:"seq_no"`
	URL         string                        `json:"url"`
	Receipt     *operationapp.ProviderReceipt `json:"receipt,omitempty"`
}

// IngestOutput names the media and candidate records created for one result.
type IngestOutput struct {
	OutputID     string `json:"output_id"`
	MediaAssetID string `json:"media_asset_id"`
	ObjectKey    string `json:"object_key"`
	Kind         string `json:"kind"`
	MIMEType     string `json:"mime_type"`
	ByteSize     int64  `json:"byte_size"`
	SHA256       string `json:"sha256"`
}

// SourceOperation holds only the persisted facts needed to bind a generated
// media asset to its project, provider, model, and stable object path.
type SourceOperation struct {
	ID          uuid.UUID
	ProjectID   uuid.UUID
	ProviderKey string
	ModelKey    string
	Region      string
	AspectRatio string
	OutputKind  domain.Kind
	CreateTime  time.Time
}

// Downloaded owns a bounded temporary result. Close removes it.
type Downloaded struct {
	File     *os.File
	Size     int64
	MIMEType string
	SHA256   string
}

// Close closes and removes the temporary result file.
func (d *Downloaded) Close() error {
	if d == nil || d.File == nil {
		return nil
	}
	name := d.File.Name()
	closeErr := d.File.Close()
	removeErr := os.Remove(name)
	d.File = nil
	return errors.Join(closeErr, removeErr)
}

// ProbeResult is derived from the bytes by ffprobe, never from a URL suffix.
type ProbeResult struct {
	Kind          domain.Kind
	Extension     string
	Width         *int32
	Height        *int32
	DurationMS    *int32
	FPS           *float64
	AudioChannels *int32
	Codec         *string
}

// RenditionFile contains one verified preview produced from the result bytes.
type RenditionFile struct {
	Kind   domain.RenditionKind
	Result *Downloaded
	Width  int32
	Height int32
	Ext    string
}

// Renderer creates the previews supported by the current media.rendition schema.
type Renderer interface {
	Render(context.Context, *Downloaded, ProbeResult, string) ([]RenditionFile, error)
}

// Repository owns the database boundary. Save atomically creates the media
// asset and operation output, returning the existing row on an Activity replay.
type Repository interface {
	FindOutput(context.Context, uuid.UUID, int) (IngestOutput, bool, error)
	LoadOperation(context.Context, uuid.UUID) (SourceOperation, error)
	SaveOutput(context.Context, domain.MediaAsset, []domain.Rendition, int, uuid.UUID) (IngestOutput, error)
	FindRenditions(context.Context, uuid.UUID) ([]domain.Rendition, error)
}

// Downloader fetches one provider result through an SSRF-safe network boundary.
type Downloader interface {
	Download(context.Context, string) (*Downloaded, error)
}

// StagedSource binds a private image to its durable provider receipt before
// reading bounded bytes. It accepts no URL, path, or object key from a caller.
type StagedSource interface {
	ValidateReceipt(context.Context, operationapp.ProviderReceipt) error
	Download(context.Context, operationapp.ProviderReceipt) (*Downloaded, error)
}

// Prober verifies the downloaded bytes and extracts media metadata.
type Prober interface {
	Probe(context.Context, *Downloaded) (ProbeResult, error)
}

// ObjectStore conditionally creates one result object and reads its metadata.
type ObjectStore interface {
	PutIfAbsent(context.Context, string, io.Reader, int64, string, string) error
	Stat(context.Context, string) (ObjectInfo, error)
}

// ObjectInfo contains metadata needed to verify a conditional write.
type ObjectInfo struct {
	Size        int64
	ContentType string
	SHA256      string
}

// IngestService owns the order of external transfer and durable registration.
type IngestService struct {
	repo       Repository
	downloader Downloader
	staged     StagedSource
	prober     Prober
	renderer   Renderer
	objects    ObjectStore
	now        func() time.Time
}

// NewIngestService injects the database and external transfer boundaries.
func NewIngestService(repo Repository, downloader Downloader, prober Prober, renderer Renderer, objects ObjectStore) *IngestService {
	return &IngestService{repo: repo, downloader: downloader, prober: prober, renderer: renderer, objects: objects, now: time.Now}
}

// NewStagedIngestService injects the receipt boundary for private provider bytes.
func NewStagedIngestService(repo Repository, source StagedSource, prober Prober, renderer Renderer, objects ObjectStore) *IngestService {
	return &IngestService{repo: repo, staged: source, prober: prober, renderer: renderer, objects: objects, now: time.Now}
}

// Ingest transfers one result and atomically registers its media and output rows.
func (s *IngestService) Ingest(ctx context.Context, input IngestInput) (IngestOutput, error) {
	opID, err := uuid.Parse(input.OperationID)
	if err != nil || opID == uuid.Nil || input.SeqNo < 1 || input.SeqNo > 8 || (input.URL == "") == (input.Receipt == nil) {
		return IngestOutput{}, ErrInvalidIngest
	}
	if input.Receipt != nil {
		if s.staged == nil || input.Receipt.Validate() != nil || input.Receipt.Identity.OperationID != opID || input.Receipt.Outputs[0].Sequence != int32(input.SeqNo) {
			return IngestOutput{}, ErrInvalidIngest
		}
		if err := s.staged.ValidateReceipt(ctx, *input.Receipt); err != nil {
			return IngestOutput{}, fmt.Errorf("validate staged media receipt: %w", err)
		}
	} else if s.downloader == nil {
		return IngestOutput{}, ErrInvalidIngest
	}
	if existing, found, err := s.repo.FindOutput(ctx, opID, input.SeqNo); err != nil {
		return IngestOutput{}, fmt.Errorf("find media output: %w", err)
	} else if found {
		if input.Receipt != nil {
			output := input.Receipt.Outputs[0]
			if existing.Kind != string(domain.KindImage) || existing.ByteSize != output.SizeBytes || existing.MIMEType != output.MIMEType || existing.SHA256 != output.SHA256 {
				return IngestOutput{}, ErrObjectMismatch
			}
		}
		if err := s.verifyObject(ctx, existing.ObjectKey, existing.ByteSize, existing.MIMEType, existing.SHA256); err != nil {
			return IngestOutput{}, err
		}
		assetID, parseErr := uuid.Parse(existing.MediaAssetID)
		if parseErr != nil || assetID == uuid.Nil {
			return IngestOutput{}, ErrObjectMismatch
		}
		renditions, err := s.repo.FindRenditions(ctx, assetID)
		if err != nil {
			return IngestOutput{}, fmt.Errorf("find existing media renditions: %w", err)
		}
		if !completeRenditions(domain.Kind(existing.Kind), renditions) {
			return IngestOutput{}, ErrObjectMismatch
		}
		for _, rendition := range renditions {
			if rendition.ByteSize == nil {
				return IngestOutput{}, ErrObjectMismatch
			}
			// Rendition SHA is kept in the private object's user metadata.
			stored, err := s.objects.Stat(ctx, rendition.ObjectKey)
			if err != nil || stored.Size != *rendition.ByteSize || stored.SHA256 == "" {
				return IngestOutput{}, ErrObjectMismatch
			}
		}
		return existing, nil
	}
	op, err := s.repo.LoadOperation(ctx, opID)
	if err != nil {
		return IngestOutput{}, fmt.Errorf("load source operation: %w", err)
	}
	if input.Receipt != nil && input.Receipt.Identity.ProjectID != op.ProjectID {
		return IngestOutput{}, ErrInvalidIngest
	}
	var downloaded *Downloaded
	if input.Receipt != nil {
		downloaded, err = s.staged.Download(ctx, *input.Receipt)
	} else {
		downloaded, err = s.downloader.Download(ctx, input.URL)
	}
	if err != nil {
		return IngestOutput{}, fmt.Errorf("download media result: %w", err)
	}
	if downloaded == nil || downloaded.File == nil {
		return IngestOutput{}, ErrInvalidIngest
	}
	defer func() { _ = downloaded.Close() }()
	probe, err := s.prober.Probe(ctx, downloaded)
	if err != nil {
		return IngestOutput{}, fmt.Errorf("probe media result: %w", err)
	}
	if probe.Extension == "" || probe.Kind == "" {
		return IngestOutput{}, ErrInvalidIngest
	}
	if probe.Kind != op.OutputKind {
		return IngestOutput{}, ErrInvalidIngest
	}
	rendered, err := s.renderer.Render(ctx, downloaded, probe, op.AspectRatio)
	if err != nil {
		return IngestOutput{}, fmt.Errorf("render media previews: %w", err)
	}
	if !completeRendered(probe.Kind, rendered) {
		for _, rendition := range rendered {
			if rendition.Result != nil {
				_ = rendition.Result.Close()
			}
		}
		return IngestOutput{}, ErrInvalidIngest
	}
	defer func() {
		for _, rendition := range rendered {
			_ = rendition.Result.Close()
		}
	}()
	assetID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("media-asset/"+opID.String()+"/"+strconv.Itoa(input.SeqNo)))
	outputID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("operation-output/"+opID.String()+"/"+strconv.Itoa(input.SeqNo)))
	objectKey := path.Join("projects", op.ProjectID.String(), string(probe.Kind),
		op.CreateTime.UTC().Format("2006"), op.CreateTime.UTC().Format("01"), assetID.String()+"."+probe.Extension)
	if _, err := downloaded.File.Seek(0, io.SeekStart); err != nil {
		return IngestOutput{}, fmt.Errorf("rewind media result: %w", err)
	}
	if err := s.objects.PutIfAbsent(ctx, objectKey, downloaded.File, downloaded.Size, downloaded.MIMEType, downloaded.SHA256); err != nil && !errors.Is(err, ErrObjectAlreadyExists) {
		return IngestOutput{}, fmt.Errorf("store media result: %w", err)
	}
	if err := s.verifyObject(ctx, objectKey, downloaded.Size, downloaded.MIMEType, downloaded.SHA256); err != nil {
		return IngestOutput{}, err
	}
	now := s.now().UTC()
	providerKey, modelKey, region := op.ProviderKey, op.ModelKey, op.Region
	sha := downloaded.SHA256
	asset := domain.MediaAsset{
		ID: assetID, ProjectID: op.ProjectID, Kind: probe.Kind,
		Origin: domain.OriginGenerated, Status: domain.StatusProcessing,
		ObjectKey: objectKey, MimeType: downloaded.MIMEType, ByteSize: downloaded.Size,
		SHA256: &sha, Width: probe.Width, Height: probe.Height, DurationMS: probe.DurationMS,
		FPS: probe.FPS, AudioChannels: probe.AudioChannels, Codec: probe.Codec,
		SourceOperationID: &opID, ProviderKey: &providerKey, ModelKey: &modelKey,
		Region: &region, ModerationStatus: domain.ModerationPending,
		Revision: 1, CreateTime: now, UpdateTime: now,
	}
	if err := asset.Validate(); err != nil {
		return IngestOutput{}, fmt.Errorf("validate generated media asset: %w", err)
	}
	toSave := make([]domain.Rendition, 0, len(rendered))
	for _, file := range rendered {
		if file.Result == nil || file.Result.File == nil || file.Ext == "" || file.Width < 1 || file.Height < 1 {
			return IngestOutput{}, ErrInvalidIngest
		}
		key := path.Join(strings.TrimSuffix(objectKey, path.Ext(objectKey)), string(file.Kind)+"."+file.Ext)
		if _, err := file.Result.File.Seek(0, io.SeekStart); err != nil {
			return IngestOutput{}, fmt.Errorf("rewind media preview: %w", err)
		}
		if err := s.objects.PutIfAbsent(ctx, key, file.Result.File, file.Result.Size, file.Result.MIMEType, file.Result.SHA256); err != nil && !errors.Is(err, ErrObjectAlreadyExists) {
			return IngestOutput{}, fmt.Errorf("store media preview: %w", err)
		}
		if err := s.verifyObject(ctx, key, file.Result.Size, file.Result.MIMEType, file.Result.SHA256); err != nil {
			return IngestOutput{}, err
		}
		width, height, size := file.Width, file.Height, file.Result.Size
		rendition := domain.Rendition{
			ID:           uuid.NewSHA1(uuid.NameSpaceOID, []byte("media-rendition/"+assetID.String()+"/"+string(file.Kind))),
			MediaAssetID: assetID, Kind: file.Kind, ObjectKey: key,
			Width: &width, Height: &height, ByteSize: &size,
			CreateTime: now, UpdateTime: now,
		}
		if err := rendition.Validate(); err != nil {
			return IngestOutput{}, fmt.Errorf("validate media preview: %w", err)
		}
		toSave = append(toSave, rendition)
	}
	result, err := s.repo.SaveOutput(ctx, asset, toSave, input.SeqNo, outputID)
	if err != nil {
		return IngestOutput{}, fmt.Errorf("register media output: %w", err)
	}
	return result, nil
}

func requiredRenditions(kind domain.Kind) []domain.RenditionKind {
	switch kind {
	case domain.KindImage:
		return []domain.RenditionKind{domain.RenditionThumb256, domain.RenditionThumb640}
	case domain.KindVideo:
		return []domain.RenditionKind{domain.RenditionPoster, domain.RenditionProxy720p}
	case domain.KindAudio:
		return nil
	default:
		return nil
	}
}

func completeRendered(kind domain.Kind, rendered []RenditionFile) bool {
	required := requiredRenditions(kind)
	if len(rendered) != len(required) {
		return false
	}
	for i, target := range required {
		if rendered[i].Kind != target || rendered[i].Result == nil {
			return false
		}
	}
	return true
}

func completeRenditions(kind domain.Kind, renditions []domain.Rendition) bool {
	required := requiredRenditions(kind)
	if len(renditions) != len(required) {
		return false
	}
	for i, target := range required {
		if renditions[i].Kind != target {
			return false
		}
	}
	return true
}

func (s *IngestService) verifyObject(ctx context.Context, key string, size int64, contentType, sha string) error {
	stored, err := s.objects.Stat(ctx, key)
	if err != nil {
		return fmt.Errorf("verify media object: %w", err)
	}
	if stored.Size != size || stored.ContentType != contentType || stored.SHA256 != sha {
		return ErrObjectMismatch
	}
	return nil
}
