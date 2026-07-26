// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** Render Episode POST /v1/episodes/${param0}/renders */
export async function renderEpisode(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.renderEpisodeParams,
  options?: RequestOptions
) {
  const { episode_id: param0, ...queryParams } = params;
  return request<any>(`/v1/episodes/${param0}/renders`, {
    method: "POST",
    params: { ...queryParams },
    ...(options || {}),
  });
}
