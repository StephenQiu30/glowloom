package bible_test

import (
	"context"

	"github.com/google/uuid"

	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	scriptwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/script"
	canvasapp "github.com/StephenQiu30/glowloom-server/backend/internal/creation/canvas/application"
	scriptapp "github.com/StephenQiu30/glowloom-server/backend/internal/creation/script/application"
	workspacepg "github.com/StephenQiu30/glowloom-server/backend/internal/creation/workspace/adapter/postgres"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

func bibleScriptAccess(access *workspacepg.ProjectContentAccessStore) scriptapp.ProjectAccess {
	return scriptwire.ProjectAccess(access)
}
func bibleLegacyMedia(query *mediaapp.AssetQuery) canvasapp.MediaReader {
	return mediawire.LegacyAssetReader(query)
}

type bibleScriptWorkGuard func(context.Context, scriptapp.Actor, uuid.UUID) (bool, error)

func (g bibleScriptWorkGuard) HasInflightWork(ctx context.Context, actor identityapp.Principal, project uuid.UUID) (bool, error) {
	return g(ctx, bibleScriptActor(actor), project)
}
func bibleScriptActor(actor identityapp.Principal) scriptapp.Actor { return scriptwire.Actor(actor) }
func bibleMediaActor(actor identityapp.Principal) mediaapp.Actor   { return mediawire.Actor(actor) }
