// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** List Script Versions GET /v1/episodes/${param0}/script-versions */
export async function listScriptVersions(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listScriptVersionsParams,
  options?: RequestOptions
) {
  const { episode_id: param0, ...queryParams } = params;
  return request<API.ScriptVersionListResponse>(
    `/v1/episodes/${param0}/script-versions`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}
