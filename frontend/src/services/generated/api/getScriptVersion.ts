// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** Get Script Version GET /v1/script-versions/${param0} */
export async function getScriptVersion(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getScriptVersionParams,
  options?: RequestOptions
) {
  const { version_id: param0, ...queryParams } = params;
  return request<API.ScriptVersionResponse>(`/v1/script-versions/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}
