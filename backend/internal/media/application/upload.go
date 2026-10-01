package application

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

var (
	// ErrInvalidUpload rejects a malformed upload or missing review declaration.
	ErrInvalidUpload = errors.New("invalid media upload")
	// ErrUnsupportedUpload rejects unsupported or incorrectly labelled bytes.
	ErrUnsupportedUpload = errors.New("unsupported media upload")
	// ErrUploadTooLarge rejects a file or multipart envelope exceeding its cap.
	ErrUploadTooLarge = errors.New("media upload too large")
	// ErrUploadConflict rejects a changed body under an existing idempotency key.
	ErrUploadConflict = errors.New("media upload idempotency conflict")
)

// UploadResult contains only referenceable metadata and an optional reuse notice.
type UploadResult struct {
	Asset       AssetSummary `json:"asset"`
	DuplicateOf *uuid.UUID   `json:"duplicate_of"`
}

// UploadRequest is the immutable, server-hashed local upload command.
type UploadRequest struct {
	ProjectID uuid.UUID
	Key       uuid.UUID
	SHA256    string
	FileName  string
	ByteSize  int64
	RequestID uuid.UUID
}

// UploadRepository authorizes before reading and atomically commits reviewed media.
type UploadRepository interface {
	AuthorizeUpload(context.Context, identityapp.Principal, uuid.UUID) (string, error)
	FindUpload(context.Context, identityapp.Principal, UploadRequest) (UploadResult, bool, error)
	CommitUpload(context.Context, identityapp.Principal, UploadRequest, domain.MediaAsset, []domain.Rendition) (UploadResult, error)
	// UploadAssetExists waits for any commit holding this project's upload lock
	// to finish before establishing whether the owned asset was persisted.
	UploadAssetExists(context.Context, uuid.UUID, uuid.UUID) (bool, error)
}

// UploadObjects removes only unique object keys owned by this upload attempt.
type UploadObjects interface {
	ObjectStore
	Remove(context.Context, string) error
}

// UploadInput carries already bounded bytes and the explicit local review declaration.
type UploadInput struct {
	Actor                identityapp.Principal
	Request              UploadRequest
	File                 *Downloaded
	LocalReviewConfirmed bool
}

// UploadService validates local input and publishes it only after durable object checks.
type UploadService struct {
	repo     UploadRepository
	prober   Prober
	renderer Renderer
	objects  UploadObjects
	now      func() time.Time
}

// NewUploadService injects current permissions, real media tools and private storage.
func NewUploadService(repo UploadRepository, prober Prober, renderer Renderer, objects UploadObjects, now func() time.Time) *UploadService {
	return &UploadService{repo: repo, prober: prober, renderer: renderer, objects: objects, now: now}
}

// Authorize rejects inaccessible or archived projects without reading the body.
func (s *UploadService) Authorize(ctx context.Context, actor identityapp.Principal, project uuid.UUID) error {
	if s == nil || s.repo == nil {
		return ErrUnavailable
	}
	_, err := s.repo.AuthorizeUpload(ctx, actor, project)
	return err
}

