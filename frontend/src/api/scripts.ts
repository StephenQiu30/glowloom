// @ts-ignore
/* eslint-disable */
import request, { type RequestOptions } from "@/lib/api-request";

/** Set Current Version POST /api/v1/episodes/${param0}/current-script-version */
export async function setCurrentVersionApiV1EpisodesEpisodeIdCurrentScriptVersionPost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.setCurrentVersionApiV1EpisodesEpisodeIdCurrentScriptVersionPostParams,
  body: API.CurrentScriptVersionRequest,
  options?: RequestOptions
) {
  const { episode_id: param0, ...queryParams } = params;
  return request<API.ApiResponseCurrentScriptVersionResponse_>(
    `/api/v1/episodes/${param0}/current-script-version`,
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

/** Import Text Source POST /api/v1/episodes/${param0}/script-sources */
export async function importTextSourceApiV1EpisodesEpisodeIdScriptSourcesPost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.importTextSourceApiV1EpisodesEpisodeIdScriptSourcesPostParams,
  body: API.ScriptImportRequest,
  options?: RequestOptions
) {
  const { episode_id: param0, ...queryParams } = params;
  return request<API.ApiResponseScriptImportResponse_>(
    `/api/v1/episodes/${param0}/script-sources`,
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

/** Get Source GET /api/v1/script-sources/${param0} */
export async function getSourceApiV1ScriptSourcesSourceIdGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getSourceApiV1ScriptSourcesSourceIdGetParams,
  options?: RequestOptions
) {
  const { source_id: param0, ...queryParams } = params;
  return request<API.ApiResponseScriptSourceResponse_>(
    `/api/v1/script-sources/${param0}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Archive Source POST /api/v1/script-sources/${param0}/archive */
export async function archiveSourceApiV1ScriptSourcesSourceIdArchivePost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.archiveSourceApiV1ScriptSourcesSourceIdArchivePostParams,
  body: API.ScriptSourceStateRequest,
  options?: RequestOptions
) {
  const { source_id: param0, ...queryParams } = params;
  return request<API.ApiResponseScriptSourceResponse_>(
    `/api/v1/script-sources/${param0}/archive`,
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

/** Restore Source POST /api/v1/script-sources/${param0}/restore */
export async function restoreSourceApiV1ScriptSourcesSourceIdRestorePost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.restoreSourceApiV1ScriptSourcesSourceIdRestorePostParams,
  body: API.ScriptSourceStateRequest,
  options?: RequestOptions
) {
  const { source_id: param0, ...queryParams } = params;
  return request<API.ApiResponseScriptSourceResponse_>(
    `/api/v1/script-sources/${param0}/restore`,
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

/** List Versions GET /api/v1/script-sources/${param0}/versions */
export async function listVersionsApiV1ScriptSourcesSourceIdVersionsGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listVersionsApiV1ScriptSourcesSourceIdVersionsGetParams,
  options?: RequestOptions
) {
  const { source_id: param0, ...queryParams } = params;
  return request<API.ApiResponsePaginatedScriptVersions_>(
    `/api/v1/script-sources/${param0}/versions`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    }
  );
}

/** Publish Version POST /api/v1/script-sources/${param0}/versions */
export async function publishVersionApiV1ScriptSourcesSourceIdVersionsPost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.publishVersionApiV1ScriptSourcesSourceIdVersionsPostParams,
  body: API.ScriptVersionPublishRequest,
  options?: RequestOptions
) {
  const { source_id: param0, ...queryParams } = params;
  return request<API.ApiResponseScriptVersionPublishResponse_>(
    `/api/v1/script-sources/${param0}/versions`,
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

/** Get Version GET /api/v1/script-versions/${param0} */
export async function getVersionApiV1ScriptVersionsVersionIdGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getVersionApiV1ScriptVersionsVersionIdGetParams,
  options?: RequestOptions
) {
  const { version_id: param0, ...queryParams } = params;
  return request<API.ApiResponseScriptVersionResponse_>(
    `/api/v1/script-versions/${param0}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Diff Versions GET /api/v1/script-versions/${param0}/diff */
export async function diffVersionsApiV1ScriptVersionsVersionIdDiffGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.diffVersionsApiV1ScriptVersionsVersionIdDiffGetParams,
  options?: RequestOptions
) {
  const { version_id: param0, ...queryParams } = params;
  return request<API.ApiResponseScriptVersionDiffResponse_>(
    `/api/v1/script-versions/${param0}/diff`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    }
  );
}
