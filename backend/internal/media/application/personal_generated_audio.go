package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// ErrInvalidGeneratedAudio rejects an incomplete, changed or unowned TTS receipt.
var ErrInvalidGeneratedAudio = errors.New("invalid personal generated audio")

// PersonalGeneratedAudioProvenance is the fixed terminal receipt accepted by the
// subject owner. It is technical provider evidence, never a local upload review.
type PersonalGeneratedAudioProvenance struct {
	SchemaVersion     int       `json:"schema_version"`
	ReceiptSHA256     string    `json:"receipt_sha256"`
	ReleaseID         uuid.UUID `json:"release_id"`
	ModelVersionID    uuid.UUID `json:"model_version_id"`
	ProviderRequestID uuid.UUID `json:"provider_request_id"`
	ProviderLogID     string    `json:"provider_log_id,omitempty"`
	TerminalCode      int       `json:"terminal_code"`
}

// PersonalGeneratedAudioRequest comes exclusively from an authorized, frozen
// voice job. The owner pins Intent during admission; ingest cannot pick a route.
type PersonalGeneratedAudioRequest struct {
	AssetID, OwnerScopeID, SourceJobID uuid.UUID
	Personal                           domain.PersonalOwnership
	FileName                           string
	CreatedAt                          time.Time
	SHA256                             string
	ByteSize                           int64
	MIMEType                           string
	Intent                             StorageWorkIntent
	Provenance                         PersonalGeneratedAudioProvenance
}

// Validate closes identity, terminal evidence and the original object facts.
func (r PersonalGeneratedAudioRequest) Validate() error {
	p := r.Provenance
	name, err := SafeUploadFileName(r.FileName)
	if err != nil || name != r.FileName || !strings.EqualFold(path.Ext(name), ".mp3") ||
		r.AssetID == uuid.Nil || r.OwnerScopeID == uuid.Nil || r.SourceJobID == uuid.Nil ||
		r.Personal.OrgID == uuid.Nil || r.Personal.ActorID == uuid.Nil || r.CreatedAt.IsZero() ||
		!uploadFactHash(r.SHA256) || r.ByteSize < 1 || r.ByteSize > MaxUploadAudioBytes || r.MIMEType != "audio/mpeg" ||
		p.SchemaVersion != 1 || !uploadFactHash(p.ReceiptSHA256) || p.ReleaseID == uuid.Nil || p.ModelVersionID == uuid.Nil ||
		p.ProviderRequestID != r.SourceJobID || p.TerminalCode != 20000000 ||
		len(p.ProviderLogID) > 256 || !utf8.ValidString(p.ProviderLogID) || strings.TrimSpace(p.ProviderLogID) != p.ProviderLogID ||
		r.Intent.ID == uuid.Nil || r.Intent.Owner.Validate() != nil || r.Intent.Owner.Kind != "subject-voice" ||
		r.Intent.Owner.OwnerID != r.OwnerScopeID || r.Intent.Owner.WorkID != r.SourceJobID ||
		r.Intent.State != "open" && r.Intent.State != "accepted" || strings.TrimSpace(r.Intent.ConfigRevision) == "" {
		return ErrInvalidGeneratedAudio
	}
	for _, char := range p.ProviderLogID {
		if unicode.IsControl(char) {
			return ErrInvalidGeneratedAudio
		}
	}
	ref := domain.ObjectRef{ProfileID: r.Intent.ProfileID, Bucket: r.Intent.Bucket, Key: r.objectKey()}
	if ref.Validate() != nil {
		return ErrInvalidGeneratedAudio
	}
	return nil
}

func (r PersonalGeneratedAudioRequest) objectKey() string {
	return path.Join("personal", r.Personal.OrgID.String(), r.Personal.ActorID.String(), "audio",
		r.CreatedAt.UTC().Format("2006"), r.CreatedAt.UTC().Format("01"), r.AssetID.String()+".mp3")
}

// ObjectKey returns the stable original path for staging before receipt freeze.
// It contains no access grant and must never be included in a public job result.
func (r PersonalGeneratedAudioRequest) ObjectKey() string { return r.objectKey() }

