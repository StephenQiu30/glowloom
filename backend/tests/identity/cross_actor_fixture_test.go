package identity_test

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	scriptwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/script"
	scriptapp "github.com/StephenQiu30/glowloom-server/backend/internal/creation/script/application"
	workspacepg "github.com/StephenQiu30/glowloom-server/backend/internal/creation/workspace/adapter/postgres"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	toolapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/mediatool/application"
)

func identityScriptActor(actor identityapp.Principal) scriptapp.Actor { return scriptwire.Actor(actor) }
func identityScriptAccess(access *workspacepg.ProjectContentAccessStore) scriptapp.ProjectAccess {
	return scriptwire.ProjectAccess(access)
}
func identityAvatarAccess(db *gorm.DB) mediaapp.AvatarActorAccess {
	return mediawire.NewAvatarActorAccess(db)
}
func identityDerivedMedia(service *mediaapp.DerivedService) toolapp.DepthDerivedMedia {
	return mediawire.LegacyDerivedMedia(service)
}
func identityMediaPrincipal(actor mediaapp.Actor) identityapp.Principal {
	return mediawire.IdentityActor(actor)
}
func identityMediaActor(actor identityapp.Principal) mediaapp.Actor { return mediawire.Actor(actor) }
func identityMediaHTTPActor(c *gin.Context) (mediaapp.Actor, error) {
	if c == nil {
		return mediaapp.Actor{}, mediaapp.ErrSessionInvalid
	}
	actor := identityMediaActor(identityhttp.Principal(c))
	if actor.ID == uuid.Nil || actor.SessionID == uuid.Nil || actor.CredentialEpoch < 1 {
		return mediaapp.Actor{}, mediaapp.ErrSessionInvalid
	}
	return actor, nil
}
