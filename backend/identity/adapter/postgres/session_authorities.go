package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// ReadSessionAuthorities rechecks the current exact account/session and source
// decisions. It never derives authority generations from credential revisions,
// initializes a head, records admission, updates activity or sends an RPC.
func (s *SessionAdmissionStore) ReadSessionAuthorities(ctx context.Context, actor application.Principal, clock func() time.Time) (application.SessionAuthorities, error) {
	var out application.SessionAuthorities
	if s == nil || clock == nil {
		return out, writefence.ErrUnavailable
	}
	err := identityWriterTransaction(ctx, s.db, func(tx *gorm.DB) error {
		if _, err := lockSelfActor(tx, actor, clock); err != nil {
			return err
		}
		var err error
		out, err = readOpenSessionAuthorities(tx, actor)
		return err
	})
	if err != nil {
		return application.SessionAuthorities{}, err
	}
	return out, nil
}

// Caller already holds the actual current account/session policy locks. The head
// locks and exact four-writer checks stay in that same local transaction.
func readOpenSessionAuthorities(tx *gorm.DB, actor application.Principal) (application.SessionAuthorities, error) {
	var out application.SessionAuthorities
	if err := requireOpenAccountHead(tx, actor.ID); err != nil {
		return out, err
	}
	for _, key := range []struct {
		kind        writefence.Kind
		id          uuid.UUID
		destination *writefence.Authority
	}{{writefence.Account, actor.ID, &out.Account}, {writefence.Session, actor.SessionID, &out.Session}} {
		var head struct {
			Epoch             int64
			CoordinationID    uuid.UUID
			Blocked, Complete bool
		}
		read := tx.Raw(`SELECT epoch,coordination_id,blocked,complete FROM infra.authority_head WHERE service_owner=infra.runtime_owner() AND issuer='identity' AND authority_kind=? AND authority_id=? FOR UPDATE`, key.kind, key.id).Scan(&head)
		if read.Error != nil {
			return out, read.Error
		}
		if read.RowsAffected != 1 || head.Epoch < 1 || head.Blocked || !head.Complete {
			return out, writefence.ErrFenced
		}
		control := writefence.Control{Authority: writefence.Authority{Issuer: writefence.Identity, Kind: key.kind, ID: key.id, Epoch: head.Epoch}, CoordinationID: head.CoordinationID}
		if err := requirePasswordControl(tx.Statement.Context, tx, control, true); err != nil {
			return out, err
		}
		*key.destination = control.Authority
	}
	return out, nil
}

var _ application.SessionAuthorityReader = (*SessionAdmissionStore)(nil)
