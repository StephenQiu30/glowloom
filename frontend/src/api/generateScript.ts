// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** Generate Script POST /v1/episodes/${param0}/script-generations */
export async function generateScript(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.generateScriptParams,
  options?: RequestOptions
) {
  const { episode_id: param0, ...queryParams } = params;
  return request<any>(`/v1/episodes/${param0}/script-generations`, {
    method: "POST",
    params: { ...queryParams },
    ...(options || {}),
  });
}
