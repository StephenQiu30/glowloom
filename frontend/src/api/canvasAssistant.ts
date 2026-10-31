// @ts-ignore
/* eslint-disable */
import { request } from "@/lib/request";
import type { RequestOptions } from "@/lib/request";

/** 画布助手会话列表 GET /api/canvases/${param0}/assistant-sessions */
export async function listCanvasAssistantSessions(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listCanvasAssistantSessionsParams,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.AssistantSessions>(
    `/api/canvases/${param0}/assistant-sessions`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 新建画布助手会话 POST /api/canvases/${param0}/assistant-sessions */
export async function createCanvasAssistantSession(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.createCanvasAssistantSessionParams,
  body: API.AssistantCreate,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.AssistantSession>(
    `/api/canvases/${param0}/assistant-sessions`,
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

/** 读取画布助手会话 GET /api/canvases/${param0}/assistant-sessions/${param1} */
export async function readCanvasAssistantSession(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.readCanvasAssistantSessionParams,
  options?: RequestOptions,
) {
  const { id: param0, sid: param1, ...queryParams } = params;
  return request<API.AssistantSession>(
    `/api/canvases/${param0}/assistant-sessions/${param1}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 绑定画布助手下一轮输入 POST /api/canvases/${param0}/assistant-sessions/${param1}/turns */
export async function appendCanvasAssistantTurn(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.appendCanvasAssistantTurnParams,
  body: API.AssistantAppend,
  options?: RequestOptions,
) {
  const { id: param0, sid: param1, ...queryParams } = params;
  return request<API.AssistantSession>(
    `/api/canvases/${param0}/assistant-sessions/${param1}/turns`,
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
