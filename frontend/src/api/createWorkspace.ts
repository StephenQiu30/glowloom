// @ts-ignore
/* eslint-disable */
import request, { type RequestOptions } from "@/lib/request";

/** 此处后端没有提供注释 POST /api/workspaces */
export async function createWorkspace(
  body: API.CreateWorkspaceRequest,
  options?: RequestOptions
) {
  return request<API.WorkspaceResponse>("/api/workspaces", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}
