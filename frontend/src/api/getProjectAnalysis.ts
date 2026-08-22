// @ts-ignore
/* eslint-disable */
import request, { type RequestOptions } from "@/lib/request";

/** 此处后端没有提供注释 GET /api/projects/${param0}/analysis */
export async function getProjectAnalysis(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getProjectAnalysisParams,
  options?: RequestOptions
) {
  const { project_id: param0, ...queryParams } = params;
  return request<API.AnalysisResponse>(`/api/projects/${param0}/analysis`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}
