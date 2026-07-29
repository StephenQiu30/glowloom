// @ts-ignore
/* eslint-disable */
import request, { type RequestOptions } from "@/lib/api-request";

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
