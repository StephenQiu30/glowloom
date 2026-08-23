// @ts-ignore
/* eslint-disable */
import { request } from "@umijs/max";

/** 查询 Workspace 访问审计 GET /api/admin/audit-events */
export async function adminListAccessAudit(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.adminListAccessAuditParams,
  options?: { [key: string]: any }
) {
  return request<API.AccessAuditPageEnvelope>("/api/admin/audit-events", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** 查询 Workspace 用户 GET /api/admin/members */
export async function adminListMembers(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.adminListMembersParams,
  options?: { [key: string]: any }
) {
  return request<API.WorkspaceMemberPageEnvelope>("/api/admin/members", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** 修改 Workspace 用户 PATCH /api/admin/members/${param0} */
export async function adminUpdateMember(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.adminUpdateMemberParams,
  body: API.updateMemberRequest,
  options?: { [key: string]: any }
) {
  const { membership_id: param0, ...queryParams } = params;
  return request<API.WorkspaceMemberEnvelope>(`/api/admin/members/${param0}`, {
    method: "PATCH",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}
