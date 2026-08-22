package identity

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/stephenqiu30/lanverse/backend/src/platform/database"
	"github.com/stephenqiu30/lanverse/backend/src/platform/httpapi"
	"github.com/stephenqiu30/lanverse/backend/src/platform/toolkit"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrEmailRegistered    = errors.New("email is already registered")
	ErrRefreshInvalid     = errors.New("refresh session is invalid")
	ErrRefreshReplay      = errors.New("refresh token replay detected")
)

type RefreshReplayError struct {
	SessionID uuid.UUID
	FamilyID  uuid.UUID
}

func (e *RefreshReplayError) Error() string { return ErrRefreshReplay.Error() }
func (e *RefreshReplayError) Unwrap() error { return ErrRefreshReplay }

type IdentityService struct {
	repository IdentityStore
	cache      IdentityCache
	jwt        *JWTManager
	config     AuthConfig
}

func NewIdentityService(repository IdentityStore, cache IdentityCache, jwtManager *JWTManager, config AuthConfig) *IdentityService {
	return &IdentityService{repository: repository, cache: cache, jwt: jwtManager, config: config}
}

func (s *IdentityService) Register(ctx context.Context, input RegisterInput, remoteIP string) (AuthResponse, SessionIssue, error) {
	email, displayName, workspaceName, err := validateRegistration(input)
	if err != nil {
		return AuthResponse{}, SessionIssue{}, err
	}
	if err := s.allow(ctx, IdentityActionRegister, email.String(), remoteIP); err != nil {
		return AuthResponse{}, SessionIssue{}, err
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return AuthResponse{}, SessionIssue{}, fmt.Errorf("hash registration password: %w", err)
	}
	issue, err := s.repository.RegisterAccount(ctx, PersistedRegisterInput{Email: email, PasswordHash: string(passwordHash), DisplayName: displayName, Workspace: workspaceName})
	if errors.Is(err, ErrEmailRegistered) {
		return AuthResponse{}, SessionIssue{}, httpapi.Conflict("邮箱已注册", "直接登录或使用其他邮箱")
	}
	if err != nil {
		return AuthResponse{}, SessionIssue{}, err
	}
	response, err := s.issueAccessToken(issue)
	return response, issue, err
}

func (s *IdentityService) Login(ctx context.Context, email, password string, workspaceID uuid.UUID, remoteIP string) (AuthResponse, SessionIssue, error) {
	emailAddress, err := ParseEmailAddress(email)
	if err != nil || password == "" || workspaceID == uuid.Nil {
		return AuthResponse{}, SessionIssue{}, httpapi.Validation("邮箱、密码和 Workspace 不能为空", "提供有效的邮箱、密码和 Workspace 后重试")
	}
	if err := s.allow(ctx, IdentityActionLogin, emailAddress.String(), remoteIP); err != nil {
		return AuthResponse{}, SessionIssue{}, err
	}
	account, err := s.repository.FindLoginAccount(database.WithWorkspaceID(ctx, workspaceID), emailAddress, workspaceID)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			return AuthResponse{}, SessionIssue{}, httpapi.NewError(httpapi.StatusUnauthorized, httpapi.CodeUnauthorized, "邮箱或密码错误", "确认凭据后重试")
		}
		return AuthResponse{}, SessionIssue{}, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(account.PasswordHash), []byte(password)); err != nil {
		return AuthResponse{}, SessionIssue{}, httpapi.NewError(httpapi.StatusUnauthorized, httpapi.CodeUnauthorized, "邮箱或密码错误", "确认凭据后重试")
	}
	issue, err := s.repository.CreateSession(database.WithWorkspaceID(ctx, workspaceID), account.Identity)
	if err != nil {
		return AuthResponse{}, SessionIssue{}, err
	}
	response, err := s.issueAccessToken(issue)
	return response, issue, err
}

