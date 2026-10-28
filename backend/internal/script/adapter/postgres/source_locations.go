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
	if s.locations == nil || len(p.Objects) == 0 {
		return nil
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
		return nil
	}
	var rows []struct {
		ObjectKey string
		Location  *uuid.UUID
	}
	if err := tx.Raw(`SELECT object_key,object_location_id AS location FROM script.object_intent WHERE actor_id=? AND request_id=? ORDER BY object_key`, p.ActorID, p.Command.Key).Scan(&rows).Error; err != nil {
		return err
	}
	if len(rows) != len(p.Objects) {
		return application.ErrObjectMismatch
	}
	for i := range p.Objects {
		found := false
		for _, row := range rows {
			if row.ObjectKey == p.Objects[i].Key {
				p.Objects[i].LocationID = locationValue(row.Location)
				found = true
				break
			}
		}
		if !found || p.StorageIntentID != uuid.Nil && p.Objects[i].LocationID == uuid.Nil {
			return application.ErrObjectMismatch
		}
	}
	applyPlanLocations(p)
	return nil
}

func locationValue(p *uuid.UUID) uuid.UUID {
	if p == nil {
		return uuid.Nil
	}
	return *p
}

func nullableLocation(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}
