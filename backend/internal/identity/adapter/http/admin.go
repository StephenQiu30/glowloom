package http

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// AdminDependencies binds only the existing audited account use cases.
type AdminDependencies struct {
	List    *application.ListUsersQuery
	Create  *application.CreateUserCommand
	Update  *application.UpdateUserCommand
	Disable *application.DisableUserCommand
	Enable  *application.EnableUserCommand
	Reset   *application.ResetPasswordCommand
	Unlock  *application.UnlockUserCommand
}

// AdminHandler serves the organization account directory behind session authentication.
type AdminHandler struct{ deps AdminDependencies }

// NewAdminHandler injects the account commands without changing their transactions.
func NewAdminHandler(deps AdminDependencies) *AdminHandler { return &AdminHandler{deps: deps} }

// Register installs account management under the existing administrator session guard.
func (h *AdminHandler) Register(group *gin.RouterGroup) {
	g := group.Group("/admin/users")
	g.Use(func(c *gin.Context) {
		c.Header("Cache-Control", "private, no-store")
		a := Principal(c)
		if a.ID == uuid.Nil || a.OrgID == uuid.Nil || a.Role != domain.RoleAdmin || a.MustChangePassword {
			httpapi.WriteProblem(c, 403, "forbidden", nil)
			return
		}
		c.Next()
	})
	g.GET("", h.List)
	g.POST("", h.Create)
	g.PUT("/:id", h.Update)
	g.POST("/:id/disable", h.Disable)
	g.POST("/:id/enable", h.Enable)
	g.POST("/:id/password", h.ResetPassword)
	g.POST("/:id/unlock", h.Unlock)
}