func (s *IdentityService) Refresh(ctx context.Context, refreshToken string, workspaceID uuid.UUID, remoteIP string) (AuthResponse, SessionIssue, error) {
	if strings.TrimSpace(refreshToken) == "" || workspaceID == uuid.Nil {
		return AuthResponse{}, SessionIssue{}, httpapi.NewError(httpapi.StatusUnauthorized, httpapi.CodeUnauthorized, "刷新会话缺失", "重新登录后重试")
	}
	if err := s.allow(ctx, IdentityActionRefresh, hashSecret(refreshToken), remoteIP); err != nil {
		return AuthResponse{}, SessionIssue{}, err
	}
	lockKey := identityRefreshLockPrefix + hashSecret(refreshToken)
	lockValue := uuid.NewString()
	locked, err := s.cache.IdentitySetNX(ctx, lockKey, lockValue, identityRefreshLockTTL)
	if err != nil {
		return AuthResponse{}, SessionIssue{}, dependencyUnavailable("Redis 刷新锁不可用", err)
	}
	if !locked {
		return AuthResponse{}, SessionIssue{}, httpapi.RateLimited(1, "刷新请求正在处理中", "稍后重试或等待当前页面完成恢复")
	}
	defer func() { _ = s.cache.IdentityCompareAndDelete(context.Background(), lockKey, lockValue) }()

	issue, err := s.repository.RotateRefreshSession(database.WithWorkspaceID(ctx, workspaceID), workspaceID, refreshToken)
	if err != nil {
		var replay *RefreshReplayError
		if errors.As(err, &replay) && replay.SessionID != uuid.Nil {
			_ = s.markRevoked(ctx, replay.SessionID, s.config.RefreshTTL)
			return AuthResponse{}, SessionIssue{}, httpapi.NewError(httpapi.StatusUnauthorized, httpapi.CodeUnauthorized, "刷新会话已失效", "重新登录后重试")
		}
		if errors.Is(err, ErrRefreshInvalid) {
			return AuthResponse{}, SessionIssue{}, httpapi.NewError(httpapi.StatusUnauthorized, httpapi.CodeUnauthorized, "刷新会话已失效", "重新登录后重试")
		}
		return AuthResponse{}, SessionIssue{}, err
	}
	if issue.PreviousSessionID != uuid.Nil {
		if err := s.markRevoked(ctx, issue.PreviousSessionID, s.config.RefreshTTL); err != nil {
			return AuthResponse{}, SessionIssue{}, err
		}
	}
	response, err := s.issueAccessToken(issue)
	return response, issue, err
}

func (s *IdentityService) Logout(ctx context.Context, refreshToken string, workspaceID uuid.UUID) error {
	if strings.TrimSpace(refreshToken) == "" {
		return nil
	}
	sessionID, err := s.repository.RevokeRefreshSession(database.WithWorkspaceID(ctx, workspaceID), workspaceID, refreshToken)
	if errors.Is(err, ErrRefreshInvalid) {
		return nil
	}
	if err != nil {
		return err
	}
	if sessionID != uuid.Nil {
		return s.markRevoked(ctx, sessionID, s.config.RefreshTTL)
	}
	return nil
}

func (s *IdentityService) Authenticate(ctx context.Context, rawAccessToken string, workspaceID uuid.UUID) (Principal, error) {
	claims, err := s.jwt.Parse(rawAccessToken)
	if err != nil {
		return Principal{}, httpapi.NewError(httpapi.StatusUnauthorized, httpapi.CodeUnauthorized, "访问令牌无效", "刷新登录会话后重试")
	}
	claimWorkspace, _ := uuid.Parse(claims.WorkspaceID)
	if workspaceID == uuid.Nil || claimWorkspace != workspaceID {
		return Principal{}, httpapi.NewError(httpapi.StatusUnauthorized, httpapi.CodeUnauthorized, "访问令牌与 Workspace 不匹配", "切换到令牌所属 Workspace 后重试")
	}
	sessionID, _ := uuid.Parse(claims.SessionID)
	userID, _ := uuid.Parse(claims.Subject)
	if _, revoked, err := s.cache.IdentityGet(ctx, revokedKey(sessionID)); err != nil {
		return Principal{}, dependencyUnavailable("Redis 会话状态不可用", err)
	} else if revoked {
		return Principal{}, httpapi.NewError(httpapi.StatusUnauthorized, httpapi.CodeUnauthorized, "登录会话已撤销", "刷新登录会话后重试")
	}
	principal, err := s.repository.Authenticate(database.WithWorkspaceID(ctx, workspaceID), userID, sessionID, workspaceID)
	if err != nil {
		if apiErr := httpapi.From(err); apiErr.Status == httpapi.StatusServiceUnavailable {
			return Principal{}, apiErr
		}
		return Principal{}, httpapi.NewError(httpapi.StatusUnauthorized, httpapi.CodeUnauthorized, "登录会话已失效", "刷新登录会话后重试")
	}
	return principal, nil
}

