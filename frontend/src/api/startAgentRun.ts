// @ts-ignore
/* eslint-disable */
import request, { type RequestOptions } from "@/lib/request";

/** 此处后端没有提供注释 POST /api/agent-runs */
export async function startAgentRun(
  body: API.StartAgentRunRequest,
  options?: RequestOptions
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
