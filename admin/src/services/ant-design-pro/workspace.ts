// @ts-ignore
/* eslint-disable */
import { request } from "@umijs/max";

/** 创建 Workspace POST /api/workspaces */
export async function workspaceCreate(
  body: API.createWorkspaceRequest,
  options?: { [key: string]: any }
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
