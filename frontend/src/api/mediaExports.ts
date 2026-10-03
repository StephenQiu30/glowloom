// @ts-ignore
/* eslint-disable */
import { request } from "@/lib/request";

/** Get local media export GET /api/media-exports/${param0} */
export async function getMediaExport(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getMediaExportParams,
  options?: import("@/lib/request").RequestOptions,
) {
  const { job_id: param0, ...queryParams } = params;
  return request<API.internalMediatoolAdapterHttpExportJobResponse>(
    `/api/media-exports/${param0}`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    },
  );
}

/** Cancel local media export POST /api/media-exports/${param0}/cancel */
export async function cancelMediaExport(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.cancelMediaExportParams,
  body: API.githubComStephenQiu30LanverseBackendInternalMediatoolApplicationControlInput,
  options?: import("@/lib/request").RequestOptions,
) {
  const { job_id: param0, ...queryParams } = params;
  return request<any>(`/api/media-exports/${param0}/cancel`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** Download reviewed media export GET /api/media-exports/${param0}/download */
export async function downloadMediaExport(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.downloadMediaExportParams,
  options?: import("@/lib/request").RequestOptions,
) {
  const { job_id: param0, ...queryParams } = params;
  return request<string>(`/api/media-exports/${param0}/download`, {
    method: "GET",
    params: {
      ...queryParams,
    },
    ...(options || {}),
  });
}

/** Preview pending or reviewed media export GET /api/media-exports/${param0}/preview */
export async function previewMediaExport(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.previewMediaExportParams,
  options?: import("@/lib/request").RequestOptions,
) {
  const { job_id: param0, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalMediatoolApplicationExportPreview>(
    `/api/media-exports/${param0}/preview`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    },
  );
}

/** Retry failed or cancelled local media export POST /api/media-exports/${param0}/retry */
export async function retryMediaExport(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.retryMediaExportParams,
  body: API.githubComStephenQiu30LanverseBackendInternalMediatoolApplicationControlInput,
  options?: import("@/lib/request").RequestOptions,
) {
  const { job_id: param0, ...queryParams } = params;
  return request<any>(`/api/media-exports/${param0}/retry`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** Confirm review of actual exported file POST /api/media-exports/${param0}/review */
export async function reviewMediaExport(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.reviewMediaExportParams,
  body: API.githubComStephenQiu30LanverseBackendInternalMediatoolApplicationReviewInput,
  options?: import("@/lib/request").RequestOptions,
) {
  const { job_id: param0, ...queryParams } = params;
  return request<API.internalMediatoolAdapterHttpExportJobResponse>(
    `/api/media-exports/${param0}/review`,
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

/** Download timeline subtitles as SRT GET /api/media-exports/${param0}/subtitles */
export async function downloadMediaExportSubtitles(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.downloadMediaExportSubtitlesParams,
  options?: import("@/lib/request").RequestOptions,
) {
  const { job_id: param0, ...queryParams } = params;
  return request<string>(`/api/media-exports/${param0}/subtitles`, {
    method: "GET",
    params: {
      ...queryParams,
    },
    ...(options || {}),
  });
}

/** List project media exports GET /api/projects/${param0}/media-exports */
export async function listMediaExports(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listMediaExportsParams,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalMediatoolApplicationExportPage>(
    `/api/projects/${param0}/media-exports`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    },
  );
}

/** Create local timeline export POST /api/projects/${param0}/media-exports */
export async function createMediaExport(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.createMediaExportParams,
  body: API.githubComStephenQiu30LanverseBackendInternalMediatoolApplicationCreateInput,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<any>(`/api/projects/${param0}/media-exports`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}
