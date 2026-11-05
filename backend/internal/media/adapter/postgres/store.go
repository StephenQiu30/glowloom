// Package postgres persists project-scoped media metadata.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	workspacedomain "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/domain"
)

var (
	// ErrNotFound hides missing and out-of-project media records.
	ErrNotFound = application.ErrNotFound
	// ErrUnavailable means the store has no database handle.
	ErrUnavailable = application.ErrUnavailable
	// ErrProjectStateConflict means an inactive project cannot accept new media.
	ErrProjectStateConflict = workspacedomain.ErrProjectStateConflict
)

// Store keeps the database handle injected by the composition root.
type Store struct {
	db *gorm.DB
	scopeAuthority
}

// NewStore creates a media store using the caller's database handle.
func NewStore(db *gorm.DB) *Store { return &Store{db: db} }

// NewScopedStore injects transaction-bound content authority.
func NewScopedStore(db *gorm.DB, scopes ScopeAccessFactory) *Store {
	return &Store{db: db, scopeAuthority: scopeAuthority{scopes: scopes}}
}

// CreateAsset records metadata for a new upload or generated result. Object
// transfer and ingest are separate steps; this method does not mark it ready.
func (s *Store) CreateAsset(ctx context.Context, actor identityapp.Principal, asset domain.MediaAsset) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	if err := asset.Validate(); err != nil {
		return fmt.Errorf("validate media asset: %w", err)
	}
	if asset.IsDelete || asset.Revision != 1 ||
		(asset.Status != domain.StatusUploading && asset.Status != domain.StatusProcessing) {
		return domain.ErrInvalidMediaAsset
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if err := s.requireProject(tx, actor, asset.ProjectID, true); err != nil {
			return err
		}
		record := mediaAssetRecordFromDomain(asset)
		if err := tx.Create(&record).Error; err != nil {
			return fmt.Errorf("insert media asset: %w", err)
		}
		return nil
	})
}

// FindAsset returns one visible asset after rechecking the current actor and
// project. A caller cannot discover another project's asset by its ID.
func (s *Store) FindAsset(ctx context.Context, actor identityapp.Principal, projectID, assetID uuid.UUID) (domain.MediaAsset, error) {
	return s.findAsset(ctx, actor, projectID, assetID, true)
}

// findAsset keeps the original current-facts reader available only to owning
// consumers of already frozen evidence. Ordinary public reads require visibility.
func (s *Store) findAsset(ctx context.Context, actor identityapp.Principal, projectID, assetID uuid.UUID, newBinding bool) (domain.MediaAsset, error) {
	if s == nil || s.db == nil {
		return domain.MediaAsset{}, ErrUnavailable
	}
	if projectID == uuid.Nil || assetID == uuid.Nil {
		return domain.MediaAsset{}, ErrNotFound
	}
	var row assetRow
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if newBinding {
			library, err := s.projectMediaLibrary(tx, actor, projectID)
			if err != nil {
				return err
			}
			if err := requireNewMediaVisible(tx, library.ID, []uuid.UUID{assetID}); err != nil {
				return err
			}
		} else if err := s.requireProject(tx, actor, projectID, false); err != nil {
			return err
		}
		result := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where(map[string]any{"id": assetID, "project_id": projectID, "is_delete": false}).Take(&row)
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		if result.Error != nil {
			return fmt.Errorf("read media asset: %w", result.Error)
		}
		return row.loadLocation(tx)
	})
	if err != nil {
		return domain.MediaAsset{}, fmt.Errorf("find media asset: %w", err)
	}
	asset := row.domain()
	if err := asset.Validate(); err != nil {
		return domain.MediaAsset{}, fmt.Errorf("validate stored media asset: %w", err)
	}
	return asset, nil
}

// AddRendition appends a derived object only after checking the parent asset's
// project scope. The parent foreign key carries that scope in the database.
func (s *Store) AddRendition(ctx context.Context, actor identityapp.Principal, projectID uuid.UUID, rendition domain.Rendition) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	if projectID == uuid.Nil {
		return ErrNotFound
	}
	if err := rendition.Validate(); err != nil {
		return fmt.Errorf("validate rendition: %w", err)
	}
	if rendition.IsDelete {
		return domain.ErrInvalidRendition
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if err := s.requireProject(tx, actor, projectID, true); err != nil {
			return err
		}
		parentKey, err := s.visibleAssetKey(tx, actor, projectID, rendition.MediaAssetID)
		if err != nil {
			return err
		}
		prefix := strings.TrimSuffix(parentKey, path.Ext(parentKey)) + "/"
		name := strings.TrimPrefix(rendition.ObjectKey, prefix)
		if !strings.HasPrefix(rendition.ObjectKey, prefix) ||
			strings.Contains(name, "/") || path.Ext(name) == "" ||
			strings.TrimSuffix(name, path.Ext(name)) != string(rendition.Kind) {
			return domain.ErrInvalidRendition
		}
		record := mediaRenditionRecordFromDomain(rendition)
		if err := tx.Create(&record).Error; err != nil {
			return fmt.Errorf("insert rendition: %w", err)
		}
		return nil
	})
}

