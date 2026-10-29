package postgres

import (
	"github.com/google/uuid"
	"gorm.io/gorm"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

// Agent operations carry private conversation context. The immutable quote
// receipt identifies their initiating actor even before a charge is confirmed.
const agentOwnerPredicate = `(o.origin<>'agent' OR EXISTS(SELECT 1 FROM operation.quote_request aq WHERE aq.actor_id=? AND aq.result->>'OperationID'=o.id::text))`

func requireAgentOperationOwner(tx *gorm.DB, actor identity.Principal, project, id uuid.UUID) error {
	var found int
	r := tx.Raw(`SELECT 1 FROM operation.operation o WHERE o.id=? AND o.project_id=? AND NOT o.is_delete AND `+agentOwnerPredicate, id, project, actor.ID).Scan(&found)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return application.ErrPublicNotFound
	}
	return nil
}