// Upload performs the synchronous local-owner-reviewed ingest. Generated media
// continues to use its separate operation and moderation pipeline.
func (s *UploadService) Upload(ctx context.Context, in UploadInput) (result UploadResult, err error) {
	if s == nil || s.repo == nil || s.prober == nil || s.renderer == nil || s.objects == nil || s.now == nil {
		return result, ErrUnavailable
	}
	r := in.Request
	file := in.File
	name, nameErr := SafeUploadFileName(r.FileName)
	if !in.LocalReviewConfirmed || nameErr != nil || name != r.FileName || r.ProjectID == uuid.Nil || r.Key == uuid.Nil || r.RequestID == uuid.Nil ||
		file == nil || file.File == nil || file.Size < 1 || len(file.SHA256) != 64 {
		return result, ErrInvalidUpload
	}
	r.SHA256, r.ByteSize = file.SHA256, file.Size
	aspect, err := s.repo.AuthorizeUpload(ctx, in.Actor, r.ProjectID)
	if err != nil {
		return result, err
	}
	if result, found, err := s.repo.FindUpload(ctx, in.Actor, r); err != nil || found {
		return result, err
	}
	probe, err := s.prober.Probe(ctx, file)
	if err != nil {
		return result, err
	}
	if !validUploadProbe(file, probe) {
		return result, ErrUnsupportedUpload
	}
	rendered, err := s.renderer.Render(ctx, file, probe, aspect)
	if err != nil {
		return result, err
	}
	defer func() {
		for _, rendition := range rendered {
			if rendition.Result != nil {
				_ = rendition.Result.Close()
			}
		}
	}()
	if !completeUploadRendered(probe.Kind, rendered) {
		return result, ErrUnavailable
	}
	now := s.now().UTC()
	if now.IsZero() {
		return result, ErrUnavailable
	}
	id := uuid.New()
	key := path.Join("projects", r.ProjectID.String(), string(probe.Kind), now.Format("2006"), now.Format("01"), id.String()+"."+probe.Extension)
	owned := make([]string, 0, len(rendered)+1)
	commitAttempted := false
	defer func() {
		if err == nil && result.Asset.ID == id {
			return
		}
		cleanCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if commitAttempted && err != nil {
			present, checkErr := s.repo.UploadAssetExists(cleanCtx, r.ProjectID, id)
			if checkErr != nil || present {
				return
			} // An unknown commit never destroys a usable original.
		}
		for _, objectKey := range owned {
			_ = s.objects.Remove(cleanCtx, objectKey)
		}
	}()
	owned = append(owned, key)
	if err = s.putUpload(ctx, key, file); err != nil {
		return result, err
	}
	sha := file.SHA256
	review, err := json.Marshal(LocalUploadReview{Method: "local_workspace_owner_review", PrincipalID: in.Actor.ID, ReviewedAt: now, SHA256: sha, RightsConfirmed: true, NoAuthorizationRequiredRealPerson: true})
	if err != nil {
		return result, err
	}
	asset := domain.MediaAsset{ID: id, ProjectID: r.ProjectID, Kind: probe.Kind, Origin: domain.OriginUpload, Status: domain.StatusReady,
		ObjectKey: key, FileName: name, MimeType: file.MIMEType, ByteSize: file.Size, SHA256: &sha, Width: probe.Width, Height: probe.Height,
		DurationMS: probe.DurationMS, FPS: probe.FPS, AudioChannels: probe.AudioChannels, Codec: probe.Codec,
		ModerationStatus: domain.ModerationPassed, ModerationDetail: review, Revision: 1, CreateTime: now, UpdateTime: now}
	if err = asset.Validate(); err != nil {
		return result, err
	}
	rends := make([]domain.Rendition, 0, len(rendered))
	for _, rendition := range rendered {
		objectKey := path.Join(strings.TrimSuffix(key, path.Ext(key)), string(rendition.Kind)+"."+rendition.Ext)
		owned = append(owned, objectKey)
		if err = s.putUpload(ctx, objectKey, rendition.Result); err != nil {
			return result, err
		}
		width, height, size := rendition.Width, rendition.Height, rendition.Result.Size
		rends = append(rends, domain.Rendition{ID: uuid.New(), MediaAssetID: id, Kind: rendition.Kind, ObjectKey: objectKey,
			Width: &width, Height: &height, ByteSize: &size, CreateTime: now, UpdateTime: now})
	}
	commitAttempted = true
	result, err = s.repo.CommitUpload(ctx, in.Actor, r, asset, rends)
	if err != nil {
		return UploadResult{}, err
	}
	if result.Asset.ProjectID != r.ProjectID || result.Asset.ID == uuid.Nil {
		return UploadResult{}, ErrUnavailable
	}
	return result, nil
}

// LocalUploadReview records an explicit human declaration, never a machine check.
type LocalUploadReview struct {
	Method                            string    `json:"method"`
	PrincipalID                       uuid.UUID `json:"principal_id"`
	ReviewedAt                        time.Time `json:"reviewed_at"`
	SHA256                            string    `json:"sha256"`
	RightsConfirmed                   bool      `json:"rights_confirmed"`
	NoAuthorizationRequiredRealPerson bool      `json:"no_authorization_required_real_person"`
}

