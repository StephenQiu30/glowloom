import request, { type RequestOptions } from "@/lib/request";

/** Update Asset State PATCH /api/v1/asset-states/{state_id} */
export async function updateAssetStateApiV1AssetStatesStateIdPatch(
  params: API.updateAssetStateApiV1AssetStatesStateIdPatchParams,
  body: API.AssetStateUpdateRequest,
  options?: RequestOptions,
) {
  const { state_id: path0 } = params;
  return request<API.ApiResponseAssetStateResponse_>(`/api/v1/asset-states/${path0}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Set Current Asset Version POST /api/v1/asset-states/{state_id}/current-version */
export async function setCurrentAssetVersionApiV1AssetStatesStateIdCurrentVersionPost(
  params: API.setCurrentAssetVersionApiV1AssetStatesStateIdCurrentVersionPostParams,
  body: API.AssetStateCurrentRequest,
  options?: RequestOptions,
) {
  const { state_id: path0 } = params;
  return request<API.ApiResponseAssetStateCurrentResponse_>(`/api/v1/asset-states/${path0}/current-version`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Current Asset Version Preflight POST /api/v1/asset-states/{state_id}/current-version-preflight */
export async function currentAssetVersionPreflightApiV1AssetStatesStateIdCurrentVersionPreflightPost(
  params: API.currentAssetVersionPreflightApiV1AssetStatesStateIdCurrentVersionPreflightPostParams,
  body: API.AssetStateCurrentPreflightRequest,
  options?: RequestOptions,
) {
  const { state_id: path0 } = params;
  return request<API.ApiResponseAssetImpactResponse_>(`/api/v1/asset-states/${path0}/current-version-preflight`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Disable Asset State POST /api/v1/asset-states/{state_id}/disable */
export async function disableAssetStateApiV1AssetStatesStateIdDisablePost(
  params: API.disableAssetStateApiV1AssetStatesStateIdDisablePostParams,
  body: API.AssetDisableRequest,
  options?: RequestOptions,
) {
  const { state_id: path0 } = params;
  return request<API.ApiResponseAssetStateAvailabilityResponse_>(`/api/v1/asset-states/${path0}/disable`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Asset State Disable Preflight POST /api/v1/asset-states/{state_id}/disable-preflight */
export async function assetStateDisablePreflightApiV1AssetStatesStateIdDisablePreflightPost(
  params: API.assetStateDisablePreflightApiV1AssetStatesStateIdDisablePreflightPostParams,
  body: API.AssetDisablePreflightRequest,
  options?: RequestOptions,
) {
  const { state_id: path0 } = params;
  return request<API.ApiResponseAssetImpactResponse_>(`/api/v1/asset-states/${path0}/disable-preflight`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Enable Asset State POST /api/v1/asset-states/{state_id}/enable */
export async function enableAssetStateApiV1AssetStatesStateIdEnablePost(
  params: API.enableAssetStateApiV1AssetStatesStateIdEnablePostParams,
  body: API.AssetStateEnableRequest,
  options?: RequestOptions,
) {
  const { state_id: path0 } = params;
  return request<API.ApiResponseAssetStateResponse_>(`/api/v1/asset-states/${path0}/enable`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Decide Asset Occurrence POST /api/v1/asset-states/{state_id}/occurrence-decisions */
export async function decideAssetOccurrenceApiV1AssetStatesStateIdOccurrenceDecisionsPost(
  params: API.decideAssetOccurrenceApiV1AssetStatesStateIdOccurrenceDecisionsPostParams,
  body: API.AssetOccurrenceRequest,
  options?: RequestOptions,
) {
  const { state_id: path0 } = params;
  return request<API.ApiResponseAssetOccurrenceDecisionResponse_>(`/api/v1/asset-states/${path0}/occurrence-decisions`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** List Asset Occurrences GET /api/v1/asset-states/{state_id}/occurrences */
export async function listAssetOccurrencesApiV1AssetStatesStateIdOccurrencesGet(
  params: API.listAssetOccurrencesApiV1AssetStatesStateIdOccurrencesGetParams,
  options?: RequestOptions,
) {
  const { state_id: path0, ...queryParams } = params;
  return request<API.ApiResponsePaginatedAssetOccurrences_>(`/api/v1/asset-states/${path0}/occurrences`, {
    method: "GET",
    params: queryParams,
    ...(options ?? {}),
  });
}

/** Get Asset State Readiness GET /api/v1/asset-states/{state_id}/readiness */
export async function getAssetStateReadinessApiV1AssetStatesStateIdReadinessGet(
  params: API.getAssetStateReadinessApiV1AssetStatesStateIdReadinessGetParams,
  options?: RequestOptions,
) {
  const { state_id: path0, ...queryParams } = params;
  return request<API.ApiResponseAssetStateReadinessResponse_>(`/api/v1/asset-states/${path0}/readiness`, {
    method: "GET",
    params: queryParams,
    ...(options ?? {}),
  });
}

/** List Asset Versions GET /api/v1/asset-states/{state_id}/versions */
export async function listAssetVersionsApiV1AssetStatesStateIdVersionsGet(
  params: API.listAssetVersionsApiV1AssetStatesStateIdVersionsGetParams,
  options?: RequestOptions,
) {
  const { state_id: path0, ...queryParams } = params;
  return request<API.ApiResponsePaginatedAssetVersions_>(`/api/v1/asset-states/${path0}/versions`, {
    method: "GET",
    params: queryParams,
    ...(options ?? {}),
  });
}

/** Append Asset Version POST /api/v1/asset-states/{state_id}/versions */
export async function appendAssetVersionApiV1AssetStatesStateIdVersionsPost(
  params: API.appendAssetVersionApiV1AssetStatesStateIdVersionsPostParams,
  body: API.AssetVersionCreateRequest,
  options?: RequestOptions,
) {
  const { state_id: path0 } = params;
  return request<API.ApiResponseAssetVersionCreateResponse_>(`/api/v1/asset-states/${path0}/versions`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Get Asset Version GET /api/v1/asset-versions/{version_id} */
export async function getAssetVersionApiV1AssetVersionsVersionIdGet(
  params: API.getAssetVersionApiV1AssetVersionsVersionIdGetParams,
  options?: RequestOptions,
) {
  const { version_id: path0 } = params;
  return request<API.ApiResponseAssetVersionResponse_>(`/api/v1/asset-versions/${path0}`, {
    method: "GET",
    ...(options ?? {}),
  });
}

/** Get Asset Readiness GET /api/v1/asset-versions/{version_id}/readiness */
export async function getAssetReadinessApiV1AssetVersionsVersionIdReadinessGet(
  params: API.getAssetReadinessApiV1AssetVersionsVersionIdReadinessGetParams,
  options?: RequestOptions,
) {
  const { version_id: path0, ...queryParams } = params;
  return request<API.ApiResponseAssetReadinessResponse_>(`/api/v1/asset-versions/${path0}/readiness`, {
    method: "GET",
    params: queryParams,
    ...(options ?? {}),
  });
}

/** Delete Asset DELETE /api/v1/assets/{asset_id} */
export async function deleteAssetApiV1AssetsAssetIdDelete(
  params: API.deleteAssetApiV1AssetsAssetIdDeleteParams,
  options?: RequestOptions,
) {
  const { asset_id: path0, ...queryParams } = params;
  return request<API.ApiResponseAssetDeleteResponse_>(`/api/v1/assets/${path0}`, {
    method: "DELETE",
    params: queryParams,
    ...(options ?? {}),
  });
}

/** Get Asset GET /api/v1/assets/{asset_id} */
export async function getAssetApiV1AssetsAssetIdGet(
  params: API.getAssetApiV1AssetsAssetIdGetParams,
  options?: RequestOptions,
) {
  const { asset_id: path0 } = params;
  return request<API.ApiResponseAssetResponse_>(`/api/v1/assets/${path0}`, {
    method: "GET",
    ...(options ?? {}),
  });
}

/** Update Asset PATCH /api/v1/assets/{asset_id} */
export async function updateAssetApiV1AssetsAssetIdPatch(
  params: API.updateAssetApiV1AssetsAssetIdPatchParams,
  body: API.AssetUpdateRequest,
  options?: RequestOptions,
) {
  const { asset_id: path0 } = params;
  return request<API.ApiResponseAssetResponse_>(`/api/v1/assets/${path0}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Archive Asset POST /api/v1/assets/{asset_id}/archive */
export async function archiveAssetApiV1AssetsAssetIdArchivePost(
  params: API.archiveAssetApiV1AssetsAssetIdArchivePostParams,
  body: API.AssetStatusRequest,
  options?: RequestOptions,
) {
  const { asset_id: path0 } = params;
  return request<API.ApiResponseAssetResponse_>(`/api/v1/assets/${path0}/archive`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Asset Delete Preflight GET /api/v1/assets/{asset_id}/delete-preflight */
export async function assetDeletePreflightApiV1AssetsAssetIdDeletePreflightGet(
  params: API.assetDeletePreflightApiV1AssetsAssetIdDeletePreflightGetParams,
  options?: RequestOptions,
) {
  const { asset_id: path0 } = params;
  return request<API.ApiResponseAssetDeletePreflightResponse_>(`/api/v1/assets/${path0}/delete-preflight`, {
    method: "GET",
    ...(options ?? {}),
  });
}

/** Disable Asset POST /api/v1/assets/{asset_id}/disable */
export async function disableAssetApiV1AssetsAssetIdDisablePost(
  params: API.disableAssetApiV1AssetsAssetIdDisablePostParams,
  body: API.AssetDisableRequest,
  options?: RequestOptions,
) {
  const { asset_id: path0 } = params;
  return request<API.ApiResponseAssetAvailabilityResponse_>(`/api/v1/assets/${path0}/disable`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Asset Disable Preflight POST /api/v1/assets/{asset_id}/disable-preflight */
export async function assetDisablePreflightApiV1AssetsAssetIdDisablePreflightPost(
  params: API.assetDisablePreflightApiV1AssetsAssetIdDisablePreflightPostParams,
  body: API.AssetDisablePreflightRequest,
  options?: RequestOptions,
) {
  const { asset_id: path0 } = params;
  return request<API.ApiResponseAssetImpactResponse_>(`/api/v1/assets/${path0}/disable-preflight`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Enable Asset POST /api/v1/assets/{asset_id}/enable */
export async function enableAssetApiV1AssetsAssetIdEnablePost(
  params: API.enableAssetApiV1AssetsAssetIdEnablePostParams,
  body: API.AssetEnableRequest,
  options?: RequestOptions,
) {
  const { asset_id: path0 } = params;
  return request<API.ApiResponseAssetResponse_>(`/api/v1/assets/${path0}/enable`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Rename Asset POST /api/v1/assets/{asset_id}/rename */
export async function renameAssetApiV1AssetsAssetIdRenamePost(
  params: API.renameAssetApiV1AssetsAssetIdRenamePostParams,
  body: API.AssetRenameRequest,
  options?: RequestOptions,
) {
  const { asset_id: path0 } = params;
  return request<API.ApiResponseAssetRenameResponse_>(`/api/v1/assets/${path0}/rename`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Asset Rename Preflight POST /api/v1/assets/{asset_id}/rename-preflight */
export async function assetRenamePreflightApiV1AssetsAssetIdRenamePreflightPost(
  params: API.assetRenamePreflightApiV1AssetsAssetIdRenamePreflightPostParams,
  body: API.AssetRenamePreflightRequest,
  options?: RequestOptions,
) {
  const { asset_id: path0 } = params;
  return request<API.ApiResponseAssetImpactResponse_>(`/api/v1/assets/${path0}/rename-preflight`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Restore Asset POST /api/v1/assets/{asset_id}/restore */
export async function restoreAssetApiV1AssetsAssetIdRestorePost(
  params: API.restoreAssetApiV1AssetsAssetIdRestorePostParams,
  body: API.AssetStatusRequest,
  options?: RequestOptions,
) {
  const { asset_id: path0 } = params;
  return request<API.ApiResponseAssetResponse_>(`/api/v1/assets/${path0}/restore`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** List Asset States GET /api/v1/assets/{asset_id}/states */
export async function listAssetStatesApiV1AssetsAssetIdStatesGet(
  params: API.listAssetStatesApiV1AssetsAssetIdStatesGetParams,
  options?: RequestOptions,
) {
  const { asset_id: path0 } = params;
  return request<API.ApiResponsePaginatedAssetStates_>(`/api/v1/assets/${path0}/states`, {
    method: "GET",
    ...(options ?? {}),
  });
}

/** Create Asset State POST /api/v1/assets/{asset_id}/states */
export async function createAssetStateApiV1AssetsAssetIdStatesPost(
  params: API.createAssetStateApiV1AssetsAssetIdStatesPostParams,
  body: API.AssetStateCreateRequest,
  options?: RequestOptions,
) {
  const { asset_id: path0 } = params;
  return request<API.ApiResponseAssetStateCreateResponse_>(`/api/v1/assets/${path0}/states`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Get Asset Bible GET /api/v1/projects/{project_id}/asset-bible */
export async function getAssetBibleApiV1ProjectsProjectIdAssetBibleGet(
  params: API.getAssetBibleApiV1ProjectsProjectIdAssetBibleGetParams,
  options?: RequestOptions,
) {
  const { project_id: path0, ...queryParams } = params;
  return request<API.ApiResponseAssetBibleResponse_>(`/api/v1/projects/${path0}/asset-bible`, {
    method: "GET",
    params: queryParams,
    ...(options ?? {}),
  });
}

/** List Assets GET /api/v1/projects/{project_id}/assets */
export async function listAssetsApiV1ProjectsProjectIdAssetsGet(
  params: API.listAssetsApiV1ProjectsProjectIdAssetsGetParams,
  options?: RequestOptions,
) {
  const { project_id: path0, ...queryParams } = params;
  return request<API.ApiResponsePaginatedAssets_>(`/api/v1/projects/${path0}/assets`, {
    method: "GET",
    params: queryParams,
    ...(options ?? {}),
  });
}

/** Create Asset POST /api/v1/projects/{project_id}/assets */
export async function createAssetApiV1ProjectsProjectIdAssetsPost(
  params: API.createAssetApiV1ProjectsProjectIdAssetsPostParams,
  body: API.AssetCreateRequest,
  options?: RequestOptions,
) {
  const { project_id: path0 } = params;
  return request<API.ApiResponseAssetResponse_>(`/api/v1/projects/${path0}/assets`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}
