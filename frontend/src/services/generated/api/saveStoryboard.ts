// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** Save Storyboard PUT /v1/shot-spec-versions/${param0} */
export async function saveStoryboard(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.saveStoryboardParams,
  body: API.SaveStoryboardRequest,
  options?: RequestOptions
) {
  const { version_id: param0, ...queryParams } = params;
  return request<API.StoryboardVersionResponse>(
    `/v1/shot-spec-versions/${param0}`,
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
