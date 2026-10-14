// @ts-ignore
/* eslint-disable */
import { request } from "@/lib/request";
import type { RequestOptions } from "@/lib/request";

/** 查找项目中自己的草稿 GET /api/projects/${param0}/prompt-drafts */
export async function listPromptDrafts(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listPromptDraftsParams,
  options?: RequestOptions
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.DraftPage>(`/api/projects/${param0}/prompt-drafts`, {
    method: "GET",
    params: {
      ...queryParams,
    },
    ...(options || {}),
  });
}

/** 创建完整提示词草稿 POST /api/projects/${param0}/prompt-drafts */
export async function createPromptDraft(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.createPromptDraftParams,
  body: API.DraftRequest,
  options?: RequestOptions
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.DraftSnapshot>(`/api/projects/${param0}/prompt-drafts`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 读取完整提示词草稿 GET /api/projects/${param0}/prompt-drafts/${param1} */
export async function readPromptDraft(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.readPromptDraftParams,
  options?: RequestOptions
) {
  const { pid: param0, id: param1, ...queryParams } = params;
  return request<API.DraftSnapshot>(
    `/api/projects/${param0}/prompt-drafts/${param1}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** 条件保存完整提示词草稿 PUT /api/projects/${param0}/prompt-drafts/${param1} */
export async function savePromptDraft(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.savePromptDraftParams,
  body: API.DraftRequest,
  options?: RequestOptions
) {
  const { pid: param0, id: param1, ...queryParams } = params;
  return request<API.DraftSnapshot>(
    `/api/projects/${param0}/prompt-drafts/${param1}`,
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

/** 知晓接受结果或放弃未确认报价 POST /api/projects/${param0}/prompt-submissions/${param1}/resolve */
export async function resolvePromptSubmission(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.resolvePromptSubmissionParams,
  body: API.ResolveSubmissionRequest,
  options?: RequestOptions
) {
  const { pid: param0, id: param1, ...queryParams } = params;
  return request<API.SubmissionCurrent>(
    `/api/projects/${param0}/prompt-submissions/${param1}/resolve`,
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

/** 查找当前未决编辑器提交 GET /api/projects/${param0}/prompt-submissions/current */
export async function currentPromptSubmission(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.currentPromptSubmissionParams,
  options?: RequestOptions
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.SubmissionCurrent>(
    `/api/projects/${param0}/prompt-submissions/current`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    }
  );
}
