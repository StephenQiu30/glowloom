// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** Derive Script Draft POST /v1/script-versions/${param0}/drafts */
export async function deriveScriptDraft(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.deriveScriptDraftParams,
  options?: RequestOptions
) {
  const { version_id: param0, ...queryParams } = params;
  return request<API.ScriptVersionResponse>(
    `/v1/script-versions/${param0}/drafts`,
    {
      method: "POST",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}
