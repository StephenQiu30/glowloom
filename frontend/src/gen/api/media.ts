// @ts-ignore
/* eslint-disable */
import { request } from "@/lib/request";

/** 项目可用媒体列表 GET /api/projects/${param0}/media */
export async function listMediaAssets(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listMediaAssetsParams,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalMediaApplicationAssetPage>(
    `/api/projects/${param0}/media`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    },
  );
}

/** 获取媒体预览 GET /api/projects/${param0}/media/${param1}/preview */
export async function getMediaPreview(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getMediaPreviewParams,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, asset_id: param1, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalMediaApplicationPreview>(
    `/api/projects/${param0}/media/${param1}/preview`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}
