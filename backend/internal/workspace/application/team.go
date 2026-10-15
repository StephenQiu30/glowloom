package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

var (
	// ErrTeamCreationClosed reports the current platform team-creation policy.
	ErrTeamCreationClosed = errors.New("team creation is closed")
	// ErrTeamCapacity rejects exceeding the accepted initial capacity boundaries.
	ErrTeamCapacity = errors.New("team capacity exceeded")
	// ErrTeamForbidden rejects a member without team governance authority.
	ErrTeamForbidden = errors.New("team governance forbidden")
	// ErrTeamRevisionConflict prevents stale changes from overwriting newer state.
	ErrTeamRevisionConflict = errors.New("team revision conflict")
	// ErrTeamStateConflict rejects invalid lifecycle transitions.
	ErrTeamStateConflict = errors.New("team state conflict")
	// ErrLastTeamAdministrator prevents leaving a team without an available administrator.
	ErrLastTeamAdministrator = errors.New("last team administrator must transfer duties")
	// ErrInvalidTeam rejects invalid team names, reasons and mutation inputs.
	ErrInvalidTeam = errors.New("invalid team input")
)

// Team contains current team facts; transports must omit the physical storage ID.
type Team struct {
	ID, ScopeID, PhysicalScopeID          uuid.UUID
	Name, Status, ActorRole               string
	Revision, MemberRevision, MemberCount int64
	InvitationEnabled, GovernanceBlocked  bool
	CreatedAt                             time.Time
	AvatarID                              *uuid.UUID
}

// TeamMember contains the role-appropriate directory projection.
type TeamMember struct {
	UserID                uuid.UUID
	DisplayName, Role     string
	AvatarID              *uuid.UUID
	Email, Status, Source *string
	Revision              *int64
	JoinedAt              time.Time
	AccessAvailable       *bool
	LastTeamActivityAt    *time.Time
}

// TeamMemberState holds locked membership and current account eligibility.
type TeamMemberState struct {
	UserID           uuid.UUID
	Role, Status     string
	Revision         int64
	AccountAvailable bool
}

// TeamInvitationRevoker owns invalidation of identity invitations in a team transaction.
type TeamInvitationRevoker interface {
	RevokePendingTeamInvitations(context.Context, uuid.UUID, time.Time) error
}

// TeamActorAccess supplies identity checks and invitation invalidation on one transaction.
type TeamActorAccess interface {
	TeamInvitationRevoker
	NotifyTeamAccounts(context.Context, []uuid.UUID, time.Time) error
	CheckSession(context.Context, identity.Principal, time.Time) error
	CheckPlatformSession(context.Context, identity.Principal, time.Time) error
	CheckRecentSession(context.Context, identity.Principal, time.Time) error
}

// TeamTx keeps team, membership and receipt writes atomic under governance locks.
type TeamTx interface {
	RequireRecentAuthentication() error
	RequirePlatform() error
	LoadPlatformTeam(uuid.UUID) (PlatformTeam, error)
	QueryPlatformTeams(PlatformTeamFilter, PlatformTeamPosition) ([]PlatformTeam, error)
	SavePlatformTeam(PlatformTeam, int64, time.Time) error
	ValidateTeamAvatar(uuid.UUID) error
	BoundTeamAvatar(uuid.UUID, *uuid.UUID, bool) (TeamAvatarObject, error)
	Spaces() ([]Space, error)
	CreationEnabled() bool
	CreationCounts() (created, joined int64, err error)
	CreationReceipt(uuid.UUID, string) (Team, bool, error)
	SaveCreationReceipt(uuid.UUID, string, Team, time.Time) error
	InsertTeam(string, *uuid.UUID, time.Time) (Team, error)
	LoadTeam(uuid.UUID) (Team, error)
	SaveTeam(Team, int64, time.Time) error
	CanArchive(uuid.UUID) error
	CanRestore(uuid.UUID) error
	RevokePendingInvitations(uuid.UUID, time.Time) error
	Members(uuid.UUID, bool) ([]TeamMember, error)
	QueryMembers(uuid.UUID, bool, MemberFilter, MemberPosition) ([]TeamMember, error)
	Member(uuid.UUID, uuid.UUID) (TeamMemberState, error)
	AvailableAdmins(uuid.UUID) (int64, error)
	SaveMember(uuid.UUID, TeamMemberState, int64, time.Time) error
	AppendTeamEvent(uuid.UUID, *uuid.UUID, string, string, time.Time) error
}

