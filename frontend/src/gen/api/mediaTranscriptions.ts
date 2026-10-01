// @ts-ignore
/* eslint-disable */
import { request } from "@/lib/request";

/** Get local speech transcription GET /api/media-transcriptions/${param0} */
export async function getMediaTranscription(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getMediaTranscriptionParams,
  options?: import("@/lib/request").RequestOptions,
) {
  const { job_id: param0, ...queryParams } = params;
  return request<API.internalMediatoolAdapterHttpTranscriptionJobResponse>(
    `/api/media-transcriptions/${param0}`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    },
  );
}

/** Cancel local speech transcription POST /api/media-transcriptions/${param0}/cancel */
export async function cancelMediaTranscription(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.cancelMediaTranscriptionParams,
  body: API.githubComStephenQiu30LanverseBackendInternalMediatoolApplicationControlInput,
  options?: import("@/lib/request").RequestOptions,
) {
  const { job_id: param0, ...queryParams } = params;
  return request<any>(`/api/media-transcriptions/${param0}/cancel`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** Get local speech subtitle draft GET /api/media-transcriptions/${param0}/result */
export async function getMediaTranscriptionResult(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getMediaTranscriptionResultParams,
  options?: import("@/lib/request").RequestOptions,
) {
  const { job_id: param0, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalMediatoolApplicationTranscriptionResult>(
    `/api/media-transcriptions/${param0}/result`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    },
  );
}

/** Retry failed or cancelled local speech transcription POST /api/media-transcriptions/${param0}/retry */
export async function retryMediaTranscription(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.retryMediaTranscriptionParams,
  body: API.githubComStephenQiu30LanverseBackendInternalMediatoolApplicationControlInput,
  options?: import("@/lib/request").RequestOptions,
) {
  const { job_id: param0, ...queryParams } = params;
  return request<any>(`/api/media-transcriptions/${param0}/retry`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** Download recognized subtitle draft as SRT GET /api/media-transcriptions/${param0}/subtitles */
export async function downloadMediaTranscriptionSubtitles(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.downloadMediaTranscriptionSubtitlesParams,
  options?: import("@/lib/request").RequestOptions,
) {
  const { job_id: param0, ...queryParams } = params;
  return request<string>(`/api/media-transcriptions/${param0}/subtitles`, {
    method: "GET",
    params: {
      ...queryParams,
    },
    ...(options || {}),
  });
}

/** List project media transcriptions GET /api/projects/${param0}/media-transcriptions */
export async function listMediaTranscriptions(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listMediaTranscriptionsParams,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalMediatoolApplicationTranscriptionPage>(
    `/api/projects/${param0}/media-transcriptions`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    },
  );
}

/** Create local speech transcription POST /api/projects/${param0}/media-transcriptions */
export async function createMediaTranscription(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.createMediaTranscriptionParams,
  body: API.githubComStephenQiu30LanverseBackendInternalMediatoolApplicationTranscriptionCreateInput,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<any>(`/api/projects/${param0}/media-transcriptions`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}
