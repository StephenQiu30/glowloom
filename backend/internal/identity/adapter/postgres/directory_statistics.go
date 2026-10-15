package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// UserStatistics computes all account counters from the same filtered SQL snapshot.
func (s *PlatformDirectoryStore) UserStatistics(ctx context.Context, actor application.Principal, f application.DirectoryFilter, clock func() time.Time) (application.UserStatistics, error) {
	value := application.UserStatistics{AccountStates: map[string]int{"active": 0, "disabled": 0, "deleted": 0}}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockPlatformReader(tx, actor, clock); err != nil {
			return err
		}
		now := clock().UTC()
		value.AsOf = now
		var row struct{ Total, Verified, ActiveTeam, Memberships, Active, Disabled, Deleted int }
		q := filteredDirectory(tx, f, now)
		err := tx.Table("(?) AS matched", q).Select(`count(*) AS total,count(*) FILTER(WHERE verified) AS verified,count(*) FILTER(WHERE status='active' AND verified AND active_team_count>0) AS active_team,coalesce(sum((SELECT count(*) FROM workspace.team_member tm WHERE tm.user_id=matched.id)),0) AS memberships,count(*) FILTER(WHERE status='active') AS active,count(*) FILTER(WHERE status='disabled') AS disabled,count(*) FILTER(WHERE status='deleted') AS deleted`).Scan(&row).Error
		value.TotalUsers = row.Total
		value.VerifiedUsers = row.Verified
		value.ActiveTeamUsers = row.ActiveTeam
		value.MembershipCount = row.Memberships
		value.AccountStates = map[string]int{"active": row.Active, "disabled": row.Disabled, "deleted": row.Deleted}
		return err
	})
	return value, err
}

// MemberStatistics counts member states from one team-scoped filtered SQL snapshot.
func (s *PlatformDirectoryStore) MemberStatistics(ctx context.Context, actor application.Principal, team uuid.UUID, f application.ExportFilter, clock func() time.Time) (application.MemberStatistics, error) {
	value := application.MemberStatistics{TeamID: team, Filter: f, MemberStates: map[string]int{"active": 0, "suspended": 0, "removed": 0}}
	err := NewDirectoryExportStore(s.db).Transaction(ctx, func(tx application.ExportTx) error {
		if _, err := tx.Authorize(actor, &team, 0, false, clock); err != nil {
			return err
		}
		now := clock().UTC()
		value.AsOf = now
		var row struct{ Total, Active, Suspended, Removed int }
		err := memberExportQuery(tx.(*exportTx).db, team, f, now).Select(`count(*) AS total,count(*) FILTER(WHERE m.status='active') AS active,count(*) FILTER(WHERE m.status='suspended') AS suspended,count(*) FILTER(WHERE m.status='removed') AS removed`).Scan(&row).Error
		value.TotalMembers = row.Total
		value.MemberStates = map[string]int{"active": row.Active, "suspended": row.Suspended, "removed": row.Removed}
		return err
	})
	return value, err
}
