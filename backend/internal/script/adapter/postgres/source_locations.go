package postgres

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/script/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/domain"
)

// SourceObjectLocationFactory binds the media owner's port to this transaction.
type SourceObjectLocationFactory func(*gorm.DB) application.SourceObjectLocations

// WithObjectLocations injects storage into an independently constructed source
// or import store, preserving its authorization and I/O ownership dependencies.
func (s *SourceStore) WithObjectLocations(factory SourceObjectLocationFactory) *SourceStore {
	bound := *s
	bound.locations = factory
	return &bound
}

func (s *SourceStore) admitSourceLocations(ctx context.Context, tx *gorm.DB, p *application.WritePlan) error {
	if len(p.Objects) == 0 {
		return nil
	}
	if s.locations == nil {
		return application.ErrUnavailable
	}
	owner := s.locations(tx)
	if owner == nil {
		return application.ErrUnavailable
	}
	id, objects, err := owner.Admit(ctx, p.Command.ProjectID, sourceIntentID(*p), p.Objects)
	if err != nil {
		return err
	}
	if id == uuid.Nil || len(objects) != len(p.Objects) {
		return application.ErrObjectMismatch
	}
	for i := range objects {
		a, b := objects[i], p.Objects[i]
		a.LocationID, b.LocationID = uuid.Nil, uuid.Nil
		if a != b || objects[i].LocationID == uuid.Nil {
			return application.ErrObjectMismatch
		}
	}
	p.StorageIntentID, p.Objects = id, objects
	applyPlanLocations(p)
	return nil
}

func applyPlanLocations(p *application.WritePlan) {
	byKey := make(map[string]domain.ObjectFact, len(p.Objects))
	for _, f := range p.Objects {
		byKey[f.Key] = f
	}
	set := func(f *domain.ObjectFact) {
		if bound, ok := byKey[f.Key]; ok {
			f.LocationID = bound.LocationID
		}
	}
	for i := range p.NewSources {
		set(&p.NewSources[i].Original)
		set(&p.NewSources[i].Rich)
	}
	set(&p.Version.Text)
	set(&p.Version.Rich)
}

func loadPlanLocations(tx *gorm.DB, p *application.WritePlan) error {
	if len(p.Objects) == 0 {
		if p.StorageIntentID != uuid.Nil {
			return application.ErrObjectMismatch
		}
		return nil
	}
	if p.StorageIntentID == uuid.Nil {
		return application.ErrObjectMismatch
	}
	var rows []scriptObjectIntentRow
	if err := tx.Model(&scriptObjectIntentRow{}).
		Select("object_key", "object_location_id").
		Where("actor_id = ? AND request_id = ?", p.ActorID, p.Command.Key).
		Order("object_key").
		Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) != len(p.Objects) {
		return application.ErrObjectMismatch
	}
	for i := range p.Objects {
		found := false
		for _, row := range rows {
			if row.ObjectKey == p.Objects[i].Key {
				p.Objects[i].LocationID = row.ObjectLocationID
				found = true
				break
			}
		}
		if !found || p.Objects[i].LocationID == uuid.Nil {
			return application.ErrObjectMismatch
		}
	}
	applyPlanLocations(p)
	return nil
}
