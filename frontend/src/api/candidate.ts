// @ts-ignore
/* eslint-disable */
import request, { type RequestOptions } from "@/lib/request";

/** 选择候选 POST /api/candidates/${param0}/selections */
export async function candidateSelect(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.candidateSelectParams,
  body: API.selectCandidateRequest,
  options?: RequestOptions
) {
  const { candidateID: param0, ...queryParams } = params;
  return request<API.SelectionEnvelope>(
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

/** 创建候选 POST /api/shots/${param0}/fixture-candidates */
export async function candidateCreateFixture(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.candidateCreateFixtureParams,
  body: API.createFixtureCandidateRequest,
  options?: RequestOptions
) {
  const { shotID: param0, ...queryParams } = params;
  return request<API.CandidateEnvelope>(
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
