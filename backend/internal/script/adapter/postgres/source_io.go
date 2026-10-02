package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
)

func sourceIntentID(p application.WritePlan) uuid.UUID {
	return uuid.NewSHA1(p.Command.Key, []byte("source-write/"+p.ActorID.String()+"/"+p.Command.ProjectID.String()))
}
func requireSourceIO(tx *gorm.DB, p application.WritePlan, owner *uuid.UUID, requireObjects bool) error {
	var row struct {
		Status, IOState       string
		CancellationRequested bool
		IOOwnerID             *uuid.UUID
	}
	read := tx.Raw(`SELECT status,io_state,io_owner_id,cancellation_requested FROM script.command_state WHERE actor_id=? AND request_id=? FOR UPDATE`, p.ActorID, p.Command.Key).Scan(&row)
	if read.Error != nil {
		return read.Error
	}
	if read.RowsAffected != 1 {
		return application.ErrNotFound
	}
	if row.Status != "pending" || row.CancellationRequested {
		return application.ErrConflict
	}
	if row.IOState != "running" || row.IOOwnerID == nil || owner != nil && *row.IOOwnerID != *owner {
		return application.ErrNeedsReconciliation
	}
	if requireObjects {
		var incomplete bool
		if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM script.object_intent i JOIN script.object_state s USING(object_key) WHERE i.actor_id=? AND i.request_id=? AND (NOT s.confirmed OR s.removed))`, p.ActorID, p.Command.Key).Scan(&incomplete).Error; err != nil {
			return err
		}
		if incomplete {
			return application.ErrNeedsReconciliation
		}
	}
	return nil
}
func exactSourceObject(tx *gorm.DB, p application.WritePlan, f domain.ObjectFact) error {
	var count int64
	if err := tx.Raw(`SELECT count(*) FROM script.object_intent WHERE actor_id=? AND request_id=? AND org_id=? AND project_id=? AND object_key=? AND sha256=? AND byte_size=? AND mime=?`, p.ActorID, p.Command.Key, p.OrgID, p.Command.ProjectID, f.Key, f.SHA256, f.ByteSize, f.MIME).Scan(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return application.ErrObjectMismatch
	}
	return nil
}
func exactlyOne(result *gorm.DB) error {
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return application.ErrConflict
	}
	return nil
}

// ClaimSourceIO requires an actual prior exit before a new call may own I/O.
func (s *SourceStore) ClaimSourceIO(ctx context.Context, actor identityapp.Principal, p application.WritePlan, owner uuid.UUID, now time.Time) error {
	if owner == uuid.Nil {
		return domain.ErrInvalidSource
	}
	return s.transaction(ctx, actor, p.Command.ProjectID, true, func(tx *gorm.DB, _ application.ProjectAccess, _ workspaceapp.ProjectContentAccess) error {
		prior, err := findCommand(tx, actor, p.Command.ProjectID, p.Command.Key, p.RequestHash)
		if err != nil {
			return err
		}
		if prior == nil {
			return application.ErrNotFound
		}
		if prior.Receipt != nil {
			return application.ErrConflict
		}
		var row struct {
			Status, IOState       string
			CancellationRequested bool
			IOOwnerID             *uuid.UUID
		}
		if err := tx.Raw(`SELECT status,io_state,io_owner_id,cancellation_requested FROM script.command_state WHERE actor_id=? AND request_id=? FOR UPDATE`, actor.ID, p.Command.Key).Scan(&row).Error; err != nil {
			return err
		}
		if row.Status != "pending" || row.CancellationRequested {
			return application.ErrConflict
		}
		if row.IOOwnerID != nil || (row.IOState != "idle" && row.IOState != "ended") {
			return application.ErrNeedsReconciliation
		}
		return exactlyOne(tx.Exec(`UPDATE script.command_state SET io_state='running',io_owner_id=?,revision=revision+1,updated_at=? WHERE actor_id=? AND request_id=? AND status='pending' AND NOT cancellation_requested AND io_owner_id IS NULL`, owner, now, actor.ID, p.Command.Key))
	})
}

// BeginSourceObject freezes mutation admission before the synchronous private call.
func (s *SourceStore) BeginSourceObject(ctx context.Context, actor identityapp.Principal, p application.WritePlan, owner uuid.UUID, f domain.ObjectFact) error {
	return s.transaction(ctx, actor, p.Command.ProjectID, true, func(tx *gorm.DB, _ application.ProjectAccess, _ workspaceapp.ProjectContentAccess) error {
		if err := requireSourceIO(tx, p, &owner, false); err != nil {
			return err
		}
		if err := exactSourceObject(tx, p, f); err != nil {
			return err
		}
		return exactlyOne(tx.Exec(`UPDATE script.object_state SET put_started=true,updated_at=now() WHERE object_key=? AND NOT removed`, f.Key))
	})
}

// ConfirmSourceObject records only an actual full read size and digest proof.
func (s *SourceStore) ConfirmSourceObject(ctx context.Context, actor identityapp.Principal, p application.WritePlan, owner uuid.UUID, f domain.ObjectFact) error {
	return s.transaction(ctx, actor, p.Command.ProjectID, true, func(tx *gorm.DB, _ application.ProjectAccess, _ workspaceapp.ProjectContentAccess) error {
		if err := requireSourceIO(tx, p, &owner, false); err != nil {
			return err
		}
		if err := exactSourceObject(tx, p, f); err != nil {
			return err
		}
		return exactlyOne(tx.Exec(`UPDATE script.object_state SET confirmed=true,updated_at=now() WHERE object_key=? AND put_started AND NOT removed`, f.Key))
	})
}

// EndSourceIO records the matching call's actual local exit, even after actor revocation.
// It cannot authorize publication, change content, or release unknown remote objects.
func (s *SourceStore) EndSourceIO(ctx context.Context, p application.WritePlan, owner uuid.UUID, uncertain bool, now time.Time) error {
	if s == nil || s.db == nil || owner == uuid.Nil {
		return application.ErrUnavailable
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		written := tx.Exec(`UPDATE script.command_state SET io_state='ended',io_owner_id=NULL,needs_reconciliation=needs_reconciliation OR ?,revision=revision+1,updated_at=? WHERE actor_id=? AND request_id=? AND io_owner_id=? AND io_state='running' AND EXISTS(SELECT 1 FROM script.command c WHERE c.actor_id=? AND c.request_id=? AND c.org_id=? AND c.project_id=? AND c.request_hash=?)`, uncertain, now, p.ActorID, p.Command.Key, owner, p.ActorID, p.Command.Key, p.OrgID, p.Command.ProjectID, p.RequestHash)
		if err := exactlyOne(written); err != nil {
			return fmt.Errorf("record source IO exit: %w", err)
		}
		return nil
	})
}
