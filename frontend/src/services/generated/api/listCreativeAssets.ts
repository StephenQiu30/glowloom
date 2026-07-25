// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** List Creative Assets GET /v1/episodes/${param0}/creative-assets */
export async function listCreativeAssets(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listCreativeAssetsParams,
  options?: RequestOptions
) {
  const { episode_id: param0, ...queryParams } = params;
  return request<API.CreativeAssetListResponse>(
    `/v1/episodes/${param0}/creative-assets`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    }
  );
}
