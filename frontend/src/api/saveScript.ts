// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** Save Script PUT /v1/script-versions/${param0} */
export async function saveScript(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.saveScriptParams,
  body: API.SaveScriptRequest,
  options?: RequestOptions
) {
  const { version_id: param0, ...queryParams } = params;
  return request<API.ScriptVersionResponse>(`/v1/script-versions/${param0}`, {
    method: "PUT",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}
