// @ts-ignore
/* eslint-disable */
import { request } from "@/lib/request";
import type { RequestOptions } from "@/lib/request";

/** 当前项目我的 Agent 会话 GET /api/projects/${param0}/agent-sessions */
export async function listAgentSessions(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listAgentSessionsParams,
  options?: RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.Session[]>(`/api/projects/${param0}/agent-sessions`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 新建项目 Agent 私有会话 POST /api/projects/${param0}/agent-sessions */
export async function createAgentSession(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.createAgentSessionParams,
  body: API.CreateRequest,
  options?: RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.Session>(`/api/projects/${param0}/agent-sessions`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 读取 Agent 会话与阶段回执 GET /api/projects/${param0}/agent-sessions/${param1} */
export async function getAgentSession(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getAgentSessionParams,
  options?: RequestOptions,
) {
  const { pid: param0, sid: param1, ...queryParams } = params;
  return request<API.Session>(
    `/api/projects/${param0}/agent-sessions/${param1}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 恢复 Agent 轮次并准备下一阶段报价 POST /api/projects/${param0}/agent-sessions/${param1}/advance */
export async function advanceAgentSession(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.advanceAgentSessionParams,
  options?: RequestOptions,
) {
  const { pid: param0, sid: param1, ...queryParams } = params;
  return request<API.Session>(
    `/api/projects/${param0}/agent-sessions/${param1}/advance`,
    {
      method: "POST",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 确认 Agent 当前阶段积分 POST /api/projects/${param0}/agent-sessions/${param1}/confirm */
export async function confirmAgentStage(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.confirmAgentStageParams,
  body: API.ConfirmInput,
  options?: RequestOptions,
) {
  const { pid: param0, sid: param1, ...queryParams } = params;
  return request<API.Session>(
    `/api/projects/${param0}/agent-sessions/${param1}/confirm`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 停止 Agent 后续阶段 POST /api/projects/${param0}/agent-sessions/${param1}/stop */
export async function stopAgentSession(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.stopAgentSessionParams,
  body: API.StopRequest,
  options?: RequestOptions,
) {
  const { pid: param0, sid: param1, ...queryParams } = params;
  return request<API.Session>(
    `/api/projects/${param0}/agent-sessions/${param1}/stop`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 开始一轮 Agent 图片创作 POST /api/projects/${param0}/agent-sessions/${param1}/turns */
export async function startAgentTurn(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.startAgentTurnParams,
  body: API.TurnInput,
  options?: RequestOptions,
) {
  const { pid: param0, sid: param1, ...queryParams } = params;
  return request<API.Session>(
    `/api/projects/${param0}/agent-sessions/${param1}/turns`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}
