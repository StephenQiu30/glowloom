// Package bible wires bible storage and cross-module references.
package bible

import (
	"context"

	scriptwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/script"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pgbible "github.com/StephenQiu30/glowloom-server/backend/internal/bible/adapter/postgres"
	bibleapp "github.com/StephenQiu30/glowloom-server/backend/internal/bible/application"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	pgscript "github.com/StephenQiu30/glowloom-server/backend/internal/script/adapter/postgres"
)

func provideBibleDeletionImpacts(tx *gorm.DB) bibleapp.Impacts {
	return pgbible.NewDeletionImpacts(tx, provideBibleProjectAccess(tx), bibleCharacterUsage{pgscript.NewBibleScopes(tx, scriptwire.ProvideScriptProjectAccess(tx))})
}

type bibleCharacterUsage struct{ reader *pgscript.BibleScopes }

func (r bibleCharacterUsage) Read(ctx context.Context, actor identityapp.Principal, project uuid.UUID, ids []uuid.UUID) (bibleapp.CharacterUsage, error) {
	facts, err := r.reader.ReadCharacterUsage(ctx, actor, project, ids)
	return bibleapp.CharacterUsage{SHA256: facts.SHA256, CurrentLines: facts.CurrentLines, HistoricalLines: facts.HistoricalLines}, err
}
