// @ts-ignore
/* eslint-disable */
import { request } from "@/lib/request";
import type { RequestOptions } from "@/lib/request";

/** archive team POST /api/teams/${param0}/archive */
export async function archiveTeam(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.archiveTeamParams,
  body: API.TeamLifecycleRequest,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<any>(`/api/teams/${param0}/archive`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** restore team POST /api/teams/${param0}/restore */
export async function restoreTeam(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.restoreTeamParams,
  body: API.TeamLifecycleRequest,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<any>(`/api/teams/${param0}/restore`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** Update team invitation policy PATCH /api/teams/${param0}/settings */
export async function updateTeamSettings(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.updateTeamSettingsParams,
  body: API.TeamInvitationSettingsRequest,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<any>(`/api/teams/${param0}/settings`, {
    method: "PATCH",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}
