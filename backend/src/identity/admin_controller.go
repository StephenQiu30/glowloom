package identity

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/stephenqiu30/lanverse/backend/src/platform/httpapi"
)

type IdentityAdminController struct {
	service *IdentityService
}

func NewIdentityAdminController(service *IdentityService) *IdentityAdminController {
	return &IdentityAdminController{service: service}
}

func (h *IdentityAdminController) Mount(router chi.Router) {
	router.Route("/api/admin", func(router chi.Router) {
		router.Use(RequireAdmin)
		router.Get("/members", h.listMembers)
		router.Get("/audit-events", h.listAccessAudit)
		router.Patch("/members/{membership_id}", h.updateMember)
	})
}

// listAccessAudit 返回当前 Workspace 的访问审计，仅 Admin 可访问。
// @Summary 查询 Workspace 访问审计
// @Tags admin
// @ID admin_list_access_audit
// @Produce json
// @Param search query string false "跨主体、对象、动作、理由和 Request ID 搜索"
// @Param actor query string false "按主体名称、邮箱或 ID 搜索"
// @Param object query string false "按对象类型、名称、邮箱或 ID 搜索"
// @Param action query string false "按动作精确筛选"
// @Param result query string false "按结果筛选" Enums(succeeded,denied,failed)
// @Param occurred_from query string false "起始时间（RFC3339，含）"
// @Param occurred_to query string false "结束时间（RFC3339，含）"
// @Param page query int false "页码"
// @Param page_size query int false "每页数量"
// @Security BearerAccessToken
// @Success 200 {object} AccessAuditPageEnvelope
// @Failure 403 {object} httpapi.ErrorEnvelope
// @Failure 422 {object} httpapi.ErrorEnvelope
// @Router /api/admin/audit-events [get]
func (h *IdentityAdminController) listAccessAudit(w http.ResponseWriter, r *http.Request) {
	principal, ok := currentPrincipal(w, r)
	if !ok {
		return
	}
	page, ok := queryInt(w, r, "page", 1)
	if !ok {
		return
	}
	pageSize, ok := queryInt(w, r, "page_size", 20)
	if !ok {
		return
	}
	occurredFrom, ok := queryTime(w, r, "occurred_from")
	if !ok {
		return
	}
	occurredTo, ok := queryTime(w, r, "occurred_to")
	if !ok {
		return
	}
	result, err := h.service.ListAccessAudit(r.Context(), principal, AccessAuditQuery{
		Search: r.URL.Query().Get("search"), Actor: r.URL.Query().Get("actor"), Object: r.URL.Query().Get("object"),
		Action: r.URL.Query().Get("action"), Result: AccessAuditResult(strings.TrimSpace(r.URL.Query().Get("result"))),
		OccurredFrom: occurredFrom, OccurredTo: occurredTo, Page: page, PageSize: pageSize,
	})
	if err != nil {
		httpapi.WriteError(w, r, err)
		return
	}
	httpapi.WriteData(w, httpapi.StatusOK, result)
}

type updateMemberRequest struct {
	Role   *string `json:"role"`
	Status *string `json:"status"`
	Reason string  `json:"reason"`
}

// listMembers 返回当前 Workspace 的成员列表，仅 Admin 可访问。
// @Summary 查询 Workspace 用户
// @Tags admin
// @ID admin_list_members
// @Produce json
// @Param search query string false "按邮箱或显示名搜索"
// @Param page query int false "页码"
// @Param page_size query int false "每页数量"
// @Security BearerAccessToken
// @Success 200 {object} WorkspaceMemberPageEnvelope
// @Failure 403 {object} httpapi.ErrorEnvelope
// @Router /api/admin/members [get]
func (h *IdentityAdminController) listMembers(w http.ResponseWriter, r *http.Request) {
	principal, ok := currentPrincipal(w, r)
	if !ok {
		return
	}
	page, ok := queryInt(w, r, "page", 1)
	if !ok {
		return
	}
	pageSize, ok := queryInt(w, r, "page_size", 20)
	if !ok {
		return
	}
	result, err := h.service.ListWorkspaceMembers(r.Context(), principal, WorkspaceMemberQuery{Search: r.URL.Query().Get("search"), Page: page, PageSize: pageSize})
	if err != nil {
		httpapi.WriteError(w, r, err)
		return
	}
	httpapi.WriteData(w, httpapi.StatusOK, result)
}

