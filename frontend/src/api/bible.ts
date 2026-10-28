// @ts-ignore
/* eslint-disable */
import { request } from "@/lib/request";
import type { RequestOptions } from "@/lib/request";

/** 此处后端没有提供注释 GET /api/projects/${param0}/bible-voices */
export async function listBibleVoices(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listBibleVoicesParams,
  options?: RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.VoiceResponse>(`/api/projects/${param0}/bible-voices`, {
    method: "GET",
    params: {
      ...queryParams,
    },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/projects/${param0}/bible/${param1} */
export async function listBibleEntries(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listBibleEntriesParams,
  options?: RequestOptions,
) {
  const { pid: param0, kind: param1, ...queryParams } = params;
  return request<API.PageResponse>(`/api/projects/${param0}/bible/${param1}`, {
    method: "GET",
    params: {
      ...queryParams,
    },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/projects/${param0}/bible/${param1} */
export async function createBibleEntry(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.createBibleEntryParams,
  body: API.ContentRequest,
  options?: RequestOptions,
) {
  const { pid: param0, kind: param1, ...queryParams } = params;
  return request<API.Receipt>(`/api/projects/${param0}/bible/${param1}`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/projects/${param0}/bible/${param1}/${param2} */
export async function getBibleEntry(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getBibleEntryParams,
  options?: RequestOptions,
) {
  const { pid: param0, kind: param1, id: param2, ...queryParams } = params;
  return request<API.BibleDetail>(
    `/api/projects/${param0}/bible/${param1}/${param2}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 PUT /api/projects/${param0}/bible/${param1}/${param2} */
export async function updateBibleEntry(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.updateBibleEntryParams,
  body: API.ContentRequest,
  options?: RequestOptions,
) {
  const { pid: param0, kind: param1, id: param2, ...queryParams } = params;
  return request<API.Receipt>(
    `/api/projects/${param0}/bible/${param1}/${param2}`,
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

/** 此处后端没有提供注释 DELETE /api/projects/${param0}/bible/${param1}/${param2} */
export async function deleteBibleEntry(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.deleteBibleEntryParams,
  body: API.ControlRequest,
  options?: RequestOptions,
) {
  const { pid: param0, kind: param1, id: param2, ...queryParams } = params;
  return request<API.Receipt>(
    `/api/projects/${param0}/bible/${param1}/${param2}`,
    {
      method: "DELETE",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 POST /api/projects/${param0}/bible/${param1}/${param2}/adopt-result */
export async function adoptBibleResult(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.adoptBibleResultParams,
  body: API.ResultRequest,
  options?: RequestOptions,
) {
  const { pid: param0, kind: param1, id: param2, ...queryParams } = params;
  return request<API.Receipt>(
    `/api/projects/${param0}/bible/${param1}/${param2}/adopt-result`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 POST /api/projects/${param0}/bible/${param1}/${param2}/confirm */
export async function confirmBibleEntry(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.confirmBibleEntryParams,
  body: API.ControlRequest,
  options?: RequestOptions,
) {
  const { pid: param0, kind: param1, id: param2, ...queryParams } = params;
  return request<API.Receipt>(
    `/api/projects/${param0}/bible/${param1}/${param2}/confirm`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 POST /api/projects/${param0}/bible/${param1}/${param2}/looks */
export async function createBibleLook(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.createBibleLookParams,
  body: API.LookRequest,
  options?: RequestOptions,
) {
  const { pid: param0, kind: param1, id: param2, ...queryParams } = params;
  return request<API.Receipt>(
    `/api/projects/${param0}/bible/${param1}/${param2}/looks`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 PUT /api/projects/${param0}/bible/${param1}/${param2}/looks/${param3} */
export async function updateBibleLook(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.updateBibleLookParams,
  body: API.LookRequest,
  options?: RequestOptions,
) {
  const {
    pid: param0,
    kind: param1,
    id: param2,
    look: param3,
    ...queryParams
  } = params;
  return request<API.Receipt>(
    `/api/projects/${param0}/bible/${param1}/${param2}/looks/${param3}`,
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

/** 此处后端没有提供注释 DELETE /api/projects/${param0}/bible/${param1}/${param2}/looks/${param3} */
export async function deleteBibleLook(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.deleteBibleLookParams,
  body: API.ControlRequest,
  options?: RequestOptions,
) {
  const {
    pid: param0,
    kind: param1,
    id: param2,
    look: param3,
    ...queryParams
  } = params;
  return request<API.Receipt>(
    `/api/projects/${param0}/bible/${param1}/${param2}/looks/${param3}`,
    {
      method: "DELETE",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 POST /api/projects/${param0}/bible/${param1}/${param2}/looks/${param3}/default */
export async function setBibleDefaultLook(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.setBibleDefaultLookParams,
  body: API.ControlRequest,
  options?: RequestOptions,
) {
  const {
    pid: param0,
    kind: param1,
    id: param2,
    look: param3,
    ...queryParams
  } = params;
  return request<API.Receipt>(
    `/api/projects/${param0}/bible/${param1}/${param2}/looks/${param3}/default`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 PUT /api/projects/${param0}/bible/${param1}/${param2}/looks/${param3}/references */
export async function replaceBibleReferences(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.replaceBibleReferencesParams,
  body: API.ReferencesRequest,
  options?: RequestOptions,
) {
  const {
    pid: param0,
    kind: param1,
    id: param2,
    look: param3,
    ...queryParams
  } = params;
  return request<API.Receipt>(
    `/api/projects/${param0}/bible/${param1}/${param2}/looks/${param3}/references`,
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

/** 此处后端没有提供注释 POST /api/projects/${param0}/bible/${param1}/${param2}/merge */
export async function mergeBibleCharacter(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.mergeBibleCharacterParams,
  body: API.MergeRequest,
  options?: RequestOptions,
) {
  const { pid: param0, kind: param1, id: param2, ...queryParams } = params;
  return request<API.Receipt>(
    `/api/projects/${param0}/bible/${param1}/${param2}/merge`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 POST /api/projects/${param0}/bible/${param1}/${param2}/restore */
export async function restoreBibleEntry(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.restoreBibleEntryParams,
  body: API.ControlRequest,
  options?: RequestOptions,
) {
  const { pid: param0, kind: param1, id: param2, ...queryParams } = params;
  return request<API.Receipt>(
    `/api/projects/${param0}/bible/${param1}/${param2}/restore`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 POST /api/projects/${param0}/bible/${param1}/${param2}/split */
export async function splitBibleCharacter(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.splitBibleCharacterParams,
  body: API.SplitRequest,
  options?: RequestOptions,
) {
  const { pid: param0, kind: param1, id: param2, ...queryParams } = params;
  return request<API.Receipt>(
    `/api/projects/${param0}/bible/${param1}/${param2}/split`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 GET /api/projects/${param0}/bible/${param1}/${param2}/versions */
export async function listBibleVersions(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listBibleVersionsParams,
  options?: RequestOptions,
) {
  const { pid: param0, kind: param1, id: param2, ...queryParams } = params;
  return request<API.HistoryResponse>(
    `/api/projects/${param0}/bible/${param1}/${param2}/versions`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 GET /api/projects/${param0}/bible/${param1}/${param2}/versions/${param3} */
export async function getBibleVersion(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getBibleVersionParams,
  options?: RequestOptions,
) {
  const {
    pid: param0,
    kind: param1,
    id: param2,
    version: param3,
    ...queryParams
  } = params;
  return request<API.Version>(
    `/api/projects/${param0}/bible/${param1}/${param2}/versions/${param3}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 PUT /api/projects/${param0}/bible/${param1}/${param2}/voice */
export async function bindBibleVoice(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.bindBibleVoiceParams,
  body: API.VoiceRequest,
  options?: RequestOptions,
) {
  const { pid: param0, kind: param1, id: param2, ...queryParams } = params;
  return request<API.Receipt>(
    `/api/projects/${param0}/bible/${param1}/${param2}/voice`,
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

/** 此处后端没有提供注释 DELETE /api/projects/${param0}/bible/${param1}/${param2}/voice */
export async function unbindBibleVoice(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.unbindBibleVoiceParams,
  body: API.ControlRequest,
  options?: RequestOptions,
) {
  const { pid: param0, kind: param1, id: param2, ...queryParams } = params;
  return request<API.Receipt>(
    `/api/projects/${param0}/bible/${param1}/${param2}/voice`,
    {
      method: "DELETE",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 POST /api/projects/${param0}/bible/${param1}/adopt-result */
export async function createBibleEntryFromResult(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.createBibleEntryFromResultParams,
  body: API.ResultRequest,
  options?: RequestOptions,
) {
  const { pid: param0, kind: param1, ...queryParams } = params;
  return request<API.Receipt>(
    `/api/projects/${param0}/bible/${param1}/adopt-result`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}
