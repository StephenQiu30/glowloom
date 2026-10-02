// Package domain owns media metadata and the lifecycle rules for referenceable assets.
package domain

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"path"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

var (
	// ErrInvalidMediaAsset means the asset's persisted facts violate its contract.
	ErrInvalidMediaAsset = errors.New("invalid media asset")
	// ErrMediaStateConflict means a lifecycle change is not allowed.
	ErrMediaStateConflict = errors.New("media state conflict")
	// ErrInvalidRendition means a derived object lacks a valid parent or metadata.
	ErrInvalidRendition = errors.New("invalid media rendition")
)

// Kind is the detected category of a media asset.
type Kind string

// Kind values match media.media_asset.kind.
const (
	KindImage    Kind = "image"
	KindVideo    Kind = "video"
	KindAudio    Kind = "audio"
	KindDocument Kind = "document"
	KindModel    Kind = "model"
)

// MaxModelBytes bounds a local self-contained GLB 2.0 model asset.
const MaxModelBytes int64 = 64 << 20

// Document MIME types describe original TXT and ordinary, non-macro OOXML files.
const (
	MIMEText               = "text/plain"
	MIMEDOCX               = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	MaxDocumentBytes int64 = 20 << 20
)

// Origin identifies how an asset entered the media library.
type Origin string

// Origin values match media.media_asset.origin.
const (
	OriginUpload    Origin = "upload"
	OriginGenerated Origin = "generated"
	OriginSystem    Origin = "system"
)

// Status is the persisted media ingest state. Deletion is tracked separately.
type Status string

// Status values match media.media_asset.status.
const (
	StatusUploading  Status = "uploading"
	StatusProcessing Status = "processing"
	StatusReady      Status = "ready"
	StatusRejected   Status = "rejected"
	StatusFailed     Status = "failed"
)

// ModerationStatus records the content review result.
type ModerationStatus string

// Moderation status values match media.media_asset.moderation_status.
const (
	ModerationPending  ModerationStatus = "pending"
	ModerationPassed   ModerationStatus = "passed"
	ModerationRejected ModerationStatus = "rejected"
	ModerationSkipped  ModerationStatus = "skipped"
)

// RenditionKind describes a derived object used for browsing or preview.
type RenditionKind string

// Rendition kinds match media.rendition.kind.
const (
	RenditionThumb256  RenditionKind = "thumb_256"
	RenditionThumb640  RenditionKind = "thumb_640"
	RenditionPoster    RenditionKind = "poster"
	RenditionProxy720p RenditionKind = "proxy_720p"
	RenditionWaveform  RenditionKind = "waveform"
)

const mediaRetention = 30 * 24 * time.Hour

// MediaAsset is project-scoped. Its nullable fields mirror media.media_asset.
// Consent status and references live in other contexts and must be checked by
// the application before allowing a reference or deletion.
type MediaAsset struct {
	ID                 uuid.UUID
	ProjectID          uuid.UUID
	Kind               Kind
	Origin             Origin
	Status             Status
	ObjectKey          string
	FileName           string
	MimeType           string
	ByteSize           int64
	SHA256             *string
	Width              *int32
	Height             *int32
	DurationMS         *int32
	FPS                *float64
	AudioChannels      *int32
	Codec              *string
	SourceOperationID  *uuid.UUID
	ProviderKey        *string
	ModelKey           *string
	Region             *string
	ModerationStatus   ModerationStatus
	ModerationDetail   json.RawMessage
	AIGCMarked         bool
	ContainsRealPerson bool
	ConsentRecordID    *uuid.UUID
	UploadID           *string
	FailureReason      *string
	DeleteTime         *time.Time
	PurgeAfter         *time.Time
	Revision           int64
	CreateTime         time.Time
	UpdateTime         time.Time
	IsDelete           bool
}

