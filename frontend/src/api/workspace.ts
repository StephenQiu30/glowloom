// @ts-ignore
/* eslint-disable */
import request, { type RequestOptions } from "@/lib/request";

/** 创建 Workspace POST /api/workspaces */
export async function workspaceCreate(
  body: API.createWorkspaceRequest,
  options?: RequestOptions
) {
  return request<API.WorkspaceEnvelope>("/api/workspaces", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}
