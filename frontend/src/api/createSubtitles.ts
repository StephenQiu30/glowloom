// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** Create Subtitles POST /v1/episodes/${param0}/subtitle-versions */
export async function createSubtitles(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.createSubtitlesParams,
  options?: RequestOptions
) {
  const { episode_id: param0, ...queryParams } = params;
  return request<API.SubtitleVersionResponse>(
    `/v1/episodes/${param0}/subtitle-versions`,
    {
      method: "POST",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}
