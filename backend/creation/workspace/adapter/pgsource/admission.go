// Package pgsource persists Creation's current resource-policy admission. It owns
// no account/session policy and never queries another owner's business tables.
package pgsource

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	workspacesource "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"

	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

// Store uses the restricted Creation connection without switching roles or GUCs.
type Store struct{ db *gorm.DB }

// New requires source registration to run in its own local transaction.
func New(db *gorm.DB) *Store { return &Store{db: db} }

var _ workspacesource.CommandStore = (*Store)(nil)

// RegisterSourceCommand retains the original scope/team/member/organization and
// project locks through admission commit. A historical admission is not reused
// after membership removal, project closure, pending control or an epoch change.
// Only the original effect-owner receipt may resolve already accepted effects.
func (s *Store) RegisterSourceCommand(ctx context.Context, id workspacesource.Identity, request workspacesource.AdmissionRequest) (workspacesource.Permit, error) {
	if err := request.Validate(id); err != nil {
		return workspacesource.Permit{}, err
	}
	if s == nil || s.db == nil || s.db.Statement == nil {
		return workspacesource.Permit{}, writefence.ErrUnavailable
	}
	if _, open := s.db.Statement.ConnPool.(gorm.TxCommitter); open {
		return workspacesource.Permit{}, writefence.ErrTransaction
	}
	var permit workspacesource.Permit
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		owner, err := pgfence.New(tx).Owner(ctx)
		if err != nil {
			return err
		}
		if owner != writefence.WriterCreation {
			return writefence.ErrInvalid
		}
		physical, err := lockScope(ctx, tx, id.AccountID, request.ScopeID)
		if err != nil {
			return err
		}
		if request.ProjectID != uuid.Nil {
			var state struct {
				Status   string
				IsDelete bool
			}
			read := tx.Raw(`SELECT status,is_delete FROM workspace.project WHERE id=? AND owner_scope_id=? AND org_id=? FOR SHARE`, request.ProjectID, request.ScopeID, physical).Scan(&state)
			if read.Error != nil {
				return fmt.Errorf("lock source project: %w", read.Error)
			}
			if read.RowsAffected != 1 || state.IsDelete || state.Status != "active" {
				return workspacesource.ErrProjectNotFound
			}
			if err := checkNewProjectAdmission(tx, request); err != nil {
				return err
			}
		}
		if err := pgfence.NewAuthorityStore(tx).Register(ctx, tx, request.Command, request.Target); err != nil {
			return err
		}
		permit = workspacesource.Permit{AccountID: id.AccountID, SessionID: id.SessionID, ScopeID: request.ScopeID, ProjectID: request.ProjectID, PhysicalScopeID: physical, Command: request.Command, Target: request.Target}
		return nil
	})
	if err != nil {
		return workspacesource.Permit{}, fmt.Errorf("creation source admission transaction: %w", err)
	}
	return permit, nil
}

func lockScope(ctx context.Context, tx *gorm.DB, actor, scope uuid.UUID) (uuid.UUID, error) {
	var row struct {
		Kind                string
		OwnerUserID, TeamID *uuid.UUID
		PhysicalScopeID     uuid.UUID
	}
	read := tx.WithContext(ctx).Raw(`SELECT kind,owner_user_id,team_id,physical_scope_id FROM workspace.creation_scope WHERE id=? FOR SHARE`, scope).Scan(&row)
	if read.Error != nil {
		return uuid.Nil, fmt.Errorf("lock source scope: %w", read.Error)
	}
	if read.RowsAffected != 1 || row.PhysicalScopeID == uuid.Nil {
		return uuid.Nil, workspacesource.ErrScopeNotFound
	}
	switch row.Kind {
	case "personal":
		if row.OwnerUserID == nil || *row.OwnerUserID != actor || row.TeamID != nil {
			return uuid.Nil, workspacesource.ErrScopeNotFound
		}
	case "team":
		if row.TeamID == nil || *row.TeamID == uuid.Nil || row.OwnerUserID != nil {
			return uuid.Nil, workspacesource.ErrScopeNotFound
		}
		var team struct {
			Status            string
			GovernanceBlocked bool
		}
		read = tx.Raw(`SELECT status,governance_blocked FROM workspace.team WHERE id=? FOR SHARE`, *row.TeamID).Scan(&team)
		if read.Error != nil {
			return uuid.Nil, fmt.Errorf("lock source team: %w", read.Error)
		}
		if read.RowsAffected != 1 || team.Status != "active" || team.GovernanceBlocked {
			return uuid.Nil, workspacesource.ErrScopeNotFound
		}
		var member struct{ Status, Role string }
		read = tx.Raw(`SELECT status,role FROM workspace.team_member WHERE team_id=? AND user_id=? FOR SHARE`, *row.TeamID, actor).Scan(&member)
		if read.Error != nil {
			return uuid.Nil, fmt.Errorf("lock source membership: %w", read.Error)
		}
		if read.RowsAffected != 1 || member.Status != "active" || (member.Role != "member" && member.Role != "team_admin") {
			return uuid.Nil, workspacesource.ErrScopeNotFound
		}
	default:
		return uuid.Nil, workspacesource.ErrScopeNotFound
	}
	var organization struct {
		Status   string
		IsDelete bool
	}
	read = tx.Raw(`SELECT status,is_delete FROM workspace.organization WHERE id=? FOR SHARE`, row.PhysicalScopeID).Scan(&organization)
	if read.Error != nil {
		return uuid.Nil, fmt.Errorf("lock source organization: %w", read.Error)
	}
	if read.RowsAffected != 1 || organization.Status != "active" || organization.IsDelete {
		return uuid.Nil, workspacesource.ErrScopeNotFound
	}
	return row.PhysicalScopeID, nil
}
