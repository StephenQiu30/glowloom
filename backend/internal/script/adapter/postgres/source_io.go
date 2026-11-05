package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/domain"
	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

func sourceIntentID(p application.WritePlan) uuid.UUID {
	return uuid.NewSHA1(p.Command.Key, []byte("source-write/"+p.ActorID.String()+"/"+p.Command.ProjectID.String()))
}
func requireSourceIO(tx *gorm.DB, p application.WritePlan, owner *uuid.UUID, requireObjects bool) error {
	var row scriptCommandStateRow
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("actor_id = ? AND request_id = ?", p.ActorID, p.Command.Key).Take(&row).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return application.ErrNotFound
	} else if err != nil {
		return err
	}
	if row.Status != scriptCommandPending || row.CancellationRequested {
		return application.ErrConflict
	}
	if row.IOState != scriptIORunning || row.IOOwnerID == nil || owner != nil && *row.IOOwnerID != *owner {
		return application.ErrNeedsReconciliation
	}
	if requireObjects {
		var intents []scriptObjectIntentRow
		if err := tx.Where("actor_id = ? AND request_id = ?", p.ActorID, p.Command.Key).Find(&intents).Error; err != nil {
			return err
		}
		if len(intents) != len(p.Objects) {
			return application.ErrObjectMismatch
		}
		for _, intent := range intents {
			var state scriptObjectStateRow
			if err := tx.Where("object_key = ? AND object_location_id = ?", intent.ObjectKey, intent.ObjectLocationID).Take(&state).Error; err != nil {
				return err
			}
			if !state.Confirmed || state.Removed {
				return application.ErrNeedsReconciliation
			}
		}
	}
	return nil
}
func exactSourceObject(tx *gorm.DB, p application.WritePlan, f domain.ObjectFact) error {
	if f.LocationID == uuid.Nil {
		return application.ErrObjectMismatch
	}
	var intent scriptObjectIntentRow
	err := tx.Where("actor_id = ? AND request_id = ? AND org_id = ? AND project_id = ? AND object_key = ? AND sha256 = ? AND byte_size = ? AND mime = ? AND object_location_id = ?", p.ActorID, p.Command.Key, p.OrgID, p.Command.ProjectID, f.Key, f.SHA256, f.ByteSize, f.MIME, f.LocationID).Take(&intent).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return application.ErrObjectMismatch
	}
	return err
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
	return s.commandTransaction(ctx, actor, p.Command.ProjectID, p.Command.Key, p.RequestHash, true, func(tx *gorm.DB, _ application.ProjectAccess, _ workspaceapp.ProjectContentAccess) error {
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
		var row scriptCommandStateRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("actor_id = ? AND request_id = ?", actor.ID, p.Command.Key).Take(&row).Error; err != nil {
			return err
		}
		if row.Status != scriptCommandPending || row.CancellationRequested {
			return application.ErrConflict
		}
		if row.IOOwnerID != nil || (row.IOState != scriptIOIdle && row.IOState != scriptIOEnded) {
			return application.ErrNeedsReconciliation
		}
		result := tx.Model(&scriptCommandStateRow{}).
			Where("actor_id = ? AND request_id = ? AND status = ? AND cancellation_requested = ? AND io_owner_id IS NULL", actor.ID, p.Command.Key, scriptCommandPending, false).
			Updates(map[string]any{"io_state": scriptIORunning, "io_owner_id": owner, "revision": row.Revision + 1, "updated_at": now})
		return exactlyOne(result)
	})
}

// BeginSourceObject freezes mutation admission before the synchronous private call.
func (s *SourceStore) BeginSourceObject(ctx context.Context, actor identityapp.Principal, p application.WritePlan, owner uuid.UUID, f domain.ObjectFact) error {
	return s.commandTransaction(ctx, actor, p.Command.ProjectID, p.Command.Key, p.RequestHash, true, func(tx *gorm.DB, _ application.ProjectAccess, _ workspaceapp.ProjectContentAccess) error {
		if err := requireSourceIO(tx, p, &owner, false); err != nil {
			return err
		}
		if err := exactSourceObject(tx, p, f); err != nil {
			return err
		}
		if p.StorageIntentID == uuid.Nil || s.locations == nil || s.locations(tx) == nil {
			return application.ErrUnavailable
		}
		if err := s.locations(tx).Begin(ctx, p.StorageIntentID, f); err != nil {
			return err
		}
		result := tx.Model(&scriptObjectStateRow{}).
			Where("object_key = ? AND object_location_id = ? AND removed = ?", f.Key, f.LocationID, false).
			Updates(map[string]any{"put_started": true, "updated_at": time.Now().UTC()})
		return exactlyOne(result)
	})
}

// ConfirmSourceObject records only an actual full read size and digest proof.
func (s *SourceStore) ConfirmSourceObject(ctx context.Context, actor identityapp.Principal, p application.WritePlan, owner uuid.UUID, f domain.ObjectFact) error {
	return s.commandTransaction(ctx, actor, p.Command.ProjectID, p.Command.Key, p.RequestHash, true, func(tx *gorm.DB, _ application.ProjectAccess, _ workspaceapp.ProjectContentAccess) error {
		if err := requireSourceIO(tx, p, &owner, false); err != nil {
			return err
		}
		if err := exactSourceObject(tx, p, f); err != nil {
			return err
		}
		if p.StorageIntentID == uuid.Nil || s.locations == nil || s.locations(tx) == nil {
			return application.ErrUnavailable
		}
		if err := s.locations(tx).Confirm(ctx, p.StorageIntentID, f); err != nil {
			return err
		}
		result := tx.Model(&scriptObjectStateRow{}).
			Where("object_key = ? AND object_location_id = ? AND put_started = ? AND removed = ?", f.Key, f.LocationID, true, false).
			Updates(map[string]any{"confirmed": true, "updated_at": time.Now().UTC()})
		return exactlyOne(result)
	})
}

// EndSourceIO records the matching call's actual local exit, even after actor revocation.
// It cannot authorize publication, change content, or release unknown remote objects.
func (s *SourceStore) EndSourceIO(ctx context.Context, p application.WritePlan, owner uuid.UUID, uncertain bool, now time.Time) error {
	if s == nil || s.db == nil || owner == uuid.Nil {
		return application.ErrUnavailable
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var command scriptCommandRow
		if err := tx.Where("actor_id = ? AND request_id = ?", p.ActorID, p.Command.Key).Take(&command).Error; err != nil {
			return err
		}
		if command.OrgID != p.OrgID || command.ProjectID != p.Command.ProjectID || command.RequestHash != p.RequestHash {
			return application.ErrConflict
		}
		var state scriptCommandStateRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("actor_id = ? AND request_id = ? AND io_owner_id = ? AND io_state = ?", p.ActorID, p.Command.Key, owner, scriptIORunning).Take(&state).Error; err != nil {
			return err
		}
		written := tx.Model(&scriptCommandStateRow{}).
			Where("actor_id = ? AND request_id = ? AND io_owner_id = ? AND io_state = ?", p.ActorID, p.Command.Key, owner, scriptIORunning).
			Updates(map[string]any{"io_state": scriptIOEnded, "io_owner_id": nil, "needs_reconciliation": state.NeedsReconciliation || uncertain, "revision": state.Revision + 1, "updated_at": now})
		if err := exactlyOne(written); err != nil {
			return fmt.Errorf("record source IO exit: %w", err)
		}
		return nil
	})
}
