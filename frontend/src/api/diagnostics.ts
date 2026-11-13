// @ts-ignore
/* eslint-disable */
import { request } from "@/lib/request";
import type { RequestOptions } from "@/lib/request";

/** 此处后端没有提供注释 GET /api/platform/diagnostics/logs */
export async function listDiagnosticLogs(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listDiagnosticLogsParams,
  options?: RequestOptions,
) {
  return request<API.ObservabilityPage>("/api/platform/diagnostics/logs", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/platform/diagnostics/logs/${param0} */
export async function getDiagnosticLog(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getDiagnosticLogParams,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.ObservabilityDetail>(
    `/api/platform/diagnostics/logs/${param0}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 GET /api/platform/diagnostics/overview */
export async function getDiagnosticsOverview(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getDiagnosticsOverviewParams,
  options?: RequestOptions,
) {
  return request<API.Overview>("/api/platform/diagnostics/overview", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}
