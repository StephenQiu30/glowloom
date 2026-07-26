// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** Save Subtitles PUT /v1/subtitle-versions/${param0} */
export async function saveSubtitles(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.saveSubtitlesParams,
  body: API.SaveSubtitleRequest,
  options?: RequestOptions
) {
  const { version_id: param0, ...queryParams } = params;
  return request<API.SubtitleVersionResponse>(
    `/v1/subtitle-versions/${param0}`,
    {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    }
  );
}
