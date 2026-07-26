// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** List Candidates GET /v1/episodes/${param0}/candidates */
export async function listCandidates(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listCandidatesParams,
  options?: RequestOptions
) {
  const { episode_id: param0, ...queryParams } = params;
  return request<API.CandidateListResponse>(
    `/v1/episodes/${param0}/candidates`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    }
  );
}