// PersonalGeneratedAudioObjects writes only on the immutable admitted intent and
// returns verified location facts. Its adapter records intent before SDK I/O.
type PersonalGeneratedAudioObjects interface {
	CreateGeneratedAudioObject(context.Context, StorageWorkIntent, string, *Downloaded) (domain.ObjectLocation, error)
}

// PreparedPersonalGeneratedAudio has no live file or authority to commit itself.
// Accept must run inside the voice owner's result transaction.
type PreparedPersonalGeneratedAudio struct {
	Request    PersonalGeneratedAudioRequest
	Asset      domain.MediaAsset
	Renditions []domain.Rendition
}

type generatedAudioProof struct {
	Method string `json:"method"`
	PersonalGeneratedAudioProvenance
	SourceJobID  uuid.UUID `json:"source_job_id"`
	OwnerScopeID uuid.UUID `json:"owner_scope_id"`
	ActorID      uuid.UUID `json:"actor_id"`
	PersonalOrg  uuid.UUID `json:"personal_org_id"`
	AssetID      uuid.UUID `json:"asset_id"`
	SHA256       string    `json:"audio_sha256"`
	ByteSize     int64     `json:"audio_byte_size"`
	MIMEType     string    `json:"audio_mime_type"`
}

func generatedAudioEvidence(r PersonalGeneratedAudioRequest) generatedAudioProof {
	return generatedAudioProof{Method: "subject_voice_provider_receipt", PersonalGeneratedAudioProvenance: r.Provenance,
		SourceJobID: r.SourceJobID, OwnerScopeID: r.OwnerScopeID, ActorID: r.Personal.ActorID, PersonalOrg: r.Personal.OrgID,
		AssetID: r.AssetID, SHA256: r.SHA256, ByteSize: r.ByteSize, MIMEType: r.MIMEType}
}

// Validate rejects edits between preparation and the caller-owned SQL commit.
func (p PreparedPersonalGeneratedAudio) Validate() error {
	r, a := p.Request, p.Asset
	if r.Validate() != nil || a.Validate() != nil || a.ID != r.AssetID || a.Personal == nil || *a.Personal != r.Personal ||
		a.ProjectID != uuid.Nil || a.Origin != domain.OriginSystem || a.Status != domain.StatusReady ||
		a.Kind != domain.KindAudio || a.ModerationStatus != domain.ModerationPassed || !a.AIGCMarked ||
		a.ContainsRealPerson || a.ConsentRecordID != nil || a.SourceOperationID != nil || a.UploadID != nil ||
		a.ProviderKey != nil || a.ModelKey != nil || a.Region != nil || a.FailureReason != nil || a.IsDelete ||
		a.ObjectKey != r.objectKey() || a.FileName != r.FileName || a.MimeType != r.MIMEType ||
		a.SHA256 == nil || *a.SHA256 != r.SHA256 || a.ByteSize != r.ByteSize || a.Revision != 1 ||
		!a.CreateTime.Equal(r.CreatedAt) || !a.UpdateTime.Equal(r.CreatedAt) ||
		!validGeneratedAudioProbe(ProbeResult{Kind: a.Kind, Extension: "mp3", DurationMS: a.DurationMS, AudioChannels: a.AudioChannels, Codec: a.Codec, Width: a.Width, Height: a.Height, FPS: a.FPS}) ||
		!generatedAudioLocation(r.Intent, a.ObjectLocation, a.ObjectKey, a.ByteSize, a.MimeType, r.SHA256) || len(p.Renditions) != 1 {
		return ErrInvalidGeneratedAudio
	}
	var proof generatedAudioProof
	decoder := json.NewDecoder(bytes.NewReader(a.ModerationDetail))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&proof) != nil || decoder.Decode(new(any)) != io.EOF || proof != generatedAudioEvidence(r) {
		return ErrInvalidGeneratedAudio
	}
	rend := p.Renditions[0]
	if rend.Validate() != nil || rend.ID != generatedAudioRenditionID(r.AssetID) || rend.MediaAssetID != r.AssetID ||
		rend.Kind != domain.RenditionWaveform || rend.ObjectKey != strings.TrimSuffix(a.ObjectKey, ".mp3")+"/waveform.png" ||
		rend.Width == nil || *rend.Width != 1280 || rend.Height == nil || *rend.Height != 256 ||
		rend.ByteSize == nil || *rend.ByteSize < 1 || *rend.ByteSize > MaxUploadImageBytes || rend.IsDelete ||
		!rend.CreateTime.Equal(r.CreatedAt) || !rend.UpdateTime.Equal(r.CreatedAt) || rend.ObjectLocation == nil ||
		rend.ObjectLocation.SHA256 == nil || !uploadFactHash(*rend.ObjectLocation.SHA256) ||
		!generatedAudioLocation(r.Intent, rend.ObjectLocation, rend.ObjectKey, *rend.ByteSize, "image/png", *rend.ObjectLocation.SHA256) ||
		rend.ObjectLocation.ID == a.ObjectLocation.ID {
		return ErrInvalidGeneratedAudio
	}
	return nil
}

