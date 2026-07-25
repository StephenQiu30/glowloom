// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** Get Storyboard GET /v1/episodes/${param0}/storyboard */
export async function getStoryboard(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getStoryboardParams,
  options?: RequestOptions
) {
  const { episode_id: param0, ...queryParams } = params;
  return request<API.StoryboardVersionResponse>(
    `/v1/episodes/${param0}/storyboard`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}
