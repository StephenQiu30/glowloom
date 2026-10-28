package subject

import (
	"context"
	"time"

	"gorm.io/gorm"

	scopewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/scope"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/postgres"
	workspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

type personalSourceActor struct{ db *gorm.DB }

func (s personalSourceActor) PersonalSourceActor(ctx context.Context, actor identity.Principal, write bool) (identity.Principal, error) {
	var result identity.Principal
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := (scopeAccess{tx: tx}).Resolve(ctx, actor, write); err != nil {
			return err
		}
		store := pgworkspace.NewScopeStore(tx, func(inner *gorm.DB) workspace.ActorAccess { return pgidentity.NewStore(inner) })
		id, err := store.PersonalScopeID(ctx, actor.ID)
		if err != nil {
			return err
		}
		resolved, err := scopewire.ProvideRequestScopeAccess(tx).Resolve(ctx, actor, id, write)
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
func NewSourceLibrary(db *gorm.DB, objects Objects) *media.SubjectLibrary {
	library := pgmedia.NewScopedLibraryStore(db, sourceProjectFactory, time.Now, sourceScopeFactory)
	catalogue := pgmedia.NewSubjectCatalogueStore(db, library, func(tx *gorm.DB) media.SubjectSourceActor { return personalSourceActor{db: tx} })
	var files *media.LibraryMediaQuery
	if objects.Routed != nil {
		files = media.NewRoutedLibraryMediaQuery(catalogue, objects.Routed, objects.Routed, 10*time.Minute)
	} else if objects.Legacy != nil {
		files = media.NewLibraryMediaQuery(catalogue, objects.Legacy, objects.Legacy)
	}
	return media.NewSubjectLibrary(personalSourceActor{db: db}, catalogue, files)
}