func (s *UploadService) putUpload(ctx context.Context, key string, file *Downloaded) error {
	if file == nil || file.File == nil || file.Size < 1 || len(file.SHA256) != 64 {
		return ErrUnavailable
	}
	if _, err := file.File.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if err := s.objects.PutIfAbsent(ctx, key, file.File, file.Size, file.MIMEType, file.SHA256); err != nil {
		return err
	}
	info, err := s.objects.Stat(ctx, key)
	if err != nil {
		return err
	}
	if info.Size != file.Size || info.ContentType != file.MIMEType || info.SHA256 != file.SHA256 {
		return ErrObjectMismatch
	}
	return nil
}

func validUploadProbe(file *Downloaded, p ProbeResult) bool {
	if p.Codec == nil || *p.Codec == "" {
		return false
	}
	// Bound decoded frame size before invoking the renderer. int64 arithmetic
	// prevents an overflowing pixel product from bypassing the upload limit.
	if p.Kind == domain.KindImage || p.Kind == domain.KindVideo {
		if p.Width == nil || p.Height == nil || *p.Width < 1 || *p.Height < 1 || *p.Width > 8192 || *p.Height > 8192 || int64(*p.Width)*int64(*p.Height) > 40_000_000 {
			return false
		}
	}
	switch p.Kind {
	case domain.KindImage:
		return file.Size <= MaxUploadImageBytes && p.Width != nil && p.Height != nil && *p.Width > 0 && *p.Height > 0 &&
			(file.MIMEType == "image/png" && p.Extension == "png" || file.MIMEType == "image/jpeg" && p.Extension == "jpg" || file.MIMEType == "image/webp" && p.Extension == "webp")
	case domain.KindVideo:
		return file.Size <= MaxUploadVideoBytes && p.DurationMS != nil && *p.DurationMS > 0 && *p.DurationMS <= 60000 && p.Width != nil && p.Height != nil &&
			(file.MIMEType == "video/mp4" && p.Extension == "mp4" || file.MIMEType == "video/quicktime" && p.Extension == "mov")
	case domain.KindAudio:
		return file.Size <= MaxUploadAudioBytes && p.DurationMS != nil && *p.DurationMS > 0 && p.AudioChannels != nil &&
			(file.MIMEType == "audio/mpeg" && p.Extension == "mp3" || file.MIMEType == "audio/wave" && p.Extension == "wav" || file.MIMEType == "audio/mp4" && p.Extension == "m4a")
	case domain.KindModel:
		return file.Size <= MaxUploadModelBytes && file.MIMEType == "model/gltf-binary" && p.Extension == "glb" && *p.Codec == "glb2" &&
			p.Width == nil && p.Height == nil && p.DurationMS == nil && p.FPS == nil && p.AudioChannels == nil
	default:
		return false
	}
}

func completeUploadRendered(kind domain.Kind, files []RenditionFile) bool {
	if kind == domain.KindModel {
		return len(files) == 0
	}
	required := requiredRenditions(kind)
	if kind == domain.KindAudio {
		required = []domain.RenditionKind{domain.RenditionWaveform}
	}
	if len(files) != len(required) {
		return false
	}
	for i, target := range required {
		f := files[i]
		if f.Kind != target || f.Result == nil || f.Result.File == nil || f.Width < 1 || f.Height < 1 || f.Ext == "" {
			return false
		}
	}
	return true
}

// UploadSummary is the repository's safe projection of a reviewed upload.
func UploadSummary(asset domain.MediaAsset) AssetSummary { return summary(asset) }

// ValidateUploadRequest enforces the durable receipt's bounded identity.
func ValidateUploadRequest(r UploadRequest) error {
	name, err := SafeUploadFileName(r.FileName)
	if err != nil || name != r.FileName || r.ProjectID == uuid.Nil || r.Key == uuid.Nil || r.RequestID == uuid.Nil || r.ByteSize < 1 || r.ByteSize > MaxUploadVideoBytes || len(r.SHA256) != 64 {
		return ErrInvalidUpload
	}
	for _, c := range r.SHA256 {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return ErrInvalidUpload
		}
	}
	return nil
}
