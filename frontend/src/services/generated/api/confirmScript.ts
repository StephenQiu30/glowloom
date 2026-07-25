// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** Confirm Script POST /v1/script-versions/${param0}:confirm */
export async function confirmScript(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.confirmScriptParams,
  options?: RequestOptions
) {
  const { version_id: param0, ...queryParams } = params;
  return request<API.ScriptVersionResponse>(
    `/v1/script-versions/${param0}:confirm`,
    {
      method: "POST",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}
