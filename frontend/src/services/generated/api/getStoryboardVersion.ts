// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** Get Storyboard Version GET /v1/shot-spec-versions/${param0} */
export async function getStoryboardVersion(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getStoryboardVersionParams,
  options?: RequestOptions
) {
  const { version_id: param0, ...queryParams } = params;
  return request<API.StoryboardVersionResponse>(
    `/v1/shot-spec-versions/${param0}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}
