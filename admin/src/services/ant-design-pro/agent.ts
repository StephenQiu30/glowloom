// @ts-ignore
/* eslint-disable */
import { request } from "@umijs/max";

/** 创建 AgentRun POST /api/agent-runs */
export async function agentRunStart(
  body: API.startRequest,
  options?: { [key: string]: any }
) {
  return request<any>("/api/agent-runs", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 查询 AgentRun GET /api/agent-runs/${param0} */
export async function agentRunGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.agentRunGetParams,
  options?: { [key: string]: any }
) {
  const { agentRunID: param0, ...queryParams } = params;
  return request<Record<string, any>>(`/api/agent-runs/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 取消 AgentRun POST /api/agent-runs/${param0}/cancel */
export async function agentRunCancel(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.agentRunCancelParams,
  options?: { [key: string]: any }
) {
  const { agentRunID: param0, ...queryParams } = params;
  return request<Record<string, any>>(`/api/agent-runs/${param0}/cancel`, {
    method: "POST",
    params: { ...queryParams },
    ...(options || {}),
  });
}