func generatedAudioRenditionID(asset uuid.UUID) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("subject-voice-waveform/"+asset.String()))
}

func generatedAudioLocation(intent StorageWorkIntent, l *domain.ObjectLocation, key string, size int64, mime, sha string) bool {
	return l != nil && l.ID != uuid.Nil && l.Ref.Validate() == nil && l.Ref.ProfileID == intent.ProfileID && l.Ref.Bucket == intent.Bucket && l.Ref.Key == key &&
		(l.State == "present" || l.State == "accepted") && l.ByteSize != nil && *l.ByteSize == size &&
		l.MIMEType != nil && *l.MIMEType == mime && l.SHA256 != nil && *l.SHA256 == sha
}

// PersonalGeneratedAudioService verifies complete provider bytes and produces a
// fixed original plus waveform. It never submits a provider call or a SQL job.
type PersonalGeneratedAudioService struct {
	prober   Prober
	renderer Renderer
	objects  PersonalGeneratedAudioObjects
	now      func() time.Time
}

// NewPersonalGeneratedAudioService reuses real media tools and ledger-backed I/O.
func NewPersonalGeneratedAudioService(prober Prober, renderer Renderer, objects PersonalGeneratedAudioObjects, now func() time.Time) *PersonalGeneratedAudioService {
	return &PersonalGeneratedAudioService{prober: prober, renderer: renderer, objects: objects, now: now}
}

