package postgres

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/script/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/domain"
)

type copyObjectAction uint8

const (
	copyObjectBegin copyObjectAction = iota + 1
	copyObjectConfirm
	copyObjectBeginRemoval
	copyObjectConfirmRemoval
)

// WithObjectLocations binds storage work to the coordinator transaction.
func (s *ProjectCopyStore) WithObjectLocations(factory SourceObjectLocationFactory) *ProjectCopyStore {
	bound := *s
	bound.locations = factory
	return &bound
}

func bindCopyTargets(m *application.ProjectCopyManifest, facts []domain.ObjectFact) {
	byKey := make(map[string]domain.ObjectFact, len(facts))
	for i, f := range facts {
		m.Objects[i].Target.LocationID = f.LocationID
		byKey[f.Key] = f
	}
	set := func(f *domain.ObjectFact) { f.LocationID = byKey[f.Key].LocationID }
	for i := range m.Target.Sources {
		set(&m.Target.Sources[i].Original)
		set(&m.Target.Sources[i].Rich)
	}
	for i := range m.Target.Versions {
		set(&m.Target.Versions[i].Text)
		set(&m.Target.Versions[i].Rich)
	}
}

func hydrateCopyLocations(tx *gorm.DB, m *application.ProjectCopyManifest, snapshot application.ProjectCopySnapshot) error {
	var rows []scriptCopyObjectIntentRow
	if err := tx.Where("snapshot_id = ?", snapshot.ID).Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) != len(m.Objects) {
		return application.ErrObjectMismatch
	}
	targets := make([]domain.ObjectFact, len(m.Objects))
	byTargetKey := make(map[string]scriptCopyObjectIntentRow, len(rows))
	for _, row := range rows {
		byTargetKey[row.TargetKey] = row
	}
	for i := range m.Objects {
		row, found := byTargetKey[m.Objects[i].Target.Key]
		if !found || row.SourceKey != m.Objects[i].Source.Key || row.SourceLocationID == uuid.Nil || row.TargetLocationID == uuid.Nil {
			return application.ErrObjectMismatch
		}
		m.Objects[i].Source.LocationID, m.Objects[i].Target.LocationID = row.SourceLocationID, row.TargetLocationID
		targets[i] = m.Objects[i].Target
	}
	bindCopyTargets(m, targets)
	// Original sources/versions are immutable rows; their location columns are
	// themselves immutable once bound. Read by record ID, never by default key.
	current, err := readCopyHistory(tx, m.Binding.OrgID, m.Binding.SourceProjectID)
	if err != nil {
		return err
	}
	for i := range m.Source.Sources {
		for _, r := range current.Sources {
			if r.ID == m.Source.Sources[i].ID {
				m.Source.Sources[i].Original.LocationID, m.Source.Sources[i].Rich.LocationID = r.Original.LocationID, r.Rich.LocationID
			}
		}
	}
	for i := range m.Source.Versions {
		for _, r := range current.Versions {
			if r.ID == m.Source.Versions[i].ID {
				m.Source.Versions[i].Text.LocationID, m.Source.Versions[i].Rich.LocationID = r.Text.LocationID, r.Rich.LocationID
			}
		}
	}
	return nil
}

func (s *ProjectCopyStore) copyLocationAction(ctx context.Context, tx *gorm.DB, snapshot application.ProjectCopySnapshot, fact domain.ObjectFact, action copyObjectAction) (bool, error) {
	var row scriptCopySnapshotRow
	if err := tx.Model(&scriptCopySnapshotRow{}).Select("storage_intent_id").Where("id = ?", snapshot.ID).Take(&row).Error; err != nil {
		return false, err
	}
	if row.StorageIntentID == nil || *row.StorageIntentID == uuid.Nil || fact.LocationID == uuid.Nil {
		return false, application.ErrObjectMismatch
	}
	if s.locations == nil || s.locations(tx) == nil {
		return false, application.ErrUnavailable
	}
	p := s.locations(tx)
	switch action {
	case copyObjectBegin:
		return true, p.Begin(ctx, *row.StorageIntentID, fact)
	case copyObjectConfirm:
		return true, p.Confirm(ctx, *row.StorageIntentID, fact)
	case copyObjectBeginRemoval:
		return p.BeginRemoval(ctx, *row.StorageIntentID, fact)
	case copyObjectConfirmRemoval:
		return true, p.ConfirmRemoval(ctx, *row.StorageIntentID, fact)
	}
	return false, application.ErrUnavailable
}
