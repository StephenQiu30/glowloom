// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** List Projects GET /v1/projects */
export async function listProjects(options?: RequestOptions) {
  return request<API.ProjectListResponse>("/v1/projects", {
    method: "GET",
    ...(options || {}),
  });
}
