// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** Get Current Script GET /v1/episodes/${param0}/script */
export async function getCurrentScript(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getCurrentScriptParams,
  options?: RequestOptions
) {
  const { episode_id: param0, ...queryParams } = params;
  return request<API.ScriptVersionResponse>(`/v1/episodes/${param0}/script`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}
