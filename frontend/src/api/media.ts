import request, { type RequestOptions } from "@/lib/request";

/** List Media GET /api/v1/media */
export async function listMediaApiV1MediaGet(
  params: API.listMediaApiV1MediaGetParams,
  options?: RequestOptions,
) {
  return request<API.ApiResponsePaginatedMedia_>(`/api/v1/media`, {
    method: "GET",
    params,
    ...(options ?? {}),
  });
}

/** Archive Media POST /api/v1/media-objects/{media_object_id}/archive */
export async function archiveMediaApiV1MediaObjectsMediaObjectIdArchivePost(
  params: API.archiveMediaApiV1MediaObjectsMediaObjectIdArchivePostParams,
  body: API.ArchiveMediaRequest,
  options?: RequestOptions,
) {
  const { media_object_id: path0 } = params;
  return request<API.ApiResponseMediaObjectResponse_>(`/api/v1/media-objects/${path0}/archive`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Set Current Media Version POST /api/v1/media-objects/{media_object_id}/current-version */
export async function setCurrentMediaVersionApiV1MediaObjectsMediaObjectIdCurrentVersionPost(
  params: API.setCurrentMediaVersionApiV1MediaObjectsMediaObjectIdCurrentVersionPostParams,
  body: API.CurrentMediaVersionRequest,
  options?: RequestOptions,
) {
  const { media_object_id: path0 } = params;
  return request<API.ApiResponseMediaObjectResponse_>(`/api/v1/media-objects/${path0}/current-version`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Restore Media POST /api/v1/media-objects/{media_object_id}/restore */
export async function restoreMediaApiV1MediaObjectsMediaObjectIdRestorePost(
  params: API.restoreMediaApiV1MediaObjectsMediaObjectIdRestorePostParams,
  body: API.ArchiveMediaRequest,
  options?: RequestOptions,
) {
  const { media_object_id: path0 } = params;
  return request<API.ApiResponseMediaObjectResponse_>(`/api/v1/media-objects/${path0}/restore`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Initialize Version Upload POST /api/v1/media-objects/{media_object_id}/versions */
export async function initializeVersionUploadApiV1MediaObjectsMediaObjectIdVersionsPost(
  params: API.initializeVersionUploadApiV1MediaObjectsMediaObjectIdVersionsPostParams,
  body: API.AppendVersionRequest,
  options?: RequestOptions,
) {
  const { media_object_id: path0 } = params;
  return request<API.ApiResponseUploadInitializationResponse_>(`/api/v1/media-objects/${path0}/versions`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Get Media GET /api/v1/media/{version_id} */
export async function getMediaApiV1MediaVersionIdGet(
  params: API.getMediaApiV1MediaVersionIdGetParams,
  options?: RequestOptions,
) {
  const { version_id: path0 } = params;
  return request<API.ApiResponseMediaVersionResponse_>(`/api/v1/media/${path0}`, {
    method: "GET",
    ...(options ?? {}),
  });
}

/** Create Access POST /api/v1/media/{version_id}/access */
export async function createAccessApiV1MediaVersionIdAccessPost(
  params: API.createAccessApiV1MediaVersionIdAccessPostParams,
  body: API.MediaAccessRequest,
  options?: RequestOptions,
) {
  const { version_id: path0 } = params;
  return request<API.ApiResponseMediaAccessResponse_>(`/api/v1/media/${path0}/access`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Request Media Location Migration POST /api/v1/media/{version_id}/location-migrations */
export async function requestMediaLocationMigrationApiV1MediaVersionIdLocationMigrationsPost(
  params: API.requestMediaLocationMigrationApiV1MediaVersionIdLocationMigrationsPostParams,
  body: API.MediaLocationMigrationRequest,
  options?: RequestOptions,
) {
  const { version_id: path0 } = params;
  return request<API.ApiResponseTaskResponse_>(`/api/v1/media/${path0}/location-migrations`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Request Media Location Rollback POST /api/v1/media/{version_id}/location-rollbacks */
export async function requestMediaLocationRollbackApiV1MediaVersionIdLocationRollbacksPost(
  params: API.requestMediaLocationRollbackApiV1MediaVersionIdLocationRollbacksPostParams,
  body: API.MediaLocationRollbackRequest,
  options?: RequestOptions,
) {
  const { version_id: path0 } = params;
  return request<API.ApiResponseTaskResponse_>(`/api/v1/media/${path0}/location-rollbacks`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** List Media Locations GET /api/v1/media/{version_id}/locations */
export async function listMediaLocationsApiV1MediaVersionIdLocationsGet(
  params: API.listMediaLocationsApiV1MediaVersionIdLocationsGetParams,
  options?: RequestOptions,
) {
  const { version_id: path0 } = params;
  return request<API.ApiResponseMediaLocationsResponse_>(`/api/v1/media/${path0}/locations`, {
    method: "GET",
    ...(options ?? {}),
  });
}

/** Retry Probe POST /api/v1/media/{version_id}/probe-retry */
export async function retryProbeApiV1MediaVersionIdProbeRetryPost(
  params: API.retryProbeApiV1MediaVersionIdProbeRetryPostParams,
  body: API.ProbeRetryRequest,
  options?: RequestOptions,
) {
  const { version_id: path0 } = params;
  return request<API.ApiResponseTaskResponse_>(`/api/v1/media/${path0}/probe-retry`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Initialize Upload POST /api/v1/media/uploads */
export async function initializeUploadApiV1MediaUploadsPost(
  body: API.UploadDeclaration,
  options?: RequestOptions,
) {
  return request<API.ApiResponseUploadInitializationResponse_>(`/api/v1/media/uploads`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Complete Upload POST /api/v1/media/uploads/{upload_session_id}/complete */
export async function completeUploadApiV1MediaUploadsUploadSessionIdCompletePost(
  params: API.completeUploadApiV1MediaUploadsUploadSessionIdCompletePostParams,
  options?: RequestOptions,
) {
  const { upload_session_id: path0 } = params;
  return request<API.ApiResponseUploadCompletionResponse_>(`/api/v1/media/uploads/${path0}/complete`, {
    method: "POST",
    ...(options ?? {}),
  });
}
