package canvas_test

import (
	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	canvasapp "github.com/StephenQiu30/glowloom-server/backend/internal/creation/canvas/application"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

func canvasMediaActor(actor identityapp.Principal) mediaapp.Actor { return mediawire.Actor(actor) }
func canvasLegacyMedia(query *mediaapp.AssetQuery) canvasapp.MediaReader {
	return mediawire.LegacyAssetReader(query)
}
