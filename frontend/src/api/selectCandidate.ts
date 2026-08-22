// @ts-ignore
/* eslint-disable */
import request, { type RequestOptions } from "@/lib/request";

/** 此处后端没有提供注释 POST /api/candidates/${param0}/selections */
export async function selectCandidate(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.selectCandidateParams,
  body: API.SelectionRequest,
  options?: RequestOptions
) {
  const { candidate_id: param0, ...queryParams } = params;
  return request<API.SelectionResponse>(
    `/api/candidates/${param0}/selections`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    }
  );
}