// AdminUser exposes only safe directory fields using the public creator role.
type AdminUser struct {
	ID          uuid.UUID  `json:"id"`
	LoginName   string     `json:"login_name"`
	DisplayName string     `json:"display_name"`
	Role        string     `json:"role" enums:"creator,admin"`
	Status      string     `json:"status" enums:"active,disabled"`
	LoginLocked bool       `json:"login_locked"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
	Revision    int64      `json:"revision"`
	CreateTime  time.Time  `json:"create_time"`
}

// AdminUserPage keeps pagination scoped to the current administrator and organization.
type AdminUserPage struct {
	Items      []AdminUser `json:"items"`
	NextCursor string      `json:"next_cursor,omitempty"`
}
type userCursor struct {
	Actor, Org, ID uuid.UUID
	Time           time.Time
	Limit          int
}

// List returns a bounded directory page.
// @ID listAdminUsers
// @Produce json
// @Param limit query int false "1..200, default 50"
// @Param cursor query string false "Actor and organization scoped cursor"
// @Success 200 {object} AdminUserPage
// @Failure 403 {object} httpapi.Problem
// @Router /api/admin/users [get]
func (h *AdminHandler) List(c *gin.Context) {
	actor := Principal(c)
	limit := 50
	if raw := c.Query("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 200 {
			httpapi.WriteProblem(c, 422, "invalid_request", nil)
			return
		}
		limit = n
	}
	input := application.ListUsersInput{Limit: limit}
	if raw := c.Query("cursor"); raw != "" {
		var cursor userCursor
		decoded, err := base64.RawURLEncoding.DecodeString(raw)
		if len(raw) > 1024 || err != nil || json.Unmarshal(decoded, &cursor) != nil || cursor.ID == uuid.Nil || cursor.Time.IsZero() || cursor.Actor != actor.ID || cursor.Org != actor.OrgID || cursor.Limit != limit {
			httpapi.WriteProblem(c, 422, "invalid_cursor", nil)
			return
		}
		input.After = &application.UserListCursor{ID: cursor.ID, CreateTime: cursor.Time}
	}
	page, err := h.deps.List.Execute(c.Request.Context(), actor, input)
	if err != nil {
		adminProblem(c, err)
		return
	}
	result := AdminUserPage{Items: make([]AdminUser, 0, len(page.Users))}
	for _, u := range page.Users {
		result.Items = append(result.Items, AdminUser{ID: u.ID, LoginName: u.LoginName, DisplayName: u.DisplayName, Role: publicRole(u.Role), Status: string(u.Status), LoginLocked: u.LoginLocked, LastLoginAt: u.LastLoginAt, Revision: u.Revision, CreateTime: u.CreateTime})
	}
	if page.Next != nil {
		encoded, err := json.Marshal(userCursor{Actor: actor.ID, Org: actor.OrgID, ID: page.Next.ID, Time: page.Next.CreateTime, Limit: limit})
		if err != nil {
			adminProblem(c, err)
			return
		}
		result.NextCursor = base64.RawURLEncoding.EncodeToString(encoded)
	}
	c.JSON(200, result)
}
func publicRole(role domain.Role) string {
	if role == domain.RoleAdmin {
		return "admin"
	}
	return "creator"
}
func accountRole(role string) (domain.Role, bool) {
	switch role {
	case "creator":
		return domain.RoleProducer, true
	case "admin":
		return domain.RoleAdmin, true
	default:
		return "", false
	}
}
func validDisplay(name string) bool {
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 32 {
		return false
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// AdminCreateRequest cannot choose account identity, organization or account state.
type AdminCreateRequest struct {
	LoginName       string `json:"login_name"`
	DisplayName     string `json:"display_name"`
	Role            string `json:"role" enums:"creator,admin"`
	InitialPassword string `json:"initial_password"`
}

// AdminMutationReceipt reports the actual committed revision without any secret.
type AdminMutationReceipt struct {
	ID                 uuid.UUID `json:"id"`
	Revision           int64     `json:"revision"`
	DisplayName        string    `json:"display_name,omitempty"`
	Role               string    `json:"role,omitempty" enums:"creator,admin"`
	Status             string    `json:"status,omitempty" enums:"active,disabled"`
	LoginUnlocked      bool      `json:"login_unlocked,omitempty"`
	MustChangePassword bool      `json:"must_change_password,omitempty"`
}

// Create creates a first-login-password-change account.
// @ID createAdminUser
// @Accept json
// @Produce json
// @Param body body AdminCreateRequest true "Account input"
// @Success 201 {object} AdminMutationReceipt
// @Failure 422 {object} httpapi.Problem
// @Router /api/admin/users [post]
func (h *AdminHandler) Create(c *gin.Context) {
	var body AdminCreateRequest
	if !httpapi.Decode(c, &body) {
		return
	}
	login, err := domain.NormalizeLogin(body.LoginName)
	role, ok := accountRole(body.Role)
	name := strings.TrimSpace(body.DisplayName)
	if err != nil || !ok || !validDisplay(name) {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	user, err := h.deps.Create.Execute(c.Request.Context(), Principal(c), application.CreateUserInput{LoginName: login, DisplayName: name, Role: role, InitialPassword: body.InitialPassword, RequestID: c.GetString("request_id")})
	if err != nil {
		adminProblem(c, err)
		return
	}
	c.JSON(201, AdminMutationReceipt{ID: user.ID, Revision: user.Revision, DisplayName: user.DisplayName, Role: publicRole(user.Role), Status: string(user.Status), MustChangePassword: user.MustChangePassword})
}

// AdminUpdateRequest carries an explicit optimistic revision and editable fields only.
type AdminUpdateRequest struct {
	ExpectedRevision int64  `json:"expected_revision"`
	DisplayName      string `json:"display_name"`
	Role             string `json:"role" enums:"creator,admin"`
}

// Update changes a display name or role through the audited command.
// @ID updateAdminUser
// @Accept json
// @Produce json
// @Param id path string true "User UUID"
// @Param body body AdminUpdateRequest true "Profile and expected revision"
// @Success 200 {object} AdminMutationReceipt
// @Failure 409 {object} httpapi.Problem
// @Router /api/admin/users/{id} [put]
func (h *AdminHandler) Update(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var body AdminUpdateRequest
	if !httpapi.Decode(c, &body) {
		return
	}
	role, ok := accountRole(body.Role)
	name := strings.TrimSpace(body.DisplayName)
	if !ok || !validDisplay(name) {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	if id == Principal(c).ID && role != domain.RoleAdmin {
		httpapi.WriteProblem(c, 409, "self_management_forbidden", nil)
		return
	}
	u, err := h.deps.Update.Execute(c.Request.Context(), Principal(c), application.UpdateUserInput{TargetID: id, ExpectedRevision: body.ExpectedRevision, DisplayName: &name, Role: &role, RequestID: c.GetString("request_id")})
	if err != nil {
		adminProblem(c, err)
		return
	}
	c.JSON(200, AdminMutationReceipt{ID: u.ID, Revision: u.Revision, DisplayName: u.DisplayName, Role: publicRole(u.Role)})
}
func adminProblem(c *gin.Context, err error) {
	status, code := 503, "dependency_unavailable"
	switch {
	case errors.Is(err, application.ErrForbidden):
		status, code = 403, "forbidden"
	case errors.Is(err, application.ErrLoginTaken):
		status, code = 422, "login_name_taken"
	case errors.Is(err, domain.ErrUserNotFound):
		status, code = 404, "user_not_found"
	case errors.Is(err, domain.ErrRevisionConflict):
		status, code = 409, "revision_conflict"
	case errors.Is(err, domain.ErrLastActiveAdmin):
		status, code = 409, "last_active_admin"
	case errors.Is(err, domain.ErrAccountDisabled), errors.Is(err, domain.ErrAccountActive), errors.Is(err, domain.ErrAccountUnchanged):
		status, code = 409, "account_state_conflict"
	case errors.Is(err, domain.ErrWeakPassword):
		status, code = 422, "weak_password"
	case errors.Is(err, domain.ErrPasswordReused):
		status, code = 422, "password_reused"
	case errors.Is(err, domain.ErrInvalidProfile), errors.Is(err, application.ErrInvalidCreateUser), errors.Is(err, application.ErrInvalidUpdateUser), errors.Is(err, application.ErrInvalidDisableUser), errors.Is(err, application.ErrInvalidEnableUser), errors.Is(err, application.ErrInvalidResetPassword), errors.Is(err, application.ErrInvalidUserList), errors.Is(err, application.ErrInvalidUnlockUser):
		status, code = 422, "invalid_request"
	}
	httpapi.WriteProblem(c, status, code, nil)
}
