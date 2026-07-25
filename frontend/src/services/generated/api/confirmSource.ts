// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** Confirm Source POST /v1/source-revisions/${param0}:confirm */
export async function confirmSource(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.confirmSourceParams,
  options?: RequestOptions
) {
  const { version_id: param0, ...queryParams } = params;
  return request<API.SourceRevisionResponse>(
    `/v1/source-revisions/${param0}:confirm`,
    {
      method: "POST",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}