// Validate checks only facts available in the asset itself. It does not prove
// that linked consent remains active or that the operation belongs to the project.
func (a MediaAsset) Validate() error {
	if a.ID == uuid.Nil || a.ProjectID == uuid.Nil || !a.Kind.valid() ||
		!a.Origin.valid() || !a.Status.valid() || !a.ModerationStatus.valid() ||
		!validAssetObjectKey(a) || strings.TrimSpace(a.MimeType) == "" ||
		a.ByteSize < 0 || a.Revision < 1 || a.Revision > math.MaxInt32 ||
		a.CreateTime.IsZero() || a.UpdateTime.IsZero() || a.UpdateTime.Before(a.CreateTime) ||
		!validOptionalText(a.SHA256) || !validOptionalText(a.Codec) ||
		!validOptionalText(a.ProviderKey) || !validOptionalText(a.ModelKey) ||
		!validOptionalText(a.UploadID) || !validOptionalUUID(a.SourceOperationID) ||
		!validOptionalUUID(a.ConsentRecordID) || !validPositive(a.Width) ||
		!validPositive(a.Height) || !validPositive(a.DurationMS) ||
		!validPositive(a.AudioChannels) ||
		(a.FPS != nil && (*a.FPS <= 0 || math.IsNaN(*a.FPS) || math.IsInf(*a.FPS, 0))) ||
		(len(a.ModerationDetail) > 0 && !json.Valid(a.ModerationDetail)) ||
		(a.Region != nil && *a.Region != "domestic" && *a.Region != "overseas") {
		return ErrInvalidMediaAsset
	}
	if a.Origin == OriginGenerated {
		if a.SourceOperationID == nil || a.ProviderKey == nil || a.ModelKey == nil || a.Region == nil ||
			a.Status == StatusUploading || a.UploadID != nil {
			return ErrInvalidMediaAsset
		}
	} else if a.SourceOperationID != nil || a.ProviderKey != nil || a.ModelKey != nil || a.Region != nil ||
		(a.Origin != OriginUpload && (a.Status == StatusUploading || a.UploadID != nil)) {
		return ErrInvalidMediaAsset
	}
	if a.Kind == KindModel && (a.Origin != OriginUpload || a.MimeType != "model/gltf-binary" || a.ByteSize < 1 || a.ByteSize > MaxModelBytes ||
		a.Codec == nil || *a.Codec != "glb2" || a.Width != nil || a.Height != nil || a.DurationMS != nil || a.FPS != nil || a.AudioChannels != nil) {
		return ErrInvalidMediaAsset
	}
	if a.Kind == KindDocument && !a.validDocumentFacts() {
		return ErrInvalidMediaAsset
	}
	if a.Status == StatusReady && a.ModerationStatus != ModerationPassed ||
		a.Status == StatusRejected && a.ModerationStatus != ModerationRejected ||
		a.Status == StatusUploading && a.ModerationStatus != ModerationPending ||
		a.Status == StatusFailed && (a.FailureReason == nil || strings.TrimSpace(*a.FailureReason) == "") ||
		a.Status != StatusFailed && a.Status != StatusRejected && a.FailureReason != nil ||
		(a.FailureReason != nil && strings.TrimSpace(*a.FailureReason) == "") {
		return ErrInvalidMediaAsset
	}
	if a.IsDelete {
		if a.DeleteTime == nil || a.PurgeAfter == nil ||
			a.Status == StatusUploading || a.Status == StatusProcessing ||
			!a.PurgeAfter.Equal(a.DeleteTime.Add(mediaRetention)) ||
			a.DeleteTime.Before(a.CreateTime) || a.UpdateTime.Before(*a.DeleteTime) {
			return ErrInvalidMediaAsset
		}
	} else if a.DeleteTime != nil || a.PurgeAfter != nil {
		return ErrInvalidMediaAsset
	}
	return nil
}

