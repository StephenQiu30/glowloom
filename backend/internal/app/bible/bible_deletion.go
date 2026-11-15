// Package bible wires bible storage and cross-module references.
package bible

import (
	"context"

	scriptwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/script"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pgbible "github.com/StephenQiu30/glowloom-server/backend/creation/bible/adapter/postgres"
	bibleapp "github.com/StephenQiu30/glowloom-server/backend/creation/bible/application"
	pgscript "github.com/StephenQiu30/glowloom-server/backend/creation/script/adapter/postgres"
)

func provideBibleDeletionImpacts(tx *gorm.DB) bibleapp.Impacts {
	return pgbible.NewDeletionImpacts(tx, provideBibleProjectAccess(tx), bibleCharacterUsage{pgscript.NewBibleScopes(tx, scriptwire.ProvideScriptProjectAccess(tx))})
}

type bibleCharacterUsage struct{ reader *pgscript.BibleScopes }

func (r bibleCharacterUsage) Read(ctx context.Context, actor bibleapp.Actor, project uuid.UUID, ids []uuid.UUID) (bibleapp.CharacterUsage, error) {
	facts, err := r.reader.ReadCharacterUsage(ctx, scriptwire.Actor(IdentityActor(actor)), project, ids)
	return bibleapp.CharacterUsage{SHA256: facts.SHA256, CurrentLines: facts.CurrentLines, HistoricalLines: facts.HistoricalLines}, OwningError(err)
}
