// @ts-ignore
/* eslint-disable */
import { request } from "@/lib/request";
import type { RequestOptions } from "@/lib/request";

/** Get local video depth depth GET /api/media-depths/${param0} */
export async function getMediaDepth(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getMediaDepthParams,
  options?: RequestOptions,
) {
  const { job_id: param0, ...queryParams } = params;
  return request<API.DepthJobResponse>(`/api/media-depths/${param0}`, {
    method: "GET",
    params: {
      ...queryParams,
    },
    ...(options || {}),
  });
}

/** Cancel local video depth depth POST /api/media-depths/${param0}/cancel */
export async function cancelMediaDepth(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.cancelMediaDepthParams,
  body: API.ControlInput,
  options?: RequestOptions,
) {
  const { job_id: param0, ...queryParams } = params;
  return request<any>(`/api/media-depths/${param0}/cancel`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** Download reviewed local depth output GET /api/media-depths/${param0}/download */
export async function downloadMediaDepth(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.downloadMediaDepthParams,
  options?: RequestOptions,
) {
  const { job_id: param0, ...queryParams } = params;
  return request<Blob>(`/api/media-depths/${param0}/download`, {
    method: "GET",
    params: {
      ...queryParams,
    },
    ...(options || { responseType: "blob" }),
  });
}

/** Preview exact local depth output GET /api/media-depths/${param0}/preview */
export async function previewMediaDepth(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.previewMediaDepthParams,
  options?: RequestOptions,
) {
  const { job_id: param0, ...queryParams } = params;
  return request<API.DepthPreview>(`/api/media-depths/${param0}/preview`, {
    method: "GET",
    params: {
      ...queryParams,
    },
    ...(options || {}),
  });
}

/** Reconcile original local depth result objects POST /api/media-depths/${param0}/reconcile */
export async function reconcileMediaDepth(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.reconcileMediaDepthParams,
  body: API.ControlInput,
  options?: RequestOptions,
) {
  const { job_id: param0, ...queryParams } = params;
  return request<any>(`/api/media-depths/${param0}/reconcile`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** Retry failed or cancelled local video depth depth POST /api/media-depths/${param0}/retry */
export async function retryMediaDepth(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.retryMediaDepthParams,
  body: API.ControlInput,
  options?: RequestOptions,
) {
  const { job_id: param0, ...queryParams } = params;
  return request<any>(`/api/media-depths/${param0}/retry`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** Review exact local depth output POST /api/media-depths/${param0}/review */
export async function reviewMediaDepth(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.reviewMediaDepthParams,
  body: API.ReviewInput,
  options?: RequestOptions,
) {
  const { job_id: param0, ...queryParams } = params;
  return request<API.DepthJobResponse>(`/api/media-depths/${param0}/review`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** List project media depths GET /api/projects/${param0}/media-depths */
export async function listMediaDepths(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listMediaDepthsParams,
  options?: RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.DepthPage>(`/api/projects/${param0}/media-depths`, {
    method: "GET",
    params: {
      ...queryParams,
    },
    ...(options || {}),
  });
}

/** Create local video depth depth POST /api/projects/${param0}/media-depths */
export async function createMediaDepth(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.createMediaDepthParams,
  body: API.DepthCreateInput,
  options?: RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<any>(`/api/projects/${param0}/media-depths`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}
