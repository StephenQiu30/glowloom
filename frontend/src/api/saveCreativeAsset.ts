// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** Save Creative Asset PUT /v1/creative-asset-versions/${param0} */
export async function saveCreativeAsset(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.saveCreativeAssetParams,
  body: API.SaveCreativeAssetRequest,
  options?: RequestOptions
) {
  const { version_id: param0, ...queryParams } = params;
  return request<API.CreativeAssetVersionResponse>(
    `/v1/creative-asset-versions/${param0}`,
    {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    }
  );
}
