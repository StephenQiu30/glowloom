// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** Generate Media POST /v1/episodes/${param0}/media-generations */
export async function generateMedia(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.generateMediaParams,
  body: API.GenerateMediaRequest,
  options?: RequestOptions
) {
  const { episode_id: param0, ...queryParams } = params;
  return request<any>(`/v1/episodes/${param0}/media-generations`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}
