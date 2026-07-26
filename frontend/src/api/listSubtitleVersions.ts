// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** List Subtitle Versions GET /v1/episodes/${param0}/subtitle-versions */
export async function listSubtitleVersions(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listSubtitleVersionsParams,
  options?: RequestOptions
) {
  const { episode_id: param0, ...queryParams } = params;
  return request<API.SubtitleVersionListResponse>(
    `/v1/episodes/${param0}/subtitle-versions`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}
