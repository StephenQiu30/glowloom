// Package insights wires diagnostic and analysis reads to existing live identity authority.
package insights

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	analyticshttp "github.com/StephenQiu30/glowloom-server/backend/internal/analytics/adapter/http"
	analyticspg "github.com/StephenQiu30/glowloom-server/backend/internal/analytics/adapter/postgres"
	analyticsapp "github.com/StephenQiu30/glowloom-server/backend/internal/analytics/application"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	obshttp "github.com/StephenQiu30/glowloom-server/backend/internal/observability/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/observability/adapter/memory"
	obsapp "github.com/StephenQiu30/glowloom-server/backend/internal/observability/application"
)

// PlatformAccess keeps identity locks in the same read transaction.
type PlatformAccess struct {
	database *gorm.DB
	clock    func() time.Time
}

// NewPlatformAccess injects the database and authorization clock.
func NewPlatformAccess(database *gorm.DB, clock func() time.Time) *PlatformAccess {
	return &PlatformAccess{database, clock}
}

// ReadPlatform checks the exact active session and current platform epoch, not OrgID.
func (a *PlatformAccess) ReadPlatform(ctx context.Context, actor identity.Principal, read func() error) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return a.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := pgidentity.NewStore(tx).CheckPlatformSession(ctx, actor, a.clock); err != nil {
			return err
		}
		return read()
	})
}

// Register adds only authenticated, read-only page projections.
func Register(group *gin.RouterGroup, database *gorm.DB, buffer *memory.Buffer, clock func() time.Time) {
	obshttp.NewHandler(obsapp.NewQuery(NewPlatformAccess(database, clock), buffer, clock), clock).Register(group)
	store := analyticspg.NewStore(database, func(tx *gorm.DB) analyticspg.Access { return pgidentity.NewStore(tx) })
	analyticshttp.NewHandler(analyticsapp.NewQuery(store, clock), clock).Register(group)
}
