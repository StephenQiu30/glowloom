package application

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

var (
	// ErrProjectCopyMediaUnavailable rejects incomplete or unapproved source assets.
	ErrProjectCopyMediaUnavailable = errors.New("project copy media unavailable")
	// ErrProjectCopyConsentUnavailable blocks person media until an owner consent contract exists.
	ErrProjectCopyConsentUnavailable = errors.New("project copy consent context unavailable")
)

// ProjectCopyBinding is trusted workspace admission evidence, not public input.
type ProjectCopyBinding struct {
	JobID, OrgID, SourceProjectID, TargetProjectID uuid.UUID
}

// Validate checks that content moves between distinct projects in one organization.
func (b ProjectCopyBinding) Validate() error {
	if b.JobID == uuid.Nil || b.OrgID == uuid.Nil || b.SourceProjectID == uuid.Nil || b.TargetProjectID == uuid.Nil || b.SourceProjectID == b.TargetProjectID {
		return ErrProjectCopyMediaUnavailable
	}
	return nil
}

// ProjectCopySourceAsset includes every live rendition owned by a source asset.
type ProjectCopySourceAsset struct {
	Asset           domain.MediaAsset
	Renditions      []domain.Rendition
	RetainedHistory *RetainedHistoryProof `json:"retained_history,omitempty"`
}

// RetainedHistoryProof preserves exact soft-deletion facts. Empty DeclaredBy
// keeps historical Script document manifests byte-compatible; library marks
// binary IDs declared by the complete owning project catalog.
type RetainedHistoryProof struct {
	AssetID    uuid.UUID `json:"asset_id"`
	Revision   int64     `json:"revision"`
	DeletedAt  time.Time `json:"deleted_at"`
	PurgeAfter time.Time `json:"purge_after"`
	DeclaredBy string    `json:"declared_by,omitempty"`
}

func validRetainedHistory(a domain.MediaAsset, p *RetainedHistoryProof) bool {
	if !a.IsDelete {
		return p == nil
	}
	if p == nil || p.DeclaredBy != "" && p.DeclaredBy != "library" || p.DeclaredBy == "" && a.Kind != domain.KindDocument {
		return false
	}
	return a.DeleteTime != nil && a.PurgeAfter != nil &&
		p.AssetID == a.ID && p.Revision == a.Revision && p.DeletedAt.Equal(*a.DeleteTime) && p.PurgeAfter.Equal(*a.PurgeAfter)
}

// ProjectCopyObject binds one exact source object to one unpublished target object.
// Size and SHA may be absent only until bounded reading establishes a durable intent.
type ProjectCopyObject struct {
	TargetAssetID   uuid.UUID
	RenditionKind   string
	SourceObjectKey string
	TargetObjectKey string
	ByteSize        *int64
	ContentType     string
	SHA256          *string
	Status          string
	SourceVerified  bool
	WriteStarted    bool
}

// ProjectMediaCopy is private content metadata plus each required object transfer.
type ProjectMediaCopy struct {
	Assets  []ProjectCopySourceAsset
	Objects []ProjectCopyObject
	Library *ProjectLibraryCopy `json:"library,omitempty"`
}

// ProjectCopySnapshot identifies one immutable media-owned source manifest.
type ProjectCopySnapshot struct {
	ID             uuid.UUID
	ManifestSHA256 string
	Assets         int
	Renditions     int
	AssetMapping   map[uuid.UUID]uuid.UUID
}

// ProjectCopyReceipt proves that all frozen bytes and metadata were registered.
type ProjectCopyReceipt struct {
	ManifestSHA256 string
	ContentSHA256  string
	Assets         int
	Renditions     int
}

func copySHA(value *string) bool {
	if value == nil {
		return true
	}
	bytes, err := hex.DecodeString(*value)
	return err == nil && len(bytes) == 32 && *value == strings.ToLower(*value)
}

func renditionCopyMIME(extension string) string {
	switch extension {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".mp4":
		return "video/mp4"
	case ".json":
		return "application/json"
	default:
		return ""
	}
}

