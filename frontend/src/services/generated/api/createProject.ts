// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** Create Project POST /v1/projects */
export async function createProject(
  body: API.CreateProjectRequest,
  options?: RequestOptions
) {
  return request<API.ProjectDetailResponse>("/v1/projects", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}
