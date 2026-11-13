package catalog_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	pgcatalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/postgres"
	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/tests/identityfixture"
)

type catalogSessionRead struct {
	catalog.ActorAccess
	before func() error
}

func (a catalogSessionRead) CheckPlatformSession(ctx context.Context, actor identity.Principal, clock func() time.Time) error {
	if err := a.before(); err != nil {
		return err
	}
	return a.ActorAccess.CheckPlatformSession(ctx, actor, clock)
}

func TestAdminCatalogSnapshotsRetryActivityAndRecheckRevocation(t *testing.T) {
	for _, kind := range []string{"providers", "models", "detail"} {
		for _, revoked := range []bool{false, true} {
			name := kind + "/activity"
			if revoked {
				name = kind + "/revocation"
			}
			t.Run(name, func(t *testing.T) {
				ctx, database := modelCatalogDB(t)
				actor := insertModelCatalogActor(ctx, t, database, insertModelCatalogOrganization(ctx, t, database))
				require.NoError(t, database.Exec(`UPDATE identity."user" SET platform_role='platform_admin' WHERE id=?`, actor.ID).Error)
				actor = identityfixture.Session(t, database, actor)
				provider := insertModelCatalogProvider(ctx, t, database, "domestic", "active", "")
				model, _ := insertModelCatalogModel(ctx, t, database, "admin-snapshot-"+uuid.NewString(), provider, "text.generate", "disabled", "text_to_text", false)
				attempts := 0
				store := pgcatalog.NewScopedStore(database, func(tx *gorm.DB) catalog.ActorAccess {
					return catalogSessionRead{ActorAccess: pgidentity.NewStore(tx), before: func() error {
						attempts++
						var present int
						if err := tx.Raw("SELECT 1").Scan(&present).Error; err != nil {
							return err
						}
						if attempts <= 4 {
							query := `UPDATE identity.user_session SET last_active_at=clock_timestamp() WHERE id=?`
							if revoked && attempts == 4 {
								query = `UPDATE identity.user_session SET revoked_at=clock_timestamp() WHERE id=?`
							}
							return database.WithContext(ctx).Exec(query, actor.SessionID).Error
						}
						return nil
					}}
				}, nil)
				var err error
				switch kind {
				case "providers":
					_, err = store.ListProvidersForAdmin(ctx, actor, catalog.ListProvidersInput{Limit: 1, Name: provider.String()})
				case "models":
					_, err = store.ListAdminModels(ctx, actor, catalog.AdminModelListInput{Limit: 1, ProviderID: provider})
				case "detail":
					_, err = store.ReadAdminModel(ctx, actor, model)
				}
				if revoked {
					require.ErrorIs(t, err, identity.ErrSessionInvalid)
				} else {
					require.NoError(t, err)
				}
				require.Equal(t, 5, attempts)
			})
		}
	}
}
