// @ts-ignore
/* eslint-disable */
import request, { type RequestOptions } from "@/lib/request";

/** 此处后端没有提供注释 GET /api/script-revisions/${param0}/analysis-draft */
export async function getAnalysisDraft(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getAnalysisDraftParams,
  options?: RequestOptions
) {
  const { revision_id: param0, ...queryParams } = params;
  return request<API.AnalysisResponse>(
    `/api/script-revisions/${param0}/analysis-draft`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}
