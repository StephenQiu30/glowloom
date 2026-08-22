// @ts-ignore
/* eslint-disable */
import request, { type RequestOptions } from "@/lib/request";

/** 此处后端没有提供注释 POST /api/agent-runs/${param0}/cancel */
export async function cancelAgentRun(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.cancelAgentRunParams,
  options?: RequestOptions
) {
  const { agent_run_id: param0, ...queryParams } = params;
  return request<API.AgentRunResponse>(`/api/agent-runs/${param0}/cancel`, {
    method: "POST",
    params: { ...queryParams },
    ...(options || {}),
  });
}
