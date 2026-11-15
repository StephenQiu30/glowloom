package subject

import (
	"context"
	"time"

	"gorm.io/gorm"

	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/postgres"
	workspace "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/postgres"
	scopewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/scope"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/media/adapter/postgres"
	media "github.com/StephenQiu30/glowloom-server/backend/media/application"
)

type personalSourceActor struct{ db *gorm.DB }

func (s personalSourceActor) PersonalSourceActor(ctx context.Context, actor media.Actor, write bool) (media.Actor, error) {
	var result media.Actor
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := (scopeAccess{tx: tx}).Resolve(ctx, sourceIdentityActor(actor), write); err != nil {
			return err
		}
		store := pgworkspace.NewScopeStore(tx, func(inner *gorm.DB) workspace.ActorAccess { return pgidentity.NewStore(inner) })
		id, err := store.PersonalScopeID(ctx, actor.ID)
		if err != nil {
			return err
		}
		resolved, err := scopewire.ProvideRequestScopeAccess(tx).Resolve(ctx, sourceIdentityActor(actor), id, write)
		if err != nil {
			return err
		}
		result = actor
		result.ScopeID, result.OrgID = resolved.ID, resolved.PhysicalScopeID
		return nil
	})
	return result, err
}

// NewSourceLibrary composes personal and proven generation sources under target authority.
func NewSourceLibrary(db *gorm.DB, objects Objects, previewTTL time.Duration) *media.SubjectLibrary {
	library := pgmedia.NewScopedLibraryStore(db, sourceProjectFactory, time.Now, sourceScopeFactory)
	catalogue := pgmedia.NewSubjectCatalogueStore(db, library, func(tx *gorm.DB) media.SubjectSourceActor { return personalSourceActor{db: tx} })
	files := media.NewRoutedLibraryMediaQuery(catalogue, objects.Routed, objects.Routed, previewTTL)
	return media.NewSubjectLibrary(personalSourceActor{db: db}, catalogue, files)
}
