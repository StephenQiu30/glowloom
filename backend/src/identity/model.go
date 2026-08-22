package identity

import (
	"errors"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
)

type AccountStatus string

const (
	AccountStatusActive    AccountStatus = "active"
	AccountStatusSuspended AccountStatus = "suspended"
	AccountStatusRemoved   AccountStatus = "removed"
)

type MembershipStatus string

const (
	MembershipStatusInvited   MembershipStatus = "invited"
	MembershipStatusActive    MembershipStatus = "active"
	MembershipStatusSuspended MembershipStatus = "suspended"
	MembershipStatusRemoved   MembershipStatus = "removed"
)

type RoleCode string

const (
	RoleAdmin RoleCode = "admin"
	RoleUser  RoleCode = "user"
	RoleBan   RoleCode = "ban"
)

func (r RoleCode) IsValid() bool {
	switch r {
	case RoleAdmin, RoleUser, RoleBan:
		return true
	default:
		return false
	}
}

func (r RoleCode) IsAdmin() bool {
	return r == RoleAdmin
}

func roleScopeForCode(role RoleCode) (RoleScope, bool) {
	switch role {
	case RoleAdmin, RoleUser, RoleBan:
		return RoleScopeWorkspace, true
	default:
		return "", false
	}
}

type RoleScope string

const (
	RoleScopeWorkspace RoleScope = "workspace"
)

func (s MembershipStatus) IsManageable() bool {
	return s == MembershipStatusActive || s == MembershipStatusSuspended || s == MembershipStatusRemoved
}

type IdentityAction string

const (
	IdentityActionRegister IdentityAction = "register"
	IdentityActionLogin    IdentityAction = "login"
	IdentityActionRefresh  IdentityAction = "refresh"
)

type AccessTokenType string

const AccessTokenTypeAccess AccessTokenType = "access"

const BearerAuthScheme = "Bearer"

type RatePolicy struct {
	Limit  int64
	Period time.Duration
	Burst  int64
}

var identityRatePolicies = map[IdentityAction]RatePolicy{
	IdentityActionRegister: {Limit: 5, Period: time.Minute, Burst: 2},
	IdentityActionLogin:    {Limit: 10, Period: time.Minute, Burst: 5},
	IdentityActionRefresh:  {Limit: 30, Period: time.Minute, Burst: 5},
}

const (
	identityRedisPrefix        = "lanverse:identity:"
	identityRatePrefix         = identityRedisPrefix + "rate:"
	identityRefreshLockPrefix  = identityRedisPrefix + "refresh-lock:"
	identityRevokedPrefix      = identityRedisPrefix + "revoked:"
	identityRefreshLockTTL     = 5 * time.Second
	identitySubjectEmailPrefix = "email:"
	identityRevokedValue       = "revoked"
)

type EmailAddress string

func ParseEmailAddress(value string) (EmailAddress, error) {
	email := strings.ToLower(strings.TrimSpace(value))
	if email == "" || len([]byte(email)) > 254 || strings.ContainsAny(email, "\r\n \t") {
		return "", errors.New("invalid email")
	}
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email || !strings.Contains(email, "@") {
		return "", errors.New("invalid email")
	}
	return EmailAddress(email), nil
}

func (e EmailAddress) String() string { return string(e) }

type Principal struct {
	UserID       uuid.UUID
	WorkspaceID  uuid.UUID
	MembershipID uuid.UUID
	SessionID    uuid.UUID
	Role         RoleCode
}

type Account struct {
	ID          uuid.UUID `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
}

type Workspace struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

type AuthIdentity struct {
	Account      Account
	Workspace    Workspace
	MembershipID uuid.UUID
	Role         RoleCode
}

// SessionIssue is an internal hand-off between the PostgreSQL repository and
// the service. RefreshToken must never be serialized in an API response.
type SessionIssue struct {
	SessionID         uuid.UUID
	FamilyID          uuid.UUID
	PreviousSessionID uuid.UUID
	Identity          AuthIdentity
	RefreshToken      string
	RefreshExpiresAt  time.Time
}

type AuthResponse struct {
	AccessToken string    `json:"access_token"`
	TokenType   string    `json:"token_type"`
	ExpiresAt   time.Time `json:"expires_at"`
	User        Account   `json:"user"`
	Workspace   Workspace `json:"workspace"`
	Role        RoleCode  `json:"role"`
}

// AuthResponseEnvelope 是 Swagger 注释使用的认证响应模型。
type AuthResponseEnvelope struct {
	Data AuthResponse `json:"data"`
}

type CurrentIdentity struct {
	UserID       uuid.UUID `json:"user_id"`
	WorkspaceID  uuid.UUID `json:"workspace_id"`
	MembershipID uuid.UUID `json:"membership_id"`
	SessionID    uuid.UUID `json:"session_id"`
	Role         RoleCode  `json:"role"`
}

// CurrentIdentityEnvelope 是 Swagger 注释使用的当前身份响应模型。
type CurrentIdentityEnvelope struct {
	Data CurrentIdentity `json:"data"`
}

type WorkspaceMember struct {
	MembershipID     uuid.UUID        `json:"membership_id"`
	UserID           uuid.UUID        `json:"user_id"`
	Email            string           `json:"email"`
	DisplayName      string           `json:"display_name"`
	AccountStatus    AccountStatus    `json:"account_status"`
	MembershipStatus MembershipStatus `json:"membership_status"`
	Role             RoleCode         `json:"role"`
	CreatedAt        time.Time        `json:"created_at"`
}

type WorkspaceMemberPage struct {
	Items    []WorkspaceMember `json:"items"`
	Total    int64             `json:"total"`
	Page     int               `json:"page"`
	PageSize int               `json:"page_size"`
}

// WorkspaceMemberPageEnvelope 是 Swagger 注释使用的成员列表响应模型。
type WorkspaceMemberPageEnvelope struct {
	Data WorkspaceMemberPage `json:"data"`
}

// WorkspaceMemberEnvelope 是 Swagger 注释使用的成员响应模型。
type WorkspaceMemberEnvelope struct {
	Data WorkspaceMember `json:"data"`
}

type WorkspaceMemberQuery struct {
	Search   string
	Page     int
	PageSize int
}

type WorkspaceMemberUpdate struct {
	Role   *RoleCode
	Status *MembershipStatus
}
