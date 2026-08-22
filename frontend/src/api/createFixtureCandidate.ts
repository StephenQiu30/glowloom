// @ts-ignore
/* eslint-disable */
import request, { type RequestOptions } from "@/lib/request";

/** 此处后端没有提供注释 POST /api/shots/${param0}/fixture-candidates */
export async function createFixtureCandidate(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.createFixtureCandidateParams,
  body: API.FixtureCandidateRequest,
  options?: RequestOptions
) {
  const { shot_id: param0, ...queryParams } = params;
  return request<API.CandidateResponse>(
    `/api/shots/${param0}/fixture-candidates`,
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
