// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** Get Source Revision GET /v1/source-revisions/${param0} */
export async function getSourceRevision(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getSourceRevisionParams,
  options?: RequestOptions
) {
  const { version_id: param0, ...queryParams } = params;
  return request<API.SourceRevisionResponse>(`/v1/source-revisions/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}
