// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** Get Episode GET /v1/episodes/${param0} */
export async function getEpisode(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getEpisodeParams,
  options?: RequestOptions
) {
  const { episode_id: param0, ...queryParams } = params;
  return request<API.EpisodeResponse>(`/v1/episodes/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}
