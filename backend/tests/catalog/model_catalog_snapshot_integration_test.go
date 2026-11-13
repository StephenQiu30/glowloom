package catalog_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	scopewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/scope"
	pgcatalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/postgres"
	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

type catalogProjectRead func(context.Context, identity.Principal, uuid.UUID) error

func (read catalogProjectRead) AuthorizeCatalog(ctx context.Context, actor identity.Principal, project uuid.UUID) error {
	return read(ctx, actor, project)
}

func TestModelCatalogSnapshotRetriesSessionActivityAndRechecksRevocation(t *testing.T) {
	for _, revoked := range []bool{false, true} {
		name := "activity"
		if revoked {
			name = "revocation"
		}
		t.Run(name, func(t *testing.T) {
			ctx, database := modelCatalogDB(t)
			actor := insertModelCatalogActor(ctx, t, database, insertModelCatalogOrganization(ctx, t, database))
			project := insertModelCatalogProject(ctx, t, database, actor, false)
			attempts := 0
			store := pgcatalog.NewScopedStore(database, nil, func(tx *gorm.DB) catalog.ProjectAccess {
				return catalogProjectRead(func(ctx context.Context, current identity.Principal, id uuid.UUID) error {
					attempts++
					// Establish the read snapshot before a different connection commits
					// session activity or revocation, as concurrent authenticated requests do.
					var present int
					if err := tx.Raw("SELECT 1").Scan(&present).Error; err != nil {
						return err
					}
					if attempts == 1 {
						query := `UPDATE identity.user_session SET last_active_at=now() WHERE id=?`
						if revoked {
							query = `UPDATE identity.user_session SET revoked_at=now() WHERE id=?`
						}
						if err := database.WithContext(ctx).Exec(query, actor.SessionID).Error; err != nil {
							return err
						}
					}
					_, err := scopewire.ProvideProjectContentAccess(tx).Authorize(ctx, current, id, false)
					return err
				})
			})
			page, err := store.ListModelsForProject(ctx, actor, catalog.ListModelsInput{ProjectID: project, Capability: "audio.generate", ModelKey: "snapshot-" + uuid.NewString(), Limit: 1})
			if revoked {
				require.ErrorIs(t, err, identity.ErrSessionInvalid)
			} else {
				require.NoError(t, err)
				require.Empty(t, page.Models)
			}
			require.Equal(t, 2, attempts, "the whole authorized read must restart with a fresh snapshot")
		})
	}
}

func TestModelCatalogSnapshotSurvivesBurstSessionActivity(t *testing.T) {
	ctx, database := modelCatalogDB(t)
	actor := insertModelCatalogActor(ctx, t, database, insertModelCatalogOrganization(ctx, t, database))
	project := insertModelCatalogProject(ctx, t, database, actor, false)
	attempts := 0
	store := pgcatalog.NewScopedStore(database, nil, func(tx *gorm.DB) catalog.ProjectAccess {
		return catalogProjectRead(func(ctx context.Context, current identity.Principal, id uuid.UUID) error {
			attempts++
			var present int
			if err := tx.Raw("SELECT 1").Scan(&present).Error; err != nil {
				return err
			}
			// A burst of successful authenticated reads can update activity between
			// successive snapshots; three immediate retries do not outlast that burst.
			if attempts <= 4 {
				if err := database.WithContext(ctx).Exec(`UPDATE identity.user_session SET last_active_at=clock_timestamp() WHERE id=?`, actor.SessionID).Error; err != nil {
					return err
				}
			}
			_, err := scopewire.ProvideProjectContentAccess(tx).Authorize(ctx, current, id, false)
			return err
		})
	})
	page, err := store.ListModelsForProject(ctx, actor, catalog.ListModelsInput{ProjectID: project, Capability: "image.generate", ModelKey: "snapshot-burst-" + uuid.NewString(), Limit: 1})
	require.NoError(t, err)
	require.Empty(t, page.Models)
	require.Equal(t, 5, attempts)
}
