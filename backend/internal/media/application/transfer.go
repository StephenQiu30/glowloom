package application

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

// LibraryWorkGuards is consulted only for new transfer/purge admission. Publish
// checks its own durable fence instead of blocking itself through this reader.
type LibraryWorkGuards interface {
	HasInflightProjectWork(context.Context, identityapp.Principal, uuid.UUID) (bool, error)
}

// TransferInput freezes opposite scopes and every displayed optimistic revision.
type TransferInput struct {
	Source                  domain.LibraryScope   `json:"source"`
	Target                  domain.LibraryScope   `json:"target"`
	Items                   []LibraryItemRevision `json:"items"`
	ExpectedSourceRevision  int64                 `json:"expected_source_revision"`
	ExpectedTargetRevision  int64                 `json:"expected_target_revision"`
	ExpectedProjectRevision int64                 `json:"expected_project_revision"`
	TargetFolderID          *uuid.UUID            `json:"target_folder_id" extensions:"x-nullable"`
	ExpectedFolderRevision  int64                 `json:"expected_folder_revision"`
	Key                     uuid.UUID             `json:"-"`
}

// Validate closes current scope, batch bounds and revision requirements.
func (in TransferInput) Validate() error {
	if in.Source.Validate() != nil || in.Target.Validate() != nil || in.Source.Kind == in.Target.Kind || in.Key == uuid.Nil ||
		len(in.Items) < 1 || len(in.Items) > 200 || in.ExpectedSourceRevision < 0 || in.ExpectedSourceRevision > math.MaxInt32 ||
		in.ExpectedTargetRevision < 0 || in.ExpectedTargetRevision > math.MaxInt32 || in.ExpectedProjectRevision < 1 || in.ExpectedProjectRevision > math.MaxInt32 || in.ExpectedFolderRevision > math.MaxInt32 ||
		(in.TargetFolderID == nil && in.ExpectedFolderRevision != 0) || (in.TargetFolderID != nil && (*in.TargetFolderID == uuid.Nil || in.ExpectedFolderRevision < 1)) {
		return domain.ErrInvalidLibrary
	}
	seen := make(map[uuid.UUID]bool, len(in.Items))
	for _, item := range in.Items {
		if item.ID == uuid.Nil || item.Revision < 0 || item.Revision > math.MaxInt32 || seen[item.ID] {
			return domain.ErrInvalidLibrary
		}
		seen[item.ID] = true
	}
	return nil
}

// FrozenTransferItem contains only media-owner facts; it never crosses HTTP.
type FrozenTransferItem struct {
	SourceItem       domain.LibraryItem  `json:"source_item"`
	SourceAsset      *domain.MediaAsset  `json:"source_asset,omitempty"`
	SourceRenditions []domain.Rendition  `json:"source_renditions"`
	TargetItem       domain.LibraryItem  `json:"target_item"`
	TargetAsset      *domain.MediaAsset  `json:"target_asset,omitempty"`
	TargetRenditions []domain.Rendition  `json:"target_renditions"`
	Objects          []ProjectCopyObject `json:"objects"`
}