// Space describes a selectable personal or team scope without physical storage keys.
type Space struct {
	ID               uuid.UUID
	Kind, Name, Role string
	TeamID           *uuid.UUID
	Writable         bool
}

// Spaces returns only current personal ownership and active team memberships.
func (s *Teams) Spaces(ctx context.Context, actor identity.Principal) ([]Space, error) {
	var result []Space
	err := s.store.TeamTransaction(ctx, actor, s.now().UTC(), func(tx TeamTx) error { var err error; result, err = tx.Spaces(); return err })
	return result, err
}

// ChangeRole requires recent password proof and protects the last administrator.
func (s *Teams) ChangeRole(ctx context.Context, actor identity.Principal, teamID, userID uuid.UUID, revision int64, role, reason string) error {
	reason = strings.TrimSpace(reason)
	if revision < 1 || (role != "member" && role != "team_admin") || !validTeamText(reason, 5, 500) {
		return ErrInvalidTeam
	}
	now := s.now().UTC()
	return s.store.TeamTransaction(ctx, actor, now, func(tx TeamTx) error {
		team, err := tx.LoadTeam(teamID)
		if err != nil {
			return err
		}
		if team.ActorRole != "team_admin" {
			return ErrTeamForbidden
		}
		if team.Status != "active" || team.GovernanceBlocked {
			return ErrTeamStateConflict
		}
		if err := tx.RequireRecentAuthentication(); err != nil {
			return err
		}
		member, err := tx.Member(teamID, userID)
		if err != nil {
			return err
		}
		if member.Revision != revision {
			return ErrTeamRevisionConflict
		}
		if member.Status != "active" || !member.AccountAvailable {
			return ErrTeamStateConflict
		}
		if member.Role == role {
			return nil
		}
		if role == "member" {
			if err := preserveTeamAdministrator(tx, teamID, member, "removed"); err != nil {
				return err
			}
		}
		member.Role = role
		member.Revision++
		if err := tx.SaveMember(teamID, member, revision, now); err != nil {
			return err
		}
		return tx.AppendTeamEvent(teamID, &userID, "member.role_changed", reason, now)
	})
}

// TeamStore binds current identity, settings and team state to one transaction.
type TeamStore interface {
	TeamTransaction(context.Context, identity.Principal, time.Time, func(TeamTx) error) error
}

// Teams implements team creation, directories and membership lifecycle.
type Teams struct {
	store TeamStore
	now   func() time.Time
}

// NewTeams injects the existing team store and server clock.
func NewTeams(store TeamStore, now func() time.Time) *Teams { return &Teams{store: store, now: now} }