// Prepare is safe to retry with the same frozen asset and receipt. The caller
// keeps its input file; verified private copies and rendered temporaries close here.
func (s *PersonalGeneratedAudioService) Prepare(ctx context.Context, r PersonalGeneratedAudioRequest, file *Downloaded) (PreparedPersonalGeneratedAudio, error) {
	if s == nil || s.prober == nil || s.renderer == nil || s.objects == nil || s.now == nil {
		return PreparedPersonalGeneratedAudio{}, ErrUnavailable
	}
	if r.Validate() != nil || r.Intent.State != "open" || s.now().IsZero() || r.CreatedAt.After(s.now()) ||
		file == nil || file.Size != r.ByteSize || file.SHA256 != r.SHA256 || file.MIMEType != r.MIMEType {
		return PreparedPersonalGeneratedAudio{}, ErrInvalidGeneratedAudio
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	original, err := copyGeneratedAudioFile(ctx, file, r.FileName)
	if err != nil {
		return PreparedPersonalGeneratedAudio{}, err
	}
	defer func() { _ = original.Close() }()
	probe, err := s.prober.Probe(ctx, original)
	if err != nil {
		return PreparedPersonalGeneratedAudio{}, fmt.Errorf("probe generated personal audio: %w", err)
	}
	if !validGeneratedAudioProbe(probe) {
		return PreparedPersonalGeneratedAudio{}, ErrInvalidGeneratedAudio
	}
	rendered, err := s.renderer.Render(ctx, original, probe, "16:9")
	defer func() {
		for _, file := range rendered {
			_ = file.Result.Close()
		}
	}()
	if err != nil {
		return PreparedPersonalGeneratedAudio{}, fmt.Errorf("render generated audio waveform: %w", err)
	}
	if !completeUploadRendered(domain.KindAudio, rendered) || rendered[0].Ext != "png" || rendered[0].Result.MIMEType != "image/png" || rendered[0].Width != 1280 || rendered[0].Height != 256 {
		return PreparedPersonalGeneratedAudio{}, ErrInvalidGeneratedAudio
	}
	waveform, err := copyGeneratedAudioFile(ctx, rendered[0].Result, "waveform.png")
	if err != nil {
		return PreparedPersonalGeneratedAudio{}, err
	}
	defer func() { _ = waveform.Close() }()
	image, err := s.prober.Probe(ctx, waveform)
	if err != nil || image.Kind != domain.KindImage || image.Extension != "png" || image.Width == nil || *image.Width != 1280 || image.Height == nil || *image.Height != 256 {
		return PreparedPersonalGeneratedAudio{}, errors.Join(ErrInvalidGeneratedAudio, err)
	}
	return s.writePreparedAudio(ctx, r, original, probe, waveform)
}

func validGeneratedAudioProbe(p ProbeResult) bool {
	return p.Kind == domain.KindAudio && p.Extension == "mp3" && p.Codec != nil && *p.Codec == "mp3" &&
		p.DurationMS != nil && *p.DurationMS > 0 && p.AudioChannels != nil && *p.AudioChannels > 0 &&
		p.Width == nil && p.Height == nil && p.FPS == nil
}

func copyGeneratedAudioFile(ctx context.Context, input *Downloaded, name string) (*Downloaded, error) {
	if input == nil || input.File == nil {
		return nil, ErrInvalidGeneratedAudio
	}
	if _, err := input.File.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	copied, err := ReadUpload(ctx, input.File, name)
	if err != nil {
		return nil, err
	}
	if copied.Size != input.Size || copied.SHA256 != input.SHA256 || copied.MIMEType != input.MIMEType {
		_ = copied.Close()
		return nil, ErrObjectMismatch
	}
	return copied, nil
}

func (s *PersonalGeneratedAudioService) writePreparedAudio(ctx context.Context, r PersonalGeneratedAudioRequest, file *Downloaded, probe ProbeResult, waveform *Downloaded) (PreparedPersonalGeneratedAudio, error) {
	key := r.objectKey()
	originalLocation, err := s.objects.CreateGeneratedAudioObject(ctx, r.Intent, key, file)
	if err != nil {
		return PreparedPersonalGeneratedAudio{}, err
	}
	if !generatedAudioLocation(r.Intent, &originalLocation, key, file.Size, file.MIMEType, file.SHA256) {
		return PreparedPersonalGeneratedAudio{}, ErrObjectMismatch
	}
	waveKey := strings.TrimSuffix(key, ".mp3") + "/waveform.png"
	waveLocation, err := s.objects.CreateGeneratedAudioObject(ctx, r.Intent, waveKey, waveform)
	if err != nil {
		return PreparedPersonalGeneratedAudio{}, err
	}
	proof, err := json.Marshal(generatedAudioEvidence(r))
	if err != nil {
		return PreparedPersonalGeneratedAudio{}, err
	}
	createdAt, width, height, size := r.CreatedAt.UTC(), int32(1280), int32(256), waveform.Size
	owner, sha := r.Personal, r.SHA256
	prepared := PreparedPersonalGeneratedAudio{Request: r,
		Asset: domain.MediaAsset{ID: r.AssetID, Personal: &owner, Kind: domain.KindAudio, Origin: domain.OriginSystem, Status: domain.StatusReady,
			ObjectKey: key, ObjectLocation: &originalLocation, FileName: r.FileName, MimeType: r.MIMEType, ByteSize: r.ByteSize, SHA256: &sha,
			DurationMS: probe.DurationMS, AudioChannels: probe.AudioChannels, Codec: probe.Codec, ModerationStatus: domain.ModerationPassed,
			ModerationDetail: proof, AIGCMarked: true, Revision: 1, CreateTime: createdAt, UpdateTime: createdAt},
		Renditions: []domain.Rendition{{ID: generatedAudioRenditionID(r.AssetID), MediaAssetID: r.AssetID, Kind: domain.RenditionWaveform,
			ObjectKey: waveKey, ObjectLocation: &waveLocation, Width: &width, Height: &height, ByteSize: &size, CreateTime: createdAt, UpdateTime: createdAt}}}
	if err := prepared.Validate(); err != nil {
		return PreparedPersonalGeneratedAudio{}, err
	}
	return prepared, nil
}