// PrepareTransferItem creates deterministic independent identities for one
// selected item. Source notes/favorites never enter the shared project scope.
func PrepareTransferItem(job, org, actor uuid.UUID, target domain.LibraryScope, folder *uuid.UUID, source domain.LibraryItem, file *LibraryMediaFile, now time.Time) (FrozenTransferItem, error) {
	checkedSource := source
	if checkedSource.Revision == 0 && checkedSource.AssetID != nil {
		checkedSource.Revision = 1 // A real legacy binary has no metadata row yet.
	}
	if job == uuid.Nil || org == uuid.Nil || actor == uuid.Nil || target.Validate() != nil || now.IsZero() || checkedSource.Validate() != nil || source.State != "active" || folder != nil && *folder == uuid.Nil {
		return FrozenTransferItem{}, domain.ErrInvalidLibrary
	}
	encoded, err := json.Marshal(source)
	if err != nil {
		return FrozenTransferItem{}, err
	}
	var item domain.LibraryItem
	if err := json.Unmarshal(encoded, &item); err != nil {
		return FrozenTransferItem{}, err
	}
	var frozenSource domain.LibraryItem
	if err := json.Unmarshal(encoded, &frozenSource); err != nil {
		return FrozenTransferItem{}, err
	}
	if folder != nil {
		id := *folder
		folder = &id
	}
	now = now.UTC().Truncate(time.Microsecond)
	lib, _ := target.Identity(org, actor)
	item.ID = uuid.NewSHA1(job, []byte("transfer-item/"+source.ID.String()))
	item.LibraryID, item.FolderID, item.Favorite, item.State, item.TrashedAt = lib, folder, false, "active", nil
	item.Revision, item.CreatedAt, item.UpdatedAt = 1, now, now
	if target.Kind == domain.LibraryProject {
		item.Note = ""
	}
	result := FrozenTransferItem{SourceItem: frozenSource, TargetItem: item, SourceRenditions: []domain.Rendition{}, TargetRenditions: []domain.Rendition{}, Objects: []ProjectCopyObject{}}
	if source.AssetID == nil {
		if file != nil {
			return FrozenTransferItem{}, domain.ErrInvalidLibrary
		}
		return result, nil
	}
	if file == nil {
		return FrozenTransferItem{}, ErrNotFound
	}
	original := file.Asset
	if original.ContainsRealPerson || original.ConsentRecordID != nil {
		return FrozenTransferItem{}, ErrProjectCopyConsentUnavailable
	}
	if original.ID != *source.AssetID || original.Validate() != nil || original.IsDelete || original.Status != domain.StatusReady || original.ModerationStatus != domain.ModerationPassed || original.SHA256 == nil || !copySHA(original.SHA256) || original.ByteSize < 1 || original.ByteSize > libraryOriginalLimit(original.Kind) || len(file.Renditions) > 16 {
		return FrozenTransferItem{}, ErrProjectCopyMediaUnavailable
	}
	encoded, err = json.Marshal(file)
	if err != nil || len(encoded) > 1<<20 {
		return FrozenTransferItem{}, ErrProjectCopyMediaUnavailable
	}
	var clone LibraryMediaFile
	if err := json.Unmarshal(encoded, &clone); err != nil {
		return FrozenTransferItem{}, err
	}
	var sourceClone LibraryMediaFile
	if err := json.Unmarshal(encoded, &sourceClone); err != nil {
		return FrozenTransferItem{}, err
	}
	a := clone.Asset
	a.ID = result.TargetItem.ID
	a.ProjectID, a.Personal = uuid.Nil, nil
	if target.Kind == domain.LibraryProject {
		a.ProjectID = *target.ProjectID
	} else {
		a.Personal = &domain.PersonalOwnership{OrgID: org, ActorID: actor}
	}
	a.SourceOperationID, a.UploadID, a.ProviderKey, a.ModelKey, a.Region = nil, nil, nil, nil, nil
	if a.Origin == domain.OriginGenerated {
		a.Origin = domain.OriginSystem
	}
	a.Revision, a.CreateTime, a.UpdateTime = 1, now, now
	owner := "projects/" + a.ProjectID.String()
	if a.Personal != nil {
		owner = "personal/" + org.String() + "/" + actor.String()
	}
	a.ObjectKey = fmt.Sprintf("%s/%s/%04d/%02d/%s%s", owner, a.Kind, now.Year(), now.Month(), a.ID, path.Ext(original.ObjectKey))
	if a.Validate() != nil {
		return FrozenTransferItem{}, ErrProjectCopyMediaUnavailable
	}
	result.SourceAsset = &sourceClone.Asset
	result.SourceRenditions = sourceClone.Renditions
	result.TargetAsset = &a
	result.TargetItem.AssetID = &a.ID
	result.Objects = append(result.Objects, ProjectCopyObject{TargetAssetID: a.ID, SourceObjectKey: original.ObjectKey, TargetObjectKey: a.ObjectKey, ByteSize: &a.ByteSize, ContentType: a.MimeType, SHA256: a.SHA256, Status: "pending"})
	seen := make(map[domain.RenditionKind]bool)
	for _, current := range clone.Renditions {
		prefix := strings.TrimSuffix(original.ObjectKey, path.Ext(original.ObjectKey)) + "/"
		ext := path.Ext(current.ObjectKey)
		mime := renditionCopyMIME(ext)
		if current.Validate() != nil || current.MediaAssetID != original.ID || current.IsDelete || mime == "" || seen[current.Kind] || current.ObjectKey != prefix+string(current.Kind)+ext || current.ByteSize != nil && (*current.ByteSize < 1 || *current.ByteSize > 2<<30) {
			return FrozenTransferItem{}, ErrProjectCopyMediaUnavailable
		}
		seen[current.Kind] = true
		from := current.ObjectKey
		current.ID = uuid.NewSHA1(job, []byte("transfer-rendition/"+current.ID.String()))
		current.MediaAssetID = a.ID
		current.ObjectKey = strings.TrimSuffix(a.ObjectKey, path.Ext(a.ObjectKey)) + "/" + string(current.Kind) + ext
		current.CreateTime, current.UpdateTime = now, now
		result.TargetRenditions = append(result.TargetRenditions, current)
		result.Objects = append(result.Objects, ProjectCopyObject{TargetAssetID: a.ID, RenditionKind: string(current.Kind), SourceObjectKey: from, TargetObjectKey: current.ObjectKey, ByteSize: current.ByteSize, ContentType: mime, Status: "pending"})
	}
	return result, nil
}

// TransferRepository separates public commands from private fenced execution.
type TransferRepository interface {
	CreateTransfer(context.Context, identityapp.Principal, TransferInput) (domain.TransferJob, error)
	GetTransfer(context.Context, identityapp.Principal, uuid.UUID) (domain.TransferJob, error)
	ListTransfers(context.Context, identityapp.Principal, domain.LibraryScope, int, int) ([]domain.TransferJob, error)
	ControlTransfer(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID, int64, string) (domain.TransferJob, error)
}

// TransferWorkID is immutable orchestration identity; execution is supplied by
// the real workflow and a separate SQL fence prevents an expired worker write.
type TransferWorkID struct {
	JobID       uuid.UUID `json:"job_id"`
	Attempt     int       `json:"attempt"`
	ExecutionID string    `json:"execution_id"`
	Reconcile   bool      `json:"reconcile"`
}

// TransferDelivery proves one permanent command and its actual outbox message.
type TransferDelivery struct {
	TransferWorkID
	EventID   uuid.UUID `json:"event_id"`
	RequestID uuid.UUID `json:"request_id"`
	ActorID   uuid.UUID `json:"actor_id"`
	OrgID     uuid.UUID `json:"org_id"`
	Action    string    `json:"action"`
}
