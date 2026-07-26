// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** Confirm Subtitles POST /v1/subtitle-versions/${param0}:confirm */
export async function confirmSubtitles(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.confirmSubtitlesParams,
  options?: RequestOptions
) {
  const { version_id: param0, ...queryParams } = params;
  return request<API.SubtitleVersionResponse>(
    `/v1/subtitle-versions/${param0}:confirm`,
    {
      method: "POST",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}
