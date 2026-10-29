// @ts-ignore
/* eslint-disable */
import { request } from "@/lib/request";
import type { RequestOptions } from "@/lib/request";

/** 系统音色 GET /api/projects/${param0}/system-voices */
export async function listSystemVoices(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listSystemVoicesParams,
  options?: RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.SystemVoice[]>(`/api/projects/${param0}/system-voices`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 收藏系统音色 PUT /api/projects/${param0}/system-voices/${param1}/favorite */
export async function favoriteSystemVoice(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.favoriteSystemVoiceParams,
  body: API.VoiceFavoriteRequest,
  options?: RequestOptions,
) {
  const { pid: param0, speaker: param1, ...queryParams } = params;
  return request<API.VoiceFavoriteRequest>(
    `/api/projects/${param0}/system-voices/${param1}/favorite`,
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

/** 我的参考音色 GET /api/projects/${param0}/voice-library */
export async function listReferenceVoices(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listReferenceVoicesParams,
  options?: RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.VoicePage>(`/api/projects/${param0}/voice-library`, {
    method: "GET",
    params: {
      ...queryParams,
    },
    ...(options || {}),
  });
}

/** 保存参考音色 POST /api/projects/${param0}/voice-library */
export async function createReferenceVoice(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.createReferenceVoiceParams,
  body: API.VoiceCreate,
  options?: RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.VoiceProfile>(`/api/projects/${param0}/voice-library`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 更新参考音色 PATCH /api/projects/${param0}/voice-library/${param1} */
export async function updateReferenceVoice(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.updateReferenceVoiceParams,
  body: API.VoiceUpdate,
  options?: RequestOptions,
) {
  const { pid: param0, vid: param1, ...queryParams } = params;
  return request<API.VoiceProfile>(
    `/api/projects/${param0}/voice-library/${param1}`,
    {
      method: "PATCH",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}
