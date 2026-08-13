// @ts-ignore
/* eslint-disable */
import request, { type RequestOptions } from "@/lib/request";

/** Set Current Asset Version POST /api/v1/asset-states/${param0}/current-version */
export async function setCurrentAssetVersionApiV1AssetStatesStateIdCurrentVersionPost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.setCurrentAssetVersionApiV1AssetStatesStateIdCurrentVersionPostParams,
  body: API.AssetStateCurrentRequest,
  options?: RequestOptions
) {
  const { state_id: param0, ...queryParams } = params;
  return request<API.ApiResponseAssetStateResponse_>(
    `/api/v1/asset-states/${param0}/current-version`,
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

/** Decide Asset Occurrence POST /api/v1/asset-states/${param0}/occurrence-decisions */
export async function decideAssetOccurrenceApiV1AssetStatesStateIdOccurrenceDecisionsPost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.decideAssetOccurrenceApiV1AssetStatesStateIdOccurrenceDecisionsPostParams,
  body: API.AssetOccurrenceRequest,
  options?: RequestOptions
) {
  const { state_id: param0, ...queryParams } = params;
  return request<API.ApiResponseAssetOccurrenceDecisionResponse_>(
    `/api/v1/asset-states/${param0}/occurrence-decisions`,
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

/** List Asset Occurrences GET /api/v1/asset-states/${param0}/occurrences */
export async function listAssetOccurrencesApiV1AssetStatesStateIdOccurrencesGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listAssetOccurrencesApiV1AssetStatesStateIdOccurrencesGetParams,
  options?: RequestOptions
) {
  const { state_id: param0, ...queryParams } = params;
  return request<API.ApiResponsePaginatedAssetOccurrences_>(
    `/api/v1/asset-states/${param0}/occurrences`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    }
  );
}

/** Get Asset State Readiness GET /api/v1/asset-states/${param0}/readiness */
export async function getAssetStateReadinessApiV1AssetStatesStateIdReadinessGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getAssetStateReadinessApiV1AssetStatesStateIdReadinessGetParams,
  options?: RequestOptions
) {
  const { state_id: param0, ...queryParams } = params;
  return request<API.ApiResponseAssetStateReadinessResponse_>(
    `/api/v1/asset-states/${param0}/readiness`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    }
  );
}

/** List Asset Versions GET /api/v1/asset-states/${param0}/versions */
export async function listAssetVersionsApiV1AssetStatesStateIdVersionsGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listAssetVersionsApiV1AssetStatesStateIdVersionsGetParams,
  options?: RequestOptions
) {
  const { state_id: param0, ...queryParams } = params;
  return request<API.ApiResponsePaginatedAssetVersions_>(
    `/api/v1/asset-states/${param0}/versions`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    }
  );
}