func validTeamText(text string, minimum, maximum int) bool {
	if !utf8.ValidString(text) || utf8.RuneCountInString(text) < minimum || utf8.RuneCountInString(text) > maximum {
		return false
	}
	for _, r := range text {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// Create atomically creates a team, its first administrator and its content scope.
func (s *Teams) Create(ctx context.Context, actor identity.Principal, key uuid.UUID, name string) (Team, error) {
	return s.CreateWithAvatar(ctx, actor, key, name, nil)
}

// CreateWithAvatar atomically validates and binds an optional normalized avatar.
func (s *Teams) CreateWithAvatar(ctx context.Context, actor identity.Principal, key uuid.UUID, name string, avatar *uuid.UUID) (Team, error) {
	name = strings.TrimSpace(name)
	if key == uuid.Nil || !validTeamText(name, 2, 50) || avatar != nil && *avatar == uuid.Nil {
		return Team{}, ErrInvalidTeam
	}
	payload, _ := json.Marshal([]any{name, avatar})
	sum := sha256.Sum256(payload)
	hash := hex.EncodeToString(sum[:])
	now := s.now().UTC()
	var result Team
	err := s.store.TeamTransaction(ctx, actor, now, func(tx TeamTx) error {
		prior, found, err := tx.CreationReceipt(key, hash)
		if err != nil {
			return err
		}
		if found {
			if _, err := tx.LoadTeam(prior.ID); err != nil {
				return err
			}
			result = prior
			return nil
		}
		if !tx.CreationEnabled() {
			return ErrTeamCreationClosed
		}
		created, joined, err := tx.CreationCounts()
		if err != nil {
			return err
		}
		if created >= 5 || joined >= 20 {
			return ErrTeamCapacity
		}
		if avatar != nil {
			if err := tx.ValidateTeamAvatar(*avatar); err != nil {
				return err
			}
		}
		result, err = tx.InsertTeam(name, avatar, now)
		if err != nil {
			return err
		}
		if err := tx.AppendTeamEvent(result.ID, &actor.ID, "team.created", "", now); err != nil {
			return err
		}
		return tx.SaveCreationReceipt(key, hash, result, now)
	})
	return result, err
}

// Read exposes a team only to its current active members.
func (s *Teams) Read(ctx context.Context, actor identity.Principal, id uuid.UUID) (Team, error) {
	var result Team
	err := s.store.TeamTransaction(ctx, actor, s.now().UTC(), func(tx TeamTx) error { var err error; result, err = tx.LoadTeam(id); return err })
	return result, err
}

// Members excludes email and governance fields for ordinary members.
// The active directory is bounded by the team's 100-seat capacity.
func (s *Teams) Members(ctx context.Context, actor identity.Principal, id uuid.UUID) ([]TeamMember, error) {
	var result []TeamMember
	err := s.store.TeamTransaction(ctx, actor, s.now().UTC(), func(tx TeamTx) error {
		team, err := tx.LoadTeam(id)
		if err != nil {
			return err
		}
		result, err = tx.Members(id, team.ActorRole == "team_admin")
		return err
	})
	return result, err
}

// Rename applies a versioned team-name change by a current administrator.
func (s *Teams) Rename(ctx context.Context, actor identity.Principal, id uuid.UUID, revision int64, name string) (Team, error) {
	name = strings.TrimSpace(name)
	if !validTeamText(name, 2, 50) || revision < 1 {
		return Team{}, ErrInvalidTeam
	}
	var result Team
	now := s.now().UTC()
	err := s.store.TeamTransaction(ctx, actor, now, func(tx TeamTx) error {
		team, err := tx.LoadTeam(id)
		if err != nil {
			return err
		}
		if team.ActorRole != "team_admin" {
			return ErrTeamForbidden
		}
		if team.Status != "active" || team.GovernanceBlocked {
			return ErrTeamStateConflict
		}
		if team.Revision != revision {
			return ErrTeamRevisionConflict
		}
		team.Name = name
		team.Revision++
		if err := tx.SaveTeam(team, revision, now); err != nil {
			return err
		}
		result = team
		return tx.AppendTeamEvent(id, nil, "team.updated", "", now)
	})
	return result, err
}

// ChangeMember suspends, restores or removes one membership without touching the account.
func (s *Teams) ChangeMember(ctx context.Context, actor identity.Principal, teamID, userID uuid.UUID, revision int64, action, reason string) error {
	reason = strings.TrimSpace(reason)
	if revision < 1 || !validTeamText(reason, 5, 500) || (action != "suspend" && action != "restore" && action != "remove") {
		return ErrInvalidTeam
	}
	now := s.now().UTC()
	return s.store.TeamTransaction(ctx, actor, now, func(tx TeamTx) error {
		team, err := tx.LoadTeam(teamID)
		if err != nil {
			return err
		}
		if team.ActorRole != "team_admin" {
			return ErrTeamForbidden
		}
		if team.Status != "active" || team.GovernanceBlocked {
			return ErrTeamStateConflict
		}
		member, err := tx.Member(teamID, userID)
		if err != nil {
			return err
		}
		if member.Revision != revision {
			return ErrTeamRevisionConflict
		}
		next := "removed"
		switch action {
		case "suspend":
			if member.Status != "active" {
				return ErrTeamStateConflict
			}
			next = "suspended"
		case "restore":
			if member.Status != "suspended" {
				return ErrTeamStateConflict
			}
			next = "active"
		case "remove":
			if member.Status == "removed" {
				return ErrTeamStateConflict
			}
		}
		if err := preserveTeamAdministrator(tx, teamID, member, next); err != nil {
			return err
		}
		member.Status = next
		member.Revision++
		if err := tx.SaveMember(teamID, member, revision, now); err != nil {
			return err
		}
		return tx.AppendTeamEvent(teamID, &userID, "member."+action, reason, now)
	})
}

func preserveTeamAdministrator(tx TeamTx, team uuid.UUID, member TeamMemberState, next string) error {
	if member.Role == "team_admin" && member.Status == "active" && member.AccountAvailable && next != "active" {
		count, err := tx.AvailableAdmins(team)
		if err != nil {
			return err
		}
		if count <= 1 {
			return ErrLastTeamAdministrator
		}
	}
	return nil
}

// Leave removes only the requesting membership and retains all team resources.
func (s *Teams) Leave(ctx context.Context, actor identity.Principal, id uuid.UUID, revision int64) error {
	if revision < 1 {
		return ErrInvalidTeam
	}
	now := s.now().UTC()
	return s.store.TeamTransaction(ctx, actor, now, func(tx TeamTx) error {
		if _, err := tx.LoadTeam(id); err != nil {
			return err
		}
		member, err := tx.Member(id, actor.ID)
		if err != nil {
			return err
		}
		if member.Revision != revision {
			return ErrTeamRevisionConflict
		}
		if err := preserveTeamAdministrator(tx, id, member, "removed"); err != nil {
			return err
		}
		member.Status = "removed"
		member.Revision++
		if err := tx.SaveMember(id, member, revision, now); err != nil {
			return err
		}
		return tx.AppendTeamEvent(id, &actor.ID, "member.left", "", now)
	})
}

// Archive switches the team to read-only after checking pending work and transfers.
func (s *Teams) Archive(ctx context.Context, actor identity.Principal, id uuid.UUID, revision int64, reason string) error {
	return s.lifecycle(ctx, actor, id, revision, reason, "archived", nil)
}

// Restore reopens only an archived team; platform suspension is independent.
func (s *Teams) Restore(ctx context.Context, actor identity.Principal, id uuid.UUID, revision int64, reason string) error {
	return s.lifecycle(ctx, actor, id, revision, reason, "active", nil)
}

// SetInvitations changes the current team's invitation policy.
func (s *Teams) SetInvitations(ctx context.Context, actor identity.Principal, id uuid.UUID, revision int64, enabled bool) error {
	return s.lifecycle(ctx, actor, id, revision, "", "", &enabled)
}
func (s *Teams) lifecycle(ctx context.Context, actor identity.Principal, id uuid.UUID, revision int64, reason, status string, invites *bool) error {
	reason = strings.TrimSpace(reason)
	if revision < 1 || (invites == nil && !validTeamText(reason, 5, 500)) {
		return ErrInvalidTeam
	}
	now := s.now().UTC()
	return s.store.TeamTransaction(ctx, actor, now, func(tx TeamTx) error {
		team, err := tx.LoadTeam(id)
		if err != nil {
			return err
		}
		if team.ActorRole != "team_admin" {
			return ErrTeamForbidden
		}
		if team.GovernanceBlocked {
			return ErrTeamStateConflict
		}
		if team.Revision != revision {
			return ErrTeamRevisionConflict
		}
		if invites == nil {
			if err := tx.RequireRecentAuthentication(); err != nil {
				return err
			}
			if (status == "archived" && team.Status != "active") || (status == "active" && team.Status != "archived") {
				return ErrTeamStateConflict
			}
			if status == "archived" {
				if err := tx.CanArchive(id); err != nil {
					return err
				}
			}
			if status == "active" {
				if err := tx.CanRestore(id); err != nil {
					return err
				}
			}
			team.Status = status
		} else {
			if team.Status != "active" {
				return ErrTeamStateConflict
			}
			if !*invites {
				if err := tx.RevokePendingInvitations(id, now); err != nil {
					return err
				}
			}
			team.InvitationEnabled = *invites
		}
		team.Revision++
		if err := tx.SaveTeam(team, revision, now); err != nil {
			return err
		}
		return tx.AppendTeamEvent(id, nil, "team.settings_changed", reason, now)
	})
}

// TeamAvatarAccess validates a normalized image owned by the acting administrator.
type TeamAvatarAccess interface {
	TeamAvatarReader
	Validate(context.Context, uuid.UUID, uuid.UUID) error
}