// updateMember 修改成员角色或成员状态，仅 Admin 可访问。
// @Summary 修改 Workspace 用户
// @Tags admin
// @ID admin_update_member
// @Accept json
// @Produce json
// @Param membership_id path string true "Membership UUID"
// @Param request body updateMemberRequest true "角色或状态"
// @Security BearerAccessToken
// @Success 200 {object} WorkspaceMemberEnvelope
// @Failure 403 {object} httpapi.ErrorEnvelope
// @Failure 404 {object} httpapi.ErrorEnvelope
// @Failure 409 {object} httpapi.ErrorEnvelope
// @Failure 422 {object} httpapi.ErrorEnvelope
// @Router /api/admin/members/{membership_id} [patch]
func (h *IdentityAdminController) updateMember(w http.ResponseWriter, r *http.Request) {
	principal, ok := currentPrincipal(w, r)
	if !ok {
		return
	}
	membershipID, err := parseMembershipID(chi.URLParam(r, "membership_id"))
	if err != nil {
		httpapi.WriteError(w, r, err)
		return
	}
	var body updateMemberRequest
	if !httpapi.DecodeJSON(w, r, &body, 16<<10) {
		return
	}
	input, err := parseMemberUpdate(body)
	if err != nil {
		httpapi.WriteError(w, r, err)
		return
	}
	input.RequestID = httpapi.RequestID(r)
	result, err := h.service.UpdateWorkspaceMember(r.Context(), principal, membershipID, input)
	if err != nil {
		httpapi.WriteError(w, r, err)
		return
	}
	httpapi.WriteData(w, httpapi.StatusOK, result)
}

func currentPrincipal(w http.ResponseWriter, r *http.Request) (Principal, bool) {
	principal, ok := PrincipalFromContext(r.Context())
	if !ok {
		httpapi.WriteError(w, r, httpapi.NewError(httpapi.StatusUnauthorized, httpapi.CodeUnauthorized, "登录会话缺失", "重新登录后重试"))
		return Principal{}, false
	}
	return principal, true
}

func queryInt(w http.ResponseWriter, r *http.Request, key string, fallback int) (int, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return fallback, true
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		httpapi.WriteError(w, r, httpapi.Validation(key+" 必须是整数", "修改查询参数后重试"))
		return 0, false
	}
	return value, true
}

func queryTime(w http.ResponseWriter, r *http.Request, key string) (*time.Time, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return nil, true
	}
	value, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		httpapi.WriteError(w, r, httpapi.Validation(key+" 必须是 RFC3339 时间", "修改查询参数后重试"))
		return nil, false
	}
	value = value.UTC()
	return &value, true
}

func parseMembershipID(raw string) (uuid.UUID, error) {
	membershipID, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil || membershipID == uuid.Nil {
		return uuid.Nil, httpapi.NewError(httpapi.StatusUnprocessableEntity, httpapi.CodeInvalidID, "Membership ID 无效", "提供有效 Membership ID 后重试")
	}
	return membershipID, nil
}

func parseMemberUpdate(body updateMemberRequest) (WorkspaceMemberUpdate, error) {
	input := WorkspaceMemberUpdate{Reason: strings.TrimSpace(body.Reason)}
	if body.Role != nil {
		role := RoleCode(strings.TrimSpace(*body.Role))
		input.Role = &role
	}
	if body.Status != nil {
		status := MembershipStatus(strings.TrimSpace(*body.Status))
		input.Status = &status
	}
	if input.Role == nil && input.Status == nil {
		return WorkspaceMemberUpdate{}, httpapi.Validation("至少提供角色或成员状态", "修改 role 或 status 后重试")
	}
	return input, nil
}
