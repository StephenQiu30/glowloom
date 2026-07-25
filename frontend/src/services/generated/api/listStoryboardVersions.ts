// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** List Storyboard Versions GET /v1/episodes/${param0}/shot-spec-versions */
export async function listStoryboardVersions(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listStoryboardVersionsParams,
  options?: RequestOptions
) {
  const { episode_id: param0, ...queryParams } = params;
  return request<API.StoryboardVersionListResponse>(
    `/v1/episodes/${param0}/shot-spec-versions`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}
