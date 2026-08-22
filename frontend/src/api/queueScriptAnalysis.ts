// @ts-ignore
/* eslint-disable */
import request, { type RequestOptions } from "@/lib/request";

/** 此处后端没有提供注释 POST /api/script-revisions/${param0}/analyze */
export async function queueScriptAnalysis(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.queueScriptAnalysisParams,
  options?: RequestOptions
) {
  const { revision_id: param0, ...queryParams } = params;
  return request<any>(`/api/script-revisions/${param0}/analyze`, {
    method: "POST",
    params: { ...queryParams },
    ...(options || {}),
  });
}