// FindRenditions returns the visible derived objects of a scoped parent.
func (s *Store) FindRenditions(ctx context.Context, actor identityapp.Principal, projectID, assetID uuid.UUID) ([]domain.Rendition, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	if projectID == uuid.Nil || assetID == uuid.Nil {
		return nil, ErrNotFound
	}
	var rows []renditionRow
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if _, err := s.visibleAssetKey(tx, actor, projectID, assetID); err != nil {
			return err
		}
		if err := tx.Where("media_asset_id = ? AND NOT is_delete", assetID).Order("kind").Order("id").Find(&rows).Error; err != nil {
			return fmt.Errorf("read renditions: %w", err)
		}
		for i := range rows {
			if err := rows[i].loadLocation(tx); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("find renditions: %w", err)
	}
	result := make([]domain.Rendition, 0, len(rows))
	for _, row := range rows {
		r := row.domain()
		if err := r.Validate(); err != nil {
			return nil, fmt.Errorf("validate stored rendition: %w", err)
		}
		result = append(result, r)
	}
	return result, nil
}

func (s scopeAuthority) visibleAssetKey(tx *gorm.DB, actor identityapp.Principal, projectID, assetID uuid.UUID) (string, error) {
	if projectID == uuid.Nil || assetID == uuid.Nil {
		return "", ErrNotFound
	}
	library, err := s.projectMediaLibrary(tx, actor, projectID)
	if err != nil {
		return "", err
	}
	if err := requireNewMediaVisible(tx, library.ID, []uuid.UUID{assetID}); err != nil {
		return "", err
	}
	var row assetRow
	result := tx.Clauses(clause.Locking{Strength: "SHARE"}).Select("object_key").Where(map[string]any{"id": assetID, "project_id": projectID, "is_delete": false}).Take(&row)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return "", ErrNotFound
	}
	if result.Error != nil {
		return "", fmt.Errorf("check media asset scope: %w", result.Error)
	}
	return row.ObjectKey, nil
}

type assetRow struct {
	ObjectLocationID   *uuid.UUID `json:"object_location_id,omitempty"`
	location           *domain.ObjectLocation
	ID                 uuid.UUID  `json:"id"`
	ProjectID          uuid.UUID  `json:"project_id"`
	PersonalOrgID      *uuid.UUID `json:"personal_org_id"`
	PersonalActorID    *uuid.UUID `json:"personal_actor_id"`
	Kind               string     `json:"kind"`
	Origin             string     `json:"origin"`
	Status             string     `json:"status"`
	ObjectKey          string     `json:"object_key"`
	FileName           string     `json:"file_name"`
	MimeType           string     `json:"mime_type"`
	ByteSize           int64      `json:"byte_size"`
	SHA256             *string    `json:"sha256"`
	Width              *int32     `json:"width"`
	Height             *int32     `json:"height"`
	DurationMS         *int32     `json:"duration_ms"`
	FPS                *float64   `json:"fps"`
	AudioChannels      *int32     `json:"audio_channels"`
	Codec              *string    `json:"codec"`
	SourceOperationID  *uuid.UUID `json:"source_operation_id"`
	ProviderKey        *string    `json:"provider_key"`
	ModelKey           *string    `json:"model_key"`
	Region             *string    `json:"region"`
	ModerationStatus   string     `json:"moderation_status"`
	ModerationDetail   []byte     `json:"moderation_detail"`
	AIGCMarked         bool       `json:"aigc_marked"`
	ContainsRealPerson bool       `json:"contains_real_person"`
	ConsentRecordID    *uuid.UUID `json:"consent_record_id"`
	UploadID           *string    `json:"upload_id"`
	FailureReason      *string    `json:"failure_reason"`
	DeleteTime         *time.Time `json:"delete_time"`
	PurgeAfter         *time.Time `json:"purge_after"`
	Revision           int64      `json:"revision"`
	CreateTime         time.Time  `json:"create_time"`
	UpdateTime         time.Time  `json:"update_time"`
	IsDelete           bool       `json:"is_delete"`
}

func (assetRow) TableName() string { return "media.media_asset" }

