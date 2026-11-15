package bible_test

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	app "github.com/StephenQiu30/glowloom-server/backend/creation/bible/application"
	workspacepg "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/postgres"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/http"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	biblewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/bible"
)

func bibleActor(actor identityapp.Principal) app.Actor { return biblewire.Actor(actor) }

func bibleIdentityActor(actor app.Actor) identityapp.Principal { return biblewire.IdentityActor(actor) }

func bibleContentAccess(access *workspacepg.ProjectContentAccessStore) app.ProjectAccess {
	return biblewire.ProjectAccess(access)
}

func bibleFixtureContentAccess(tx *gorm.DB) app.ProjectAccess {
	return biblewire.ProjectAccess(bibleFixtureProjectAccess(tx))
}

func bibleHTTPActor(c *gin.Context) (app.Actor, error) {
	return bibleActor(identityhttp.Principal(c)), nil
}