/** Append Asset Version POST /api/v1/asset-states/${param0}/versions */
export async function appendAssetVersionApiV1AssetStatesStateIdVersionsPost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.appendAssetVersionApiV1AssetStatesStateIdVersionsPostParams,
  body: API.AssetVersionCreateRequest,
  options?: RequestOptions
) {
  const { state_id: param0, ...queryParams } = params;
  return request<API.ApiResponseAssetVersionCreateResponse_>(
    `/api/v1/asset-states/${param0}/versions`,
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

/** Get Asset Version GET /api/v1/asset-versions/${param0} */
export async function getAssetVersionApiV1AssetVersionsVersionIdGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getAssetVersionApiV1AssetVersionsVersionIdGetParams,
  options?: RequestOptions
) {
  const { version_id: param0, ...queryParams } = params;
  return request<API.ApiResponseAssetVersionResponse_>(
    `/api/v1/asset-versions/${param0}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Get Asset Readiness GET /api/v1/asset-versions/${param0}/readiness */
export async function getAssetReadinessApiV1AssetVersionsVersionIdReadinessGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getAssetReadinessApiV1AssetVersionsVersionIdReadinessGetParams,
  options?: RequestOptions
) {
  const { version_id: param0, ...queryParams } = params;
  return request<API.ApiResponseAssetReadinessResponse_>(
    `/api/v1/asset-versions/${param0}/readiness`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    }
  );
}

/** Get Asset GET /api/v1/assets/${param0} */
export async function getAssetApiV1AssetsAssetIdGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getAssetApiV1AssetsAssetIdGetParams,
  options?: RequestOptions
) {
  const { asset_id: param0, ...queryParams } = params;
  return request<API.ApiResponseAssetResponse_>(`/api/v1/assets/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** Delete Asset DELETE /api/v1/assets/${param0} */
export async function deleteAssetApiV1AssetsAssetIdDelete(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.deleteAssetApiV1AssetsAssetIdDeleteParams,
  options?: RequestOptions
) {
  const { asset_id: param0, ...queryParams } = params;
  return request<API.ApiResponseAssetDeleteResponse_>(
    `/api/v1/assets/${param0}`,
    {
      method: "DELETE",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    }
  );
}

/** Update Asset PATCH /api/v1/assets/${param0} */
export async function updateAssetApiV1AssetsAssetIdPatch(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.updateAssetApiV1AssetsAssetIdPatchParams,
  body: API.AssetUpdateRequest,
  options?: RequestOptions
) {
  const { asset_id: param0, ...queryParams } = params;
  return request<API.ApiResponseAssetResponse_>(`/api/v1/assets/${param0}`, {
    method: "PATCH",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** Archive Asset POST /api/v1/assets/${param0}/archive */
export async function archiveAssetApiV1AssetsAssetIdArchivePost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.archiveAssetApiV1AssetsAssetIdArchivePostParams,
  body: API.AssetStatusRequest,
  options?: RequestOptions
) {
  const { asset_id: param0, ...queryParams } = params;
  return request<API.ApiResponseAssetResponse_>(
    `/api/v1/assets/${param0}/archive`,
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

/** Asset Delete Preflight GET /api/v1/assets/${param0}/delete-preflight */
export async function assetDeletePreflightApiV1AssetsAssetIdDeletePreflightGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.assetDeletePreflightApiV1AssetsAssetIdDeletePreflightGetParams,
  options?: RequestOptions
) {
  const { asset_id: param0, ...queryParams } = params;
  return request<API.ApiResponseAssetDeletePreflightResponse_>(
    `/api/v1/assets/${param0}/delete-preflight`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Restore Asset POST /api/v1/assets/${param0}/restore */
export async function restoreAssetApiV1AssetsAssetIdRestorePost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.restoreAssetApiV1AssetsAssetIdRestorePostParams,
  body: API.AssetStatusRequest,
  options?: RequestOptions
) {
  const { asset_id: param0, ...queryParams } = params;
  return request<API.ApiResponseAssetResponse_>(
    `/api/v1/assets/${param0}/restore`,
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

/** List Asset States GET /api/v1/assets/${param0}/states */
export async function listAssetStatesApiV1AssetsAssetIdStatesGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listAssetStatesApiV1AssetsAssetIdStatesGetParams,
  options?: RequestOptions
) {
  const { asset_id: param0, ...queryParams } = params;
  return request<API.ApiResponsePaginatedAssetStates_>(
    `/api/v1/assets/${param0}/states`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Create Asset State POST /api/v1/assets/${param0}/states */
export async function createAssetStateApiV1AssetsAssetIdStatesPost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.createAssetStateApiV1AssetsAssetIdStatesPostParams,
  body: API.AssetStateCreateRequest,
  options?: RequestOptions
) {
  const { asset_id: param0, ...queryParams } = params;
  return request<API.ApiResponseAssetStateCreateResponse_>(
    `/api/v1/assets/${param0}/states`,
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

/** Get Asset Bible GET /api/v1/projects/${param0}/asset-bible */
export async function getAssetBibleApiV1ProjectsProjectIdAssetBibleGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getAssetBibleApiV1ProjectsProjectIdAssetBibleGetParams,
  options?: RequestOptions
) {
  const { project_id: param0, ...queryParams } = params;
  return request<API.ApiResponseAssetBibleResponse_>(
    `/api/v1/projects/${param0}/asset-bible`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    }
  );
}

/** List Assets GET /api/v1/projects/${param0}/assets */
export async function listAssetsApiV1ProjectsProjectIdAssetsGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listAssetsApiV1ProjectsProjectIdAssetsGetParams,
  options?: RequestOptions
) {
  const { project_id: param0, ...queryParams } = params;
  return request<API.ApiResponsePaginatedAssets_>(
    `/api/v1/projects/${param0}/assets`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    }
  );
}

/** Create Asset POST /api/v1/projects/${param0}/assets */
export async function createAssetApiV1ProjectsProjectIdAssetsPost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.createAssetApiV1ProjectsProjectIdAssetsPostParams,
  body: API.AssetCreateRequest,
  options?: RequestOptions
) {
  const { project_id: param0, ...queryParams } = params;
  return request<API.ApiResponseAssetResponse_>(
    `/api/v1/projects/${param0}/assets`,
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
