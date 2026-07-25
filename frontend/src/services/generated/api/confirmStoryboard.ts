// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** Confirm Storyboard POST /v1/shot-spec-versions/${param0}:confirm */
export async function confirmStoryboard(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.confirmStoryboardParams,
  options?: RequestOptions
) {
  const { version_id: param0, ...queryParams } = params;
  return request<API.StoryboardGenerationResponse>(
    `/v1/shot-spec-versions/${param0}:confirm`,
    {
      method: "POST",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}
