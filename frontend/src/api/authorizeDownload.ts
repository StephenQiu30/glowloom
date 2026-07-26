// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** Authorize Download POST /v1/deliveries/${param0}/download-authorizations */
export async function authorizeDownload(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.authorizeDownloadParams,
  body: API.DownloadAuthorizationRequest,
  options?: RequestOptions
) {
  const { delivery_id: param0, ...queryParams } = params;
  return request<API.DownloadAuthorizationResponse>(
    `/v1/deliveries/${param0}/download-authorizations`,
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