// PrepareProjectMediaCopy clones approved source facts and creates stable private keys.
// It preserves the AIGC flag and exact moderation evidence but clears execution history.
func PrepareProjectMediaCopy(binding ProjectCopyBinding, sources []ProjectCopySourceAsset, now time.Time) (ProjectMediaCopy, error) {
	if binding.Validate() != nil || now.IsZero() || len(sources) > 4096 {
		return ProjectMediaCopy{}, ErrProjectCopyMediaUnavailable
	}
	raw, err := json.Marshal(sources)
	if err != nil || len(raw) > 32<<20 {
		return ProjectMediaCopy{}, ErrProjectCopyMediaUnavailable
	}
	result := ProjectMediaCopy{Objects: make([]ProjectCopyObject, 0)}
	if err := json.Unmarshal(raw, &result.Assets); err != nil {
		return ProjectMediaCopy{}, err
	}
	if result.Assets == nil {
		result.Assets = []ProjectCopySourceAsset{}
	}
	seen := make(map[uuid.UUID]bool, len(sources))
	renditionIDs := make(map[uuid.UUID]bool)
	now = now.UTC().Truncate(time.Microsecond)
	for i := range result.Assets {
		item := &result.Assets[i]
		original := item.Asset
		if original.ContainsRealPerson || original.ConsentRecordID != nil {
			return ProjectMediaCopy{}, ErrProjectCopyConsentUnavailable
		}
		if original.Validate() != nil || original.ProjectID != binding.SourceProjectID || !validRetainedHistory(original, item.RetainedHistory) || original.Status != domain.StatusReady || original.ModerationStatus != domain.ModerationPassed || original.ByteSize < 1 || original.ByteSize > 2<<30 || !copySHA(original.SHA256) || seen[original.ID] {
			return ProjectMediaCopy{}, ErrProjectCopyMediaUnavailable
		}
		seen[original.ID] = true
		a := &item.Asset
		a.ID = uuid.NewSHA1(binding.JobID, []byte("asset/"+original.ID.String()))
		a.ProjectID = binding.TargetProjectID
		a.IsDelete, a.DeleteTime, a.PurgeAfter = false, nil, nil
		a.ObjectKey = fmt.Sprintf("projects/%s/%s/%04d/%02d/%s%s", binding.TargetProjectID, a.Kind, now.Year(), now.Month(), a.ID, path.Ext(original.ObjectKey))
		a.SourceOperationID, a.ProviderKey, a.ModelKey, a.Region, a.UploadID = nil, nil, nil, nil, nil
		if a.Origin == domain.OriginGenerated {
			a.Origin = domain.OriginSystem
		}
		a.Revision, a.CreateTime, a.UpdateTime = 1, now, now
		if a.Validate() != nil {
			return ProjectMediaCopy{}, ErrProjectCopyMediaUnavailable
		}
		result.Objects = append(result.Objects, ProjectCopyObject{TargetAssetID: a.ID, SourceObjectKey: original.ObjectKey, TargetObjectKey: a.ObjectKey, ByteSize: &a.ByteSize, ContentType: a.MimeType, SHA256: a.SHA256, Status: "pending"})
		kinds := make(map[domain.RenditionKind]bool)
		for j := range item.Renditions {
			r := &item.Renditions[j]
			source := *r
			prefix := strings.TrimSuffix(original.ObjectKey, path.Ext(original.ObjectKey)) + "/"
			ext := path.Ext(source.ObjectKey)
			mimeType := renditionCopyMIME(ext)
			if source.Validate() != nil || source.MediaAssetID != original.ID || source.IsDelete || !strings.HasPrefix(source.ObjectKey, prefix) || strings.TrimPrefix(source.ObjectKey, prefix) != string(source.Kind)+ext || mimeType == "" || kinds[source.Kind] || renditionIDs[source.ID] || source.ByteSize != nil && (*source.ByteSize < 1 || *source.ByteSize > 2<<30) {
				return ProjectMediaCopy{}, ErrProjectCopyMediaUnavailable
			}
			kinds[source.Kind], renditionIDs[source.ID] = true, true
			r.ID = uuid.NewSHA1(binding.JobID, []byte("rendition/"+source.ID.String()))
			r.MediaAssetID = a.ID
			r.ObjectKey = strings.TrimSuffix(a.ObjectKey, path.Ext(a.ObjectKey)) + "/" + string(r.Kind) + ext
			r.CreateTime, r.UpdateTime = now, now
			result.Objects = append(result.Objects, ProjectCopyObject{TargetAssetID: a.ID, RenditionKind: string(r.Kind), SourceObjectKey: source.ObjectKey, TargetObjectKey: r.ObjectKey, ByteSize: r.ByteSize, ContentType: mimeType, Status: "pending"})
		}
	}
	return result, nil
}
