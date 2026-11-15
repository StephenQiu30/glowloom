package canvas_test

import (
	canvasapp "github.com/StephenQiu30/glowloom-server/backend/creation/canvas/application"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/media/application"
)

func canvasMediaActor(actor identityapp.Principal) mediaapp.Actor { return mediawire.Actor(actor) }
func canvasLegacyMedia(query *mediaapp.AssetQuery) canvasapp.MediaReader {
	return mediawire.LegacyAssetReader(query)
}
