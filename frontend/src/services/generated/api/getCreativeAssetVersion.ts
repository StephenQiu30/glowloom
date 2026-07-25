// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** Get Creative Asset Version GET /v1/creative-asset-versions/${param0} */
export async function getCreativeAssetVersion(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getCreativeAssetVersionParams,
  options?: RequestOptions
) {
  const { version_id: param0, ...queryParams } = params;
  return request<API.CreativeAssetVersionResponse>(
    `/v1/creative-asset-versions/${param0}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}
