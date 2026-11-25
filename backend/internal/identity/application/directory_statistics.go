package application

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// UserStatistics counts mutually exclusive account states and separate memberships.
type UserStatistics struct {
	AsOf                                                        time.Time
	Filter                                                      ExportFilter
	TotalUsers, VerifiedUsers, ActiveTeamUsers, MembershipCount int
	AccountStates                                               map[string]int
}

// MemberStatistics counts memberships independently of account login availability.
type MemberStatistics struct {
	TeamID       uuid.UUID
	AsOf         time.Time
	Filter       ExportFilter
	TotalMembers int
	MemberStates map[string]int
}

// DirectoryStatisticsStore supplies a single consistent filtered aggregate.
type DirectoryStatisticsStore interface {
	UserStatistics(context.Context, Principal, DirectoryFilter, func() time.Time) (UserStatistics, error)
	MemberStatistics(context.Context, Principal, uuid.UUID, ExportFilter, func() time.Time) (MemberStatistics, error)
}

// DirectoryStatistics validates scope-specific filters before querying aggregates.
type DirectoryStatistics struct {
	store DirectoryStatisticsStore
	now   func() time.Time
}

// NewDirectoryStatistics injects existing storage and the clock.
func NewDirectoryStatistics(store DirectoryStatisticsStore, now func() time.Time) *DirectoryStatistics {
	return &DirectoryStatistics{store: store, now: now}
}

// Users applies the same filters as the account directory.
func (s *DirectoryStatistics) Users(ctx context.Context, a Principal, f ExportFilter) (UserStatistics, error) {
	f, err := normalizeExportFilter(false, f)
	if err != nil {
		return UserStatistics{}, err
	}
	value, err := s.store.UserStatistics(ctx, a, DirectoryFilter{Search: f.Search, Status: f.Status, PlatformRole: f.PlatformRole, Order: f.Order, Verified: f.Verified, InTeam: f.InTeam, From: f.From, To: f.To}, s.now)
	value.Filter = f
	return value, err
}

// Members restricts aggregates to an actual administrator of the named team.
func (s *DirectoryStatistics) Members(ctx context.Context, a Principal, team uuid.UUID, f ExportFilter) (MemberStatistics, error) {
	f, err := normalizeExportFilter(true, f)
	if err != nil {
		return MemberStatistics{}, err
	}
	return s.store.MemberStatistics(ctx, a, team, f, s.now)
}

// Statistics applies the directory's normalized filters without pagination.
func (d *PlatformDirectory) Statistics(ctx context.Context, a Principal, f DirectoryFilter) (UserStatistics, error) {
	f, err := normalizeDirectoryFilter(f)
	if err != nil || f.Cursor != "" {
		return UserStatistics{}, ErrInvalidDirectoryFilter
	}
	store, ok := d.store.(DirectoryStatisticsStore)
	if !ok {
		return UserStatistics{}, ErrExportUnavailable
	}
	return store.UserStatistics(ctx, a, f, d.now)
}
