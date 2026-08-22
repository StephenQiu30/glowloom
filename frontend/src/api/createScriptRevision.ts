// @ts-ignore
/* eslint-disable */
import request, { type RequestOptions } from "@/lib/request";

/** 此处后端没有提供注释 POST /api/projects/${param0}/script-revisions */
export async function createScriptRevision(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.createScriptRevisionParams,
  body: API.CreateScriptRevisionRequest,
  options?: RequestOptions
) {
  const { project_id: param0, ...queryParams } = params;
  return request<API.ScriptRevisionResponse>(
    `/api/projects/${param0}/script-revisions`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    }
  );
}
