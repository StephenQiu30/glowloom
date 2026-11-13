// @ts-ignore
/* eslint-disable */
import { request } from "@/lib/request";
import type { RequestOptions } from "@/lib/request";

/** 此处后端没有提供注释 GET /api/analytics/business */
export async function getBusinessAnalyticsCapability(options?: RequestOptions) {
  return request<API.Business>("/api/analytics/business", {
    method: "GET",
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/analytics/creation */
export async function getCreationAnalytics(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getCreationAnalyticsParams,
  options?: RequestOptions,
) {
  return request<API.Creation>("/api/analytics/creation", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}
