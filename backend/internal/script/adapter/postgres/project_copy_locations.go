package postgres

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/script/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/domain"
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
	var rows []struct {
		SourceKey, TargetKey           string
		SourceLocation, TargetLocation *uuid.UUID
	}
	if err := tx.Raw(`SELECT source_key,target_key,source_location_id AS source_location,target_location_id AS target_location FROM script.copy_object_intent WHERE snapshot_id=?`, snapshot.ID).Scan(&rows).Error; err != nil {
		return err
	}
	if len(rows) != len(m.Objects) {
		return application.ErrObjectMismatch
	}
	targets := make([]domain.ObjectFact, len(m.Objects))
	for i := range m.Objects {
		found := false
		for _, r := range rows {
			if r.TargetKey == m.Objects[i].Target.Key && r.SourceKey == m.Objects[i].Source.Key {
				m.Objects[i].Source.LocationID, m.Objects[i].Target.LocationID = locationValue(r.SourceLocation), locationValue(r.TargetLocation)
				found = true
				break
			}
		}
		if !found {
			return application.ErrObjectMismatch
		}
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

func (s *ProjectCopyStore) copyLocationAction(ctx context.Context, tx *gorm.DB, snapshot application.ProjectCopySnapshot, fact domain.ObjectFact, action string) error {
	var row struct{ StorageIntentID *uuid.UUID }
	if err := tx.Raw(`SELECT storage_intent_id FROM script.copy_snapshot WHERE id=?`, snapshot.ID).Scan(&row).Error; err != nil {
		return err
	}
	id := row.StorageIntentID
	if id == nil {
		return nil
	}
	if s.locations == nil || s.locations(tx) == nil {
		return application.ErrUnavailable
	}
	p := s.locations(tx)
	switch action {
	case "begin":
		return p.Begin(ctx, *id, fact)
	case "confirm":
		return p.Confirm(ctx, *id, fact)
	case "remove":
		return p.BeginRemoval(ctx, *id, fact)
	case "removed":
		return p.ConfirmRemoval(ctx, *id, fact)
	}
	return application.ErrUnavailable
}
