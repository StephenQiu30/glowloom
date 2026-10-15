package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// EventStore supplies authorized security event reads.
type EventStore struct{ db *gorm.DB }

// NewEventStore constructs EventStore with explicit caller-supplied dependencies.
func NewEventStore(db *gorm.DB) *EventStore { return &EventStore{db: db} }

// ReadEvents rechecks current authority before querying the requested event scope.
func (s *EventStore) ReadEvents(ctx context.Context, actor application.Principal, f application.EventFilter, pos application.EventPosition, clock func() time.Time) ([]application.SecurityEvent, error) {
	items := make([]application.SecurityEvent, 0)
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		u, err := lockSelfActor(tx, actor, clock)
		if err != nil {
			return err
		}
		query := tx.Table("identity.security_event").Select("id,actor_id,target_user_id,team_id,action,result,reason,request_id,safe_diff,created_at").Where("created_at <= ?", pos.AsOf)
		switch f.ScopeKind {
		case "self":
			query = query.Where("target_user_id=? AND team_id IS NULL", u.ID)
		case "platform":
			if u.PlatformRole != "platform_admin" || u.PlatformEpoch != actor.PlatformEpoch {
				return application.ErrForbidden
			}
		case "team":
			if f.TeamID == nil {
				return application.ErrForbidden
			}
			var team struct {
				Status            string
				GovernanceBlocked bool
			}
			r := tx.Raw(`SELECT status,governance_blocked FROM workspace.team WHERE id=? FOR SHARE`, *f.TeamID).Scan(&team)
			if r.Error != nil {
				return r.Error
			}
			if r.RowsAffected != 1 || team.Status == "suspended" || team.GovernanceBlocked {
				return application.ErrForbidden
			}
			var member struct{ Role, Status string }
			r = tx.Raw(`SELECT role,status FROM workspace.team_member WHERE team_id=? AND user_id=? FOR SHARE`, *f.TeamID, u.ID).Scan(&member)
			if r.Error != nil {
				return r.Error
			}
			if r.RowsAffected != 1 || member.Role != "team_admin" || member.Status != "active" {
				return application.ErrForbidden
			}
			query = query.Where("scope_kind='team'")
		default:
			return application.ErrForbidden
		}
		if f.TeamID != nil {
			query = query.Where("team_id=?", *f.TeamID)
		}
		if f.TargetUserID != nil {
			query = query.Where("target_user_id=?", *f.TargetUserID)
		}
		if f.ActorID != nil {
			query = query.Where("actor_id=?", *f.ActorID)
		}
		if f.Action != "" {
			query = query.Where("action=?", f.Action)
		}
		if f.From != nil {
			query = query.Where("created_at >= ?", *f.From)
		}
		if f.To != nil {
			query = query.Where("created_at < ?", *f.To)
		}
		if pos.ID != uuid.Nil {
			query = query.Where("(created_at,id) < (?,?)", pos.CreatedAt, pos.ID)
		}
		var rows []struct {
			ID                                uuid.UUID
			ActorID, TargetUserID, TeamID     *uuid.UUID
			Action, Result, Reason, RequestID string
			SafeDiff                          []byte
			CreatedAt                         time.Time
		}
		if err := query.Order("created_at DESC,id DESC").Limit(f.Limit + 1).Find(&rows).Error; err != nil {
			return fmt.Errorf("query security events: %w", err)
		}
		for _, row := range rows {
			diff := map[string]string{}
			// Persistence may contain structured values. Project string fields only.
			var raw map[string]json.RawMessage
			if json.Unmarshal(row.SafeDiff, &raw) != nil {
				return fmt.Errorf("invalid security event metadata")
			}
			for key, value := range raw {
				var text string
				if json.Unmarshal(value, &text) == nil {
					diff[key] = text
				}
			}
			items = append(items, application.SecurityEvent{ID: row.ID, ActorID: row.ActorID, TargetUserID: row.TargetUserID, TeamID: row.TeamID, Action: row.Action, Result: row.Result, Reason: row.Reason, RequestID: row.RequestID, SafeDiff: diff, CreatedAt: row.CreatedAt})
		}
		return nil
	})
	return items, err
}
