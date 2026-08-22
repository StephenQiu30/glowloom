// @ts-ignore
/* eslint-disable */
import request, { type RequestOptions } from "@/lib/request";

/** 此处后端没有提供注释 POST /api/script-revisions/${param0}/approve */
export async function approveScriptAnalysis(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.approveScriptAnalysisParams,
  options?: RequestOptions
) {
  const { revision_id: param0, ...queryParams } = params;
  return request<API.AnalysisResponse>(
    `/api/script-revisions/${param0}/approve`,
    {
      method: "POST",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}
