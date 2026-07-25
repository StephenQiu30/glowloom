// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** Create Source Revision POST /v1/episodes/${param0}/sources */
export async function createSourceRevision(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.createSourceRevisionParams,
  body: API.CreateSourceRevisionRequest,
  options?: RequestOptions
) {
  const { episode_id: param0, ...queryParams } = params;
  return request<API.SourceRevisionResponse>(`/v1/episodes/${param0}/sources`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}