func (r assetRow) domain() domain.MediaAsset {
	var personal *domain.PersonalOwnership
	if r.PersonalOrgID != nil || r.PersonalActorID != nil {
		personal = &domain.PersonalOwnership{}
		if r.PersonalOrgID != nil {
			personal.OrgID = *r.PersonalOrgID
		}
		if r.PersonalActorID != nil {
			personal.ActorID = *r.PersonalActorID
		}
	}
	return domain.MediaAsset{
		ID: r.ID, ProjectID: r.ProjectID, Personal: personal, Kind: domain.Kind(r.Kind),
		Origin: domain.Origin(r.Origin), Status: domain.Status(r.Status),
		ObjectKey: r.ObjectKey, ObjectLocation: rowLocation(r.ObjectLocationID, r.location), FileName: r.FileName, MimeType: r.MimeType,
		ByteSize: r.ByteSize, SHA256: r.SHA256, Width: r.Width, Height: r.Height,
		DurationMS: r.DurationMS, FPS: r.FPS, AudioChannels: r.AudioChannels,
		Codec: r.Codec, SourceOperationID: r.SourceOperationID,
		ProviderKey: r.ProviderKey, ModelKey: r.ModelKey, Region: r.Region,
		ModerationStatus: domain.ModerationStatus(r.ModerationStatus),
		ModerationDetail: json.RawMessage(r.ModerationDetail),
		AIGCMarked:       r.AIGCMarked, ContainsRealPerson: r.ContainsRealPerson,
		ConsentRecordID: r.ConsentRecordID, UploadID: r.UploadID,
		FailureReason: r.FailureReason, DeleteTime: r.DeleteTime,
		PurgeAfter: r.PurgeAfter, Revision: r.Revision,
		CreateTime: r.CreateTime, UpdateTime: r.UpdateTime, IsDelete: r.IsDelete,
	}
}

type renditionRow struct {
	ObjectLocationID *uuid.UUID
	location         *domain.ObjectLocation
	ID               uuid.UUID
	MediaAssetID     uuid.UUID
	Kind             string
	ObjectKey        string
	Width            *int32
	Height           *int32
	ByteSize         *int64
	CreateTime       time.Time
	UpdateTime       time.Time
	IsDelete         bool
}

func (renditionRow) TableName() string { return "media.rendition" }

func (r renditionRow) domain() domain.Rendition {
	return domain.Rendition{
		ID: r.ID, MediaAssetID: r.MediaAssetID,
		Kind: domain.RenditionKind(r.Kind), ObjectKey: r.ObjectKey, ObjectLocation: rowLocation(r.ObjectLocationID, r.location),
		Width: r.Width, Height: r.Height, ByteSize: r.ByteSize,
		CreateTime: r.CreateTime, UpdateTime: r.UpdateTime, IsDelete: r.IsDelete,
	}
}

// ListReadyAssets uses an ID keyset after rechecking current actor and project visibility.
func (s *Store) ListReadyAssets(ctx context.Context, actor identityapp.Principal, projectID uuid.UUID, kind string, after uuid.UUID, limit int) ([]domain.MediaAsset, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	var rows []assetRow
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if err := s.requireProject(tx, actor, projectID, false); err != nil {
			return err
		}
		library, err := readLibrary(tx, actor, domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &projectID}, false)
		if err != nil {
			return err
		}
		query := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where(map[string]any{"project_id": projectID, "is_delete": false, "status": domain.StatusReady, "moderation_status": domain.ModerationPassed})
		hiddenLibraryItems := tx.Model(&libraryItemRow{}).Select("1").Where("library_id = ? AND id = media.media_asset.id AND (catalog_state <> ? OR purged_at IS NOT NULL)", library.ID, domain.LibraryItemStateActive)
		query = query.Where("NOT EXISTS (?)", hiddenLibraryItems)
		activePurgeStatuses := []domain.PurgeItemStatus{domain.PurgeItemStatusQueued, domain.PurgeItemStatusRunning, domain.PurgeItemStatusNeedsReconciliation, domain.PurgeItemStatusSucceeded}
		purgeItems := tx.Model(&purgeItemRow{}).Select("1").Where("item_id = media.media_asset.id AND status IN ?", activePurgeStatuses)
		query = query.Where("NOT EXISTS (?)", purgeItems)
		if kind != "" {
			query = query.Where("kind = ?", kind)
		} else {
			query = query.Where("kind IN ?", []domain.Kind{domain.KindImage, domain.KindVideo, domain.KindAudio, domain.KindModel})
		}
		if after != uuid.Nil {
			query = query.Where("id < ?", after)
		}
		if err := query.Order("id DESC").Limit(limit).Find(&rows).Error; err != nil {
			return err
		}
		for i := range rows {
			if err := rows[i].loadLocation(tx); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	assets := make([]domain.MediaAsset, 0, len(rows))
	for _, r := range rows {
		a := r.domain()
		if err := a.Validate(); err != nil {
			return nil, fmt.Errorf("validate available media: %w", err)
		}
		assets = append(assets, a)
	}
	return assets, nil
}