func (s *IdentityService) AuthorizePath(ctx context.Context, workspaceID uuid.UUID, path string) error {
	return s.repository.AuthorizePath(database.WithWorkspaceID(ctx, workspaceID), workspaceID, path)
}

func (s *IdentityService) issueAccessToken(issue SessionIssue) (AuthResponse, error) {
	accessToken, expiresAt, err := s.jwt.Issue(issue, time.Now().UTC())
	if err != nil {
		return AuthResponse{}, err
	}
	return AuthResponse{AccessToken: accessToken, TokenType: BearerAuthScheme, ExpiresAt: expiresAt, User: issue.Identity.Account, Workspace: issue.Identity.Workspace, Role: issue.Identity.Role}, nil
}

func (s *IdentityService) allow(ctx context.Context, action IdentityAction, subject, remoteIP string) error {
	policy, ok := identityRatePolicies[action]
	if !ok {
		return fmt.Errorf("identity rate policy is missing for %q", action)
	}
	allowed, retryAfter, _, err := s.cache.AllowIdentityGCRA(ctx, identityRatePrefix+string(action)+":"+hashSecret(subject)+":"+hashSecret(remoteIP), policy.Limit, policy.Period, policy.Burst)
	if err != nil {
		return dependencyUnavailable("Redis 身份限流不可用", err)
	}
	if !allowed {
		seconds := int(retryAfter.Round(time.Second) / time.Second)
		if seconds < 1 {
			seconds = 1
		}
		return httpapi.RateLimited(seconds, "请求过于频繁", "等待限流窗口恢复后重试")
	}
	return nil
}

func (s *IdentityService) markRevoked(ctx context.Context, sessionID uuid.UUID, ttl time.Duration) error {
	if sessionID == uuid.Nil {
		return nil
	}
	if err := s.cache.IdentitySet(ctx, revokedKey(sessionID), identityRevokedValue, ttl); err != nil {
		return dependencyUnavailable("Redis 撤销状态不可用", err)
	}
	return nil
}

func dependencyUnavailable(message string, cause error) error {
	return httpapi.Wrap(cause, httpapi.StatusServiceUnavailable, httpapi.CodeDependencyUnavailable, message, "稍后重试；Redis 恢复后再进行身份操作")
}

func revokedKey(sessionID uuid.UUID) string {
	return identityRevokedPrefix + hashSecret(sessionID.String())
}

func hashSecret(value string) string {
	return toolkit.SHA256String(value)
}

func validateRegistration(input RegisterInput) (EmailAddress, string, string, error) {
	email, err := ParseEmailAddress(input.Email)
	if err != nil {
		return "", "", "", httpapi.Validation("邮箱格式无效", "提供有效邮箱后重试")
	}
	if len([]byte(input.Password)) < 12 || len([]byte(input.Password)) > 72 {
		return "", "", "", httpapi.Validation("密码长度必须为 12—72 字节", "使用更长且不超过 bcrypt 限制的密码")
	}
	displayName := strings.TrimSpace(input.DisplayName)
	if displayName == "" {
		displayName = strings.Split(email.String(), "@")[0]
	}
	if len([]rune(displayName)) < 1 || len([]rune(displayName)) > 160 {
		return "", "", "", httpapi.Validation("显示名长度必须为 1—160 个字符", "修改显示名后重试")
	}
	workspaceName := strings.TrimSpace(input.Workspace)
	if len([]rune(workspaceName)) < 1 || len([]rune(workspaceName)) > 120 {
		return "", "", "", httpapi.Validation("Workspace 名称长度必须为 1—120 个字符", "修改 Workspace 名称后重试")
	}
	return email, displayName, workspaceName, nil
}
