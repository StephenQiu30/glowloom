// @ts-ignore
/* eslint-disable */
import request, { type RequestOptions } from "@/lib/request";

/** 此处后端没有提供注释 GET /api/agent-runs/${param0} */
export async function getAgentRun(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getAgentRunParams,
  options?: RequestOptions
) {
  const { agent_run_id: param0, ...queryParams } = params;
  return request<API.AgentRunResponse>(`/api/agent-runs/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}