func (a MediaAsset) validDocumentFacts() bool {
	if a.Origin == OriginGenerated || a.ByteSize < 1 || a.ByteSize > MaxDocumentBytes ||
		a.SHA256 == nil || a.Codec == nil || a.Width != nil || a.Height != nil ||
		a.DurationMS != nil || a.FPS != nil || a.AudioChannels != nil ||
		a.FileName == "" || strings.TrimSpace(a.FileName) != a.FileName || len(a.FileName) > 255 ||
		!utf8.ValidString(a.FileName) || strings.ContainsAny(a.FileName, "/\\") {
		return false
	}
	for _, r := range a.FileName {
		if unicode.IsControl(r) {
			return false
		}
	}
	hash, err := hex.DecodeString(*a.SHA256)
	if err != nil || len(hash) != 32 || *a.SHA256 != strings.ToLower(*a.SHA256) {
		return false
	}
	ext := strings.ToLower(path.Ext(a.FileName))
	objectExt := path.Ext(a.ObjectKey)
	return a.MimeType == MIMEText && *a.Codec == "txt" && ext == ".txt" && objectExt == ".txt" ||
		a.MimeType == MIMEDOCX && *a.Codec == "docx" && ext == ".docx" && objectExt == ".docx"
}

// CanReference applies the local gate. The application must additionally
// verify linked consent is active and that project access is authorized.
func (a MediaAsset) CanReference() bool {
	return a.Validate() == nil && !a.IsDelete && a.Status == StatusReady &&
		a.ModerationStatus == ModerationPassed &&
		(!a.ContainsRealPerson || a.ConsentRecordID != nil)
}

// CanTransitionTo checks the media state machine. Identical states are accepted
// for workflow replay; Transition only advances revision if facts change.
func (s Status) CanTransitionTo(next Status) error {
	if !s.valid() || !next.valid() {
		return fmt.Errorf("%w: %q -> %q", ErrMediaStateConflict, s, next)
	}
	if s == next {
		return nil
	}
	if s == StatusUploading && next == StatusProcessing ||
		s == StatusProcessing && (next == StatusReady || next == StatusRejected || next == StatusFailed) {
		return nil
	}
	return fmt.Errorf("%w: %q -> %q", ErrMediaStateConflict, s, next)
}

// Transition returns a new asset after a valid ingest or moderation update.
// Callers must persist it with an expected revision to prevent stale writes.
func (a MediaAsset) Transition(next Status, moderation ModerationStatus, failureReason string, now time.Time) (MediaAsset, error) {
	if err := a.Validate(); err != nil {
		return MediaAsset{}, err
	}
	if a.IsDelete || now.IsZero() || now.Before(a.UpdateTime) || a.Revision >= math.MaxInt32 {
		return MediaAsset{}, ErrMediaStateConflict
	}
	if err := a.Status.CanTransitionTo(next); err != nil {
		return MediaAsset{}, err
	}
	if a.Status == next && a.Status != StatusProcessing {
		if moderation == a.ModerationStatus && failureReason == optionalText(a.FailureReason) {
			return a, nil
		}
		return MediaAsset{}, ErrMediaStateConflict
	}
	if a.Status == StatusProcessing && next == StatusProcessing &&
		a.ModerationStatus != ModerationPending && moderation != a.ModerationStatus {
		return MediaAsset{}, ErrMediaStateConflict
	}
	changed := a
	changed.Status = next
	changed.ModerationStatus = moderation
	changed.FailureReason = nil
	if strings.TrimSpace(failureReason) != "" {
		changed.FailureReason = &failureReason
	}
	if err := changed.Validate(); err != nil {
		return MediaAsset{}, err
	}
	if changed.Status == a.Status && changed.ModerationStatus == a.ModerationStatus &&
		optionalText(changed.FailureReason) == optionalText(a.FailureReason) {
		return a, nil
	}
	changed.Revision++
	changed.UpdateTime = now.UTC()
	return changed, nil
}

