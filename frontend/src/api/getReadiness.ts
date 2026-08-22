// @ts-ignore
/* eslint-disable */
import request, { type RequestOptions } from "@/lib/request";

/** 此处后端没有提供注释 GET /readyz */
export async function getReadiness(options?: RequestOptions) {
  return request<API.ReadinessResponse>("/readyz", {
    method: "GET",
    ...(options || {}),
  });
}
