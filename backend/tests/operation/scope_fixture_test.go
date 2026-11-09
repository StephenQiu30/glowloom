package operation_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pgbilling "github.com/StephenQiu30/glowloom-server/backend/internal/billing/adapter/postgres"
	billingapp "github.com/StephenQiu30/glowloom-server/backend/internal/billing/application"
	pgcatalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/postgres"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	pgoperation "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/postgres"
	operationapp "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	pgprompt "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/adapter/postgres"
	promptapp "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/application"
	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
	"github.com/StephenQiu30/glowloom-server/backend/tests/identityfixture"
)

type operationProjectAccess struct {
	store  *pgworkspace.ProjectContentAccessStore
	worker bool
}

func (a operationProjectAccess) AuthorizeOperation(ctx context.Context, actor identityapp.Principal, project uuid.UUID, write bool) error {
	_, err := a.store.Authorize(ctx, actor, project, write)
	if a.worker && (errors.Is(err, identityapp.ErrForbidden) || errors.Is(err, workspaceapp.ErrScopeNotFound) || errors.Is(err, workspaceapp.ErrProjectNotFound)) {
		return operationapp.ErrExecutionAuthorityRevoked
	}
	return err
}

func (a operationProjectAccess) AuthorizeBilling(ctx context.Context, actor identityapp.Principal, project uuid.UUID, write bool) error {
	return a.AuthorizeOperation(ctx, actor, project, write)
}

func operationProjectContentAccess(tx *gorm.DB) *pgworkspace.ProjectContentAccessStore {
	scopes := workspaceapp.NewRequestScopeAccess(pgworkspace.NewRequestScopeStore(tx, func(ownerTx *gorm.DB) workspaceapp.SessionActorAccess {
		return pgidentity.NewStore(ownerTx)
	}), time.Now)
	return pgworkspace.NewScopedProjectContentAccessStore(tx, scopes)
}

func operationRequestAccess(tx *gorm.DB) operationapp.ProjectAccess {
	return operationProjectAccess{store: operationProjectContentAccess(tx)}
}

func operationWorkerAccess(tx *gorm.DB) operationapp.ProjectAccess {
	scopes := workspaceapp.NewScopeAccess(pgworkspace.NewScopeStore(tx, func(ownerTx *gorm.DB) workspaceapp.ActorAccess {
		return pgidentity.NewStore(ownerTx)
	}))
	return operationProjectAccess{store: pgworkspace.NewWorkerProjectContentAccessStore(tx, scopes), worker: true}
}

func operationPlatformAccess(tx *gorm.DB) operationapp.PlatformAccess { return pgidentity.NewStore(tx) }

func operationStore(database *gorm.DB) *pgoperation.Store {
	return pgoperation.NewScopedStore(database, operationRequestAccess, operationWorkerAccess, operationPlatformAccess).WithExecutionAvailability(fixtureExecutionAvailability)
}

func operationBillingStore(database *gorm.DB) *pgbilling.Store {
	return pgbilling.NewScopedStore(database, func(tx *gorm.DB) billingapp.ProjectAccess {
		return operationProjectAccess{store: operationProjectContentAccess(tx)}
	})
}

func operationPromptStore(database *gorm.DB) *pgprompt.Store {
	return pgprompt.NewScopedStore(database, func(tx *gorm.DB) promptapp.ActorAccess { return pgidentity.NewStore(tx) })
}

// operationReviewerSession gives a synthetic reviewer explicit team membership;
// platform authority is supplied separately by the administrator fixture.
func operationReviewerSession(t *testing.T, database *gorm.DB, operation, reviewer uuid.UUID) identityapp.Principal {
	t.Helper()
	var row struct {
		ProjectID, OrgID, OwnerScopeID, ActorID uuid.UUID
		TeamID                                  *uuid.UUID
	}
	if err := database.Raw(`SELECT o.project_id,p.org_id,p.owner_scope_id,o.confirmed_by AS actor_id,s.team_id
		FROM operation.operation o JOIN workspace.project p ON p.id=o.project_id
		JOIN workspace.creation_scope s ON s.id=p.owner_scope_id WHERE o.id=?`, operation).Scan(&row).Error; err != nil || row.ActorID == uuid.Nil {
		t.Fatal("read reviewer project authority", err)
	}
	if row.TeamID == nil {
		team, scope := uuid.New(), uuid.New()
		if err := database.Exec(`INSERT INTO workspace.team(id,name,created_by) VALUES(?,'Operation reviewers',?)`, team, row.ActorID).Error; err != nil {
			t.Fatal(err)
		}
		if err := database.Exec(`INSERT INTO workspace.creation_scope(id,kind,team_id,physical_scope_id) VALUES(?,'team',?,?)`, scope, team, row.OrgID).Error; err != nil {
			t.Fatal(err)
		}
		if err := database.Exec(`INSERT INTO workspace.team_member(team_id,user_id,role,status,joined_at,source) VALUES(?,?,'team_admin','active',now(),'created')`, team, row.ActorID).Error; err != nil {
			t.Fatal(err)
		}
		if err := database.Exec(`UPDATE workspace.project SET owner_scope_id=? WHERE id=?`, scope, row.ProjectID).Error; err != nil {
			t.Fatal(err)
		}
		row.TeamID, row.OwnerScopeID = &team, scope
	}
	if err := database.Exec(`INSERT INTO workspace.team_member(team_id,user_id,role,status,joined_at,source) VALUES(?,?,'member','active',now(),'invited') ON CONFLICT DO NOTHING`, *row.TeamID, reviewer).Error; err != nil {
		t.Fatal(err)
	}
	actor := identityfixture.Session(t, database, identityapp.Principal{ID: reviewer, OrgID: row.OrgID})
	actor.ScopeID = row.OwnerScopeID
	return actor
}

// fixtureExecutionAvailability isolates historical transaction tests that use a
// deliberately uninstalled quote-test adapter. Real cloud fixtures always use
// the production reader; no runtime code accepts this synthetic adapter.
func fixtureExecutionAvailability(tx *gorm.DB) operationapp.ExecutionAvailability {
	return legacyQuoteExecutionReader{tx: tx}
}

type legacyQuoteExecutionReader struct{ tx *gorm.DB }

func (r legacyQuoteExecutionReader) AvailableForNewExecution(ctx context.Context, version, release uuid.UUID, mode string) (bool, error) {
	var synthetic bool
	err := r.tx.WithContext(ctx).Raw(`SELECT EXISTS(SELECT 1 FROM catalog.model_profile_version v
 JOIN catalog.model_profile m ON m.id=v.model_profile_id JOIN catalog.provider p ON p.id=m.provider_id
 WHERE v.id=? AND p.adapter_key='quote-test')`, version).Row().Scan(&synthetic)
	if err != nil || synthetic {
		return synthetic, err
	}
	return pgcatalog.NewStore(r.tx).AvailableForNewExecution(ctx, version, release, mode)
}
