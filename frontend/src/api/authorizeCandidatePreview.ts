// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** Authorize Candidate Preview POST /v1/media-versions/${param0}/preview-authorizations */
export async function authorizeCandidatePreview(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.authorizeCandidatePreviewParams,
  body: API.PreviewAuthorizationRequest,
  options?: RequestOptions
) {
  const { media_version_id: param0, ...queryParams } = params;
  return request<API.PreviewAuthorizationResponse>(
    `/v1/media-versions/${param0}/preview-authorizations`,
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
