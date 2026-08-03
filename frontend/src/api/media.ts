// @ts-ignore
/* eslint-disable */
import request, { type RequestOptions } from "@/lib/api-request";

/** List Media GET /api/v1/media */
export async function listMediaApiV1MediaGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listMediaApiV1MediaGetParams,
  options?: RequestOptions
) {
  return request<API.ApiResponsePaginatedMedia_>("/api/v1/media", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** Archive Media POST /api/v1/media-objects/${param0}/archive */
export async function archiveMediaApiV1MediaObjectsMediaObjectIdArchivePost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.archiveMediaApiV1MediaObjectsMediaObjectIdArchivePostParams,
  body: API.ArchiveMediaRequest,
  options?: RequestOptions
) {
  const { media_object_id: param0, ...queryParams } = params;
  return request<API.ApiResponseMediaObjectResponse_>(
    `/api/v1/media-objects/${param0}/archive`,
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

/** Set Current Media Version POST /api/v1/media-objects/${param0}/current-version */
export async function setCurrentMediaVersionApiV1MediaObjectsMediaObjectIdCurrentVersionPost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.setCurrentMediaVersionApiV1MediaObjectsMediaObjectIdCurrentVersionPostParams,
  body: API.CurrentMediaVersionRequest,
  options?: RequestOptions
) {
  const { media_object_id: param0, ...queryParams } = params;
  return request<API.ApiResponseMediaObjectResponse_>(
    `/api/v1/media-objects/${param0}/current-version`,
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

/** Restore Media POST /api/v1/media-objects/${param0}/restore */
export async function restoreMediaApiV1MediaObjectsMediaObjectIdRestorePost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.restoreMediaApiV1MediaObjectsMediaObjectIdRestorePostParams,
  body: API.ArchiveMediaRequest,
  options?: RequestOptions
) {
  const { media_object_id: param0, ...queryParams } = params;
  return request<API.ApiResponseMediaObjectResponse_>(
    `/api/v1/media-objects/${param0}/restore`,
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

/** Initialize Version Upload POST /api/v1/media-objects/${param0}/versions */
export async function initializeVersionUploadApiV1MediaObjectsMediaObjectIdVersionsPost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.initializeVersionUploadApiV1MediaObjectsMediaObjectIdVersionsPostParams,
  body: API.AppendVersionRequest,
  options?: RequestOptions
) {
  const { media_object_id: param0, ...queryParams } = params;
  return request<API.ApiResponseUploadInitializationResponse_>(
    `/api/v1/media-objects/${param0}/versions`,
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

/** Get Media GET /api/v1/media/${param0} */
export async function getMediaApiV1MediaVersionIdGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getMediaApiV1MediaVersionIdGetParams,
  options?: RequestOptions
) {
  const { version_id: param0, ...queryParams } = params;
  return request<API.ApiResponseMediaVersionResponse_>(
    `/api/v1/media/${param0}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Create Access POST /api/v1/media/${param0}/access */
export async function createAccessApiV1MediaVersionIdAccessPost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.createAccessApiV1MediaVersionIdAccessPostParams,
  body: API.MediaAccessRequest,
  options?: RequestOptions
) {
  const { version_id: param0, ...queryParams } = params;
  return request<API.ApiResponseMediaAccessResponse_>(
    `/api/v1/media/${param0}/access`,
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

/** Retry Probe POST /api/v1/media/${param0}/probe-retry */
export async function retryProbeApiV1MediaVersionIdProbeRetryPost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.retryProbeApiV1MediaVersionIdProbeRetryPostParams,
  body: API.ProbeRetryRequest,
  options?: RequestOptions
) {
  const { version_id: param0, ...queryParams } = params;
  return request<API.ApiResponseTaskResponse_>(
    `/api/v1/media/${param0}/probe-retry`,
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

/** Initialize Upload POST /api/v1/media/uploads */
export async function initializeUploadApiV1MediaUploadsPost(
  body: API.UploadDeclaration,
  options?: RequestOptions
) {
  return request<API.ApiResponseUploadInitializationResponse_>(
    "/api/v1/media/uploads",
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      data: body,
      ...(options || {}),
    }
  );
}

/** Complete Upload POST /api/v1/media/uploads/${param0}/complete */
export async function completeUploadApiV1MediaUploadsUploadSessionIdCompletePost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.completeUploadApiV1MediaUploadsUploadSessionIdCompletePostParams,
  options?: RequestOptions
) {
  const { upload_session_id: param0, ...queryParams } = params;
  return request<API.ApiResponseUploadCompletionResponse_>(
    `/api/v1/media/uploads/${param0}/complete`,
    {
      method: "POST",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}
