// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** Generate Storyboard POST /v1/episodes/${param0}/storyboard-generations */
export async function generateStoryboard(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.generateStoryboardParams,
  options?: RequestOptions
) {
  const { episode_id: param0, ...queryParams } = params;
  return request<any>(`/v1/episodes/${param0}/storyboard-generations`, {
    method: "POST",
    params: { ...queryParams },
    ...(options || {}),
  });
}
