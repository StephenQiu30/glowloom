// @ts-ignore
/* eslint-disable */
import { request } from "@/lib/request";
import type { RequestOptions } from "@/lib/request";

/** 项目默认模型 GET /api/projects/${param0}/model-defaults */
export async function getProjectModelDefaults(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getProjectModelDefaultsParams,
  options?: RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.ModelDefaults>(`/api/projects/${param0}/model-defaults`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 保存项目默认模型 PATCH /api/projects/${param0}/model-defaults */
export async function saveProjectModelDefaults(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.saveProjectModelDefaultsParams,
  body: API.ModelDefaultsRequest,
  options?: RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.ModelDefaults>(`/api/projects/${param0}/model-defaults`, {
    method: "PATCH",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 工作区提示词偏好 GET /api/settings/prompt-preferences */
export async function listPromptPreferences(options?: RequestOptions) {
  return request<API.PreferencesResponse>("/api/settings/prompt-preferences", {
    method: "GET",
    ...(options || {}),
  });
}

/** 保存或恢复工作区提示词偏好 PUT /api/settings/prompt-preferences/${param0} */
export async function savePromptPreference(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.savePromptPreferenceParams,
  body: API.SaveRequest,
  options?: RequestOptions,
) {
  const { operation: param0, ...queryParams } = params;
  return request<API.Customization>(
    `/api/settings/prompt-preferences/${param0}`,
    {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}
