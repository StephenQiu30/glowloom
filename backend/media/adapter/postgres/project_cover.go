package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/media/domain"
)

// ProjectCoverReader observes display facts without acquiring or releasing bytes.
type ProjectCoverReader struct{ db *gorm.DB }

// NewProjectCoverReader requires an independent restricted Media pool.
func NewProjectCoverReader(db *gorm.DB) *ProjectCoverReader { return &ProjectCoverReader{db: db} }

// ReadProjectCover requires Media's original blocked control. Creation separately
// verifies its four durable ACKs before requesting and freezing this observation.
func (s *ProjectCoverReader) ReadProjectCover(ctx context.Context, request application.ProjectCoverRequest) (application.ProjectCoverSnapshot, error) {
	var result application.ProjectCoverSnapshot
	if err := request.Validate(); err != nil {
		return result, err
	}
	if s == nil || s.db == nil || s.db.Statement == nil || s.db.Statement.ConnPool == nil {
		return result, application.ErrUnavailable
	}
	if _, nested := s.db.Statement.ConnPool.(gorm.TxCommitter); nested {
		return result, writefence.ErrTransaction
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		owner, err := pgfence.New(tx).Owner(ctx)
		if err != nil {
			return err
		}
		if owner != writefence.WriterMedia {
			return writefence.ErrInvalid
		}
		var head struct {
			Epoch          int64
			Blocked        bool
			CoordinationID uuid.UUID
		}
		if err := tx.Table("infra.writer_fence").Select("epoch", "blocked", "coordination_id").Where("service_owner = infra.runtime_owner() AND issuer = ? AND authority_kind = ? AND authority_id = ?", writefence.Creation, writefence.Project, request.ProjectID).Take(&head).Error; err != nil {
			return err
		}
		if head.Epoch != request.ProjectEpoch || !head.Blocked || head.CoordinationID != request.CoordinationID {
			return writefence.ErrFenced
		}
		value, err := readClosedProjectCover(tx, request)
		if err != nil {
			return err
		}
		result = value
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return application.ProjectCoverSnapshot{}, err
	}
	return result, nil
}

func readClosedProjectCover(tx *gorm.DB, request application.ProjectCoverRequest) (application.ProjectCoverSnapshot, error) {
	value := application.ProjectCoverSnapshot{Request: request}
	var asset assetRow
	err := tx.Where("id = ? AND project_id = ?", request.AssetID, request.ProjectID).Take(&asset).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return value, nil
	}
	if err != nil {
		return value, err
	}
	value.AssetRevision = asset.Revision
	if asset.IsDelete || asset.Kind != string(domain.KindImage) || asset.Status != string(domain.StatusReady) || asset.ModerationStatus != string(domain.ModerationPassed) {
		return value, nil
	}
	var catalog struct{ Mismatch, Hidden bool }
	if err := tx.Raw(`SELECT
		EXISTS(SELECT 1 FROM media.library WHERE kind='project' AND project_id=@project AND org_id<>@org) AS mismatch,
		EXISTS(SELECT 1 FROM media.library_item i JOIN media.library l ON l.id=i.library_id
			WHERE l.kind='project' AND l.project_id=@project AND i.id=@asset AND (i.catalog_state<>'active' OR i.purged_at IS NOT NULL)) AS hidden`,
		sql.Named("project", request.ProjectID), sql.Named("org", request.OrganizationID), sql.Named("asset", request.AssetID)).Scan(&catalog).Error; err != nil {
		return value, err
	}
	if catalog.Mismatch {
		return value, writefence.ErrConflict
	}
	if catalog.Hidden {
		return value, nil
	}
	if err := requireNoPurgeReservation(tx, []uuid.UUID{request.AssetID}); err != nil {
		if errors.Is(err, domain.ErrPurgeConflict) {
			return value, nil
		}
		return value, err
	}
	// A legacy row without a resolved route is unknown. Reading never selects a
	// current profile, signs a URL, probes an object or creates an acceptance.
	if asset.ObjectLocationID == nil {
		return value, application.ErrUnavailable
	}
	var row objectLocationRow
	if err := tx.Where("id = ? AND object_key = ?", *asset.ObjectLocationID, asset.ObjectKey).Take(&row).Error; err != nil {
		return value, err
	}
	location := row.domain()
	switch location.State {
	case domain.ObjectLocationDeleted, domain.ObjectLocationDeletePending:
		return value, nil
	case domain.ObjectLocationAccepted:
		asset.location = &location
	default:
		return value, application.ErrUnavailable
	}
	if err := asset.domain().Validate(); err != nil {
		return value, err
	}
	value.Available = true
	return value, nil
}
