// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** Get Subtitle Version GET /v1/subtitle-versions/${param0} */
export async function getSubtitleVersion(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getSubtitleVersionParams,
  options?: RequestOptions
) {
  const { version_id: param0, ...queryParams } = params;
  return request<API.SubtitleVersionResponse>(
    `/v1/subtitle-versions/${param0}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}
