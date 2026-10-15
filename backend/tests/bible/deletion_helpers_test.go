package bible_test

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pg "github.com/StephenQiu30/glowloom-server/backend/internal/bible/adapter/postgres"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/bible/application"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	scriptpg "github.com/StephenQiu30/glowloom-server/backend/internal/script/adapter/postgres"
)

type deletionUsage struct{ reader *scriptpg.BibleScopes }

func (u deletionUsage) Read(ctx context.Context, actor identityapp.Principal, project uuid.UUID, ids []uuid.UUID) (app.CharacterUsage, error) {
	fact, err := u.reader.ReadCharacterUsage(ctx, actor, project, ids)
	return app.CharacterUsage{SHA256: fact.SHA256, CurrentLines: fact.CurrentLines, HistoricalLines: fact.HistoricalLines}, err
}
func deletionStore(db *gorm.DB) *pg.Store {
	return pg.NewStore(db, pg.Factories{Access: func(tx *gorm.DB) app.ProjectAccess { return bibleFixtureProjectAccess(tx) }, Impacts: func(tx *gorm.DB) app.Impacts {
		access := bibleFixtureProjectAccess(tx)
		return pg.NewDeletionImpacts(tx, access, deletionUsage{scriptpg.NewBibleScopes(tx, access)})
	}})
}
