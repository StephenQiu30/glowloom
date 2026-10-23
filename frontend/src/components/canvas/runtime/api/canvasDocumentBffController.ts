/* eslint-disable */
import { request } from "@/components/canvas/runtime/lib/request";
import type { AxiosRequestConfig } from "axios";

/** 此处后端没有提供注释 POST /canvas/add */
export async function addCanvas(
  body: CanvasContract.CanvasAddRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseCanvasCommandResultVO>(
    "/canvas/add",
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      data: body,
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 GET /canvas/command/get */
export async function getCanvasCommand(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CanvasContract.getCanvasCommandParams,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseCanvasCommandStatusVO>(
    "/canvas/command/get",
    {
      method: "GET",
      params: {
        ...params,
      },
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 POST /canvas/delete */
export async function deleteCanvas(
  body: CanvasContract.CanvasDeleteRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseCanvasCommandResultVO>(
    "/canvas/delete",
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      data: body,
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 POST /canvas/edit */
export async function editCanvas(
  body: CanvasContract.CanvasEditRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseCanvasCommandResultVO>(
    "/canvas/edit",
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      data: body,
      ...(options || {}),
    },
  );
}

/** getCanvas GET /canvas/get */
export async function getCanvasDocument(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CanvasContract.getCanvasDocumentParams,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseCanvasVO>("/canvas/get", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /canvas/graph/checkpoint */
export async function checkpointCanvasGraph(
  body: CanvasContract.CanvasCheckpointRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseCanvasCommandResultVO>(
    "/canvas/graph/checkpoint",
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      data: body,
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 POST /canvas/graph/checkpoint/list/page */
export async function listCanvasCheckpoints(
  body: CanvasContract.CanvasSnapshotQueryRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseCanvasSnapshotPageVO>(
    "/canvas/graph/checkpoint/list/page",
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      data: body,
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 POST /canvas/graph/restore */
export async function restoreCanvasGraph(
  body: CanvasContract.CanvasRestoreRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseCanvasCommandResultVO>(
    "/canvas/graph/restore",
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      data: body,
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 GET /canvas/graph/snapshot */
export async function getCanvasSnapshot(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CanvasContract.getCanvasSnapshotParams,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseCanvasSnapshotVO>(
    "/canvas/graph/snapshot",
    {
      method: "GET",
      params: {
        ...params,
      },
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 POST /canvas/list/page */
export async function listCanvases(
  body: CanvasContract.CanvasQueryRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseCanvasPageVO>("/canvas/list/page", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 本机addCanvas POST /canvas/local/add */
export async function localAddCanvas(
  body: CanvasContract.CanvasAddRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseCanvasCommandResultVO>(
    "/canvas/local/add",
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      data: body,
      ...(options || {}),
    },
  );
}

/** 本机getCanvasCommand GET /canvas/local/command/get */
export async function localGetCanvasCommand(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CanvasContract.localGetCanvasCommandParams,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseCanvasCommandStatusVO>(
    "/canvas/local/command/get",
    {
      method: "GET",
      params: {
        ...params,
      },
      ...(options || {}),
    },
  );
}

/** 本机deleteCanvas POST /canvas/local/delete */
export async function localDeleteCanvas(
  body: CanvasContract.CanvasDeleteRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseCanvasCommandResultVO>(
    "/canvas/local/delete",
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      data: body,
      ...(options || {}),
    },
  );
}

/** 本机editCanvas POST /canvas/local/edit */
export async function localEditCanvas(
  body: CanvasContract.CanvasEditRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseCanvasCommandResultVO>(
    "/canvas/local/edit",
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      data: body,
      ...(options || {}),
    },
  );
}

/** 本机getCanvas GET /canvas/local/get */
export async function localGetCanvas(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CanvasContract.localGetCanvasParams,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseCanvasVO>("/canvas/local/get", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** 本机checkpointCanvasGraph POST /canvas/local/graph/checkpoint */
export async function localCheckpointCanvasGraph(
  body: CanvasContract.CanvasCheckpointRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseCanvasCommandResultVO>(
    "/canvas/local/graph/checkpoint",
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      data: body,
      ...(options || {}),
    },
  );
}

/** 本机listCanvasCheckpoints POST /canvas/local/graph/checkpoint/list/page */
export async function localListCanvasCheckpoints(
  body: CanvasContract.CanvasSnapshotQueryRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseCanvasSnapshotPageVO>(
    "/canvas/local/graph/checkpoint/list/page",
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      data: body,
      ...(options || {}),
    },
  );
}

/** 本机restoreCanvasGraph POST /canvas/local/graph/restore */
export async function localRestoreCanvasGraph(
  body: CanvasContract.CanvasRestoreRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseCanvasCommandResultVO>(
    "/canvas/local/graph/restore",
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      data: body,
      ...(options || {}),
    },
  );
}

/** 本机getCanvasSnapshot GET /canvas/local/graph/snapshot */
export async function localGetCanvasSnapshot(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CanvasContract.localGetCanvasSnapshotParams,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseCanvasSnapshotVO>(
    "/canvas/local/graph/snapshot",
    {
      method: "GET",
      params: {
        ...params,
      },
      ...(options || {}),
    },
  );
}

/** 本机listCanvases POST /canvas/local/list/page */
export async function localListCanvases(
  body: CanvasContract.CanvasQueryRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseCanvasPageVO>(
    "/canvas/local/list/page",
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      data: body,
      ...(options || {}),
    },
  );
}