// Delete schedules object and row cleanup after thirty days. A caller must
// check formal references in the same transaction before persisting deletion.
func (a MediaAsset) Delete(now time.Time) (MediaAsset, error) {
	if err := a.Validate(); err != nil {
		return MediaAsset{}, err
	}
	if a.IsDelete || a.Status == StatusUploading || a.Status == StatusProcessing ||
		now.IsZero() || now.Before(a.UpdateTime) || a.Revision >= math.MaxInt32 {
		return MediaAsset{}, ErrMediaStateConflict
	}
	deleted := a
	deletedAt := now.UTC()
	purgeAfter := deletedAt.Add(mediaRetention)
	deleted.IsDelete = true
	deleted.DeleteTime = &deletedAt
	deleted.PurgeAfter = &purgeAfter
	deleted.UpdateTime = deletedAt
	deleted.Revision++
	return deleted, nil
}

// Rendition is scoped through its parent MediaAsset, never by its own project ID.
type Rendition struct {
	ID           uuid.UUID
	MediaAssetID uuid.UUID
	Kind         RenditionKind
	ObjectKey    string
	Width        *int32
	Height       *int32
	ByteSize     *int64
	CreateTime   time.Time
	UpdateTime   time.Time
	IsDelete     bool
}

// Validate checks a rendition's local metadata and parent identity.
func (r Rendition) Validate() error {
	if r.ID == uuid.Nil || r.MediaAssetID == uuid.Nil || !r.Kind.valid() ||
		strings.TrimSpace(r.ObjectKey) == "" || !validPositive(r.Width) ||
		!validPositive(r.Height) || (r.ByteSize != nil && *r.ByteSize < 0) ||
		r.CreateTime.IsZero() || r.UpdateTime.IsZero() || r.UpdateTime.Before(r.CreateTime) {
		return ErrInvalidRendition
	}
	return nil
}

func (k Kind) valid() bool {
	switch k {
	case KindImage, KindVideo, KindAudio, KindDocument, KindModel:
		return true
	default:
		return false
	}
}

func (o Origin) valid() bool {
	switch o {
	case OriginUpload, OriginGenerated, OriginSystem:
		return true
	default:
		return false
	}
}

func (s Status) valid() bool {
	switch s {
	case StatusUploading, StatusProcessing, StatusReady, StatusRejected, StatusFailed:
		return true
	default:
		return false
	}
}

func (s ModerationStatus) valid() bool {
	switch s {
	case ModerationPending, ModerationPassed, ModerationRejected, ModerationSkipped:
		return true
	default:
		return false
	}
}

func (k RenditionKind) valid() bool {
	switch k {
	case RenditionThumb256, RenditionThumb640, RenditionPoster, RenditionProxy720p, RenditionWaveform:
		return true
	default:
		return false
	}
}

func validOptionalText(value *string) bool {
	return value == nil || strings.TrimSpace(*value) != ""
}

func validOptionalUUID(value *uuid.UUID) bool {
	return value == nil || *value != uuid.Nil
}

func validPositive(value *int32) bool {
	return value == nil || *value > 0
}

func optionalText(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// validAssetObjectKey binds an object to its project, kind, and asset identity.
// Exact path segments prevent a valid asset row from pointing at another
// project's object, a nested key, or a traversal-shaped key.
func validAssetObjectKey(a MediaAsset) bool {
	parts := strings.Split(a.ObjectKey, "/")
	if len(parts) != 6 || parts[0] != "projects" ||
		parts[1] != a.ProjectID.String() || parts[2] != string(a.Kind) ||
		len(parts[3]) != 4 || len(parts[4]) != 2 ||
		parts[4] < "01" || parts[4] > "12" {
		return false
	}
	for _, digit := range parts[3] + parts[4] {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	extension, ok := strings.CutPrefix(parts[5], a.ID.String()+".")
	if !ok || len(extension) == 0 || len(extension) > 16 {
		return false
	}
	for _, letter := range extension {
		switch {
		case letter >= 'a' && letter <= 'z',
			letter >= 'A' && letter <= 'Z',
			letter >= '0' && letter <= '9':
		default:
			return false
		}
	}
	return true
}
