// @ts-ignore
/* eslint-disable */
import request, { type RequestOptions } from "@/lib/api-request";

/** List Asset Shot Usages GET /api/v1/asset-versions/${param0}/shot-usages */
export async function listAssetShotUsagesApiV1AssetVersionsAssetVersionIdShotUsagesGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listAssetShotUsagesApiV1AssetVersionsAssetVersionIdShotUsagesGetParams,
  options?: RequestOptions
) {
  const { asset_version_id: param0, ...queryParams } = params;
  return request<API.ApiResponsePaginatedAssetShotUsages_>(
    `/api/v1/asset-versions/${param0}/shot-usages`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    }
  );
}

/** Apply Asset Upgrade POST /api/v1/asset-versions/${param0}/upgrade */
export async function applyAssetUpgradeApiV1AssetVersionsAssetVersionIdUpgradePost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.applyAssetUpgradeApiV1AssetVersionsAssetVersionIdUpgradePostParams,
  body: API.AssetUpgradeApplyRequest,
  options?: RequestOptions
) {
  const { asset_version_id: param0, ...queryParams } = params;
  return request<API.ApiResponseAssetUpgradeApplyResponse_>(
    `/api/v1/asset-versions/${param0}/upgrade`,
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

/** Preflight Asset Upgrade POST /api/v1/asset-versions/${param0}/upgrade-preflight */
export async function preflightAssetUpgradeApiV1AssetVersionsAssetVersionIdUpgradePreflightPost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.preflightAssetUpgradeApiV1AssetVersionsAssetVersionIdUpgradePreflightPostParams,
  body: API.AssetUpgradePreflightRequest,
  options?: RequestOptions
) {
  const { asset_version_id: param0, ...queryParams } = params;
  return request<API.ApiResponseAssetUpgradePreflightResponse_>(
    `/api/v1/asset-versions/${param0}/upgrade-preflight`,
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

/** List Archived Shots GET /api/v1/episodes/${param0}/archived-shots */
export async function listArchivedShotsApiV1EpisodesEpisodeIdArchivedShotsGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listArchivedShotsApiV1EpisodesEpisodeIdArchivedShotsGetParams,
  options?: RequestOptions
) {
  const { episode_id: param0, ...queryParams } = params;
  return request<API.ApiResponseListShotResponse_>(
    `/api/v1/episodes/${param0}/archived-shots`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Get Episode Readiness GET /api/v1/episodes/${param0}/shot-readiness */
export async function getEpisodeReadinessApiV1EpisodesEpisodeIdShotReadinessGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getEpisodeReadinessApiV1EpisodesEpisodeIdShotReadinessGetParams,
  options?: RequestOptions
) {
  const { episode_id: param0, ...queryParams } = params;
  return request<API.ApiResponseShotReadinessBatchResponse_>(
    `/api/v1/episodes/${param0}/shot-readiness`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** List Shots GET /api/v1/episodes/${param0}/shots */
export async function listShotsApiV1EpisodesEpisodeIdShotsGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listShotsApiV1EpisodesEpisodeIdShotsGetParams,
  options?: RequestOptions
) {
  const { episode_id: param0, ...queryParams } = params;
  return request<API.ApiResponseShotOrderResponse_>(
    `/api/v1/episodes/${param0}/shots`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Create Manual Shot POST /api/v1/episodes/${param0}/shots */
export async function createManualShotApiV1EpisodesEpisodeIdShotsPost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.createManualShotApiV1EpisodesEpisodeIdShotsPostParams,
  body: API.ShotCreateRequest,
  options?: RequestOptions
) {
  const { episode_id: param0, ...queryParams } = params;
  return request<API.ApiResponseShotResponse_>(
    `/api/v1/episodes/${param0}/shots`,
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

/** Reorder Shots POST /api/v1/episodes/${param0}/shots/reorder */
export async function reorderShotsApiV1EpisodesEpisodeIdShotsReorderPost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.reorderShotsApiV1EpisodesEpisodeIdShotsReorderPostParams,
  body: API.ShotReorderRequest,
  options?: RequestOptions
) {
  const { episode_id: param0, ...queryParams } = params;
  return request<API.ApiResponseShotOrderResponse_>(
    `/api/v1/episodes/${param0}/shots/reorder`,
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

/** Create From Confirmed Candidate POST /api/v1/extraction-candidates/${param0}/shot */
export async function createFromConfirmedCandidateApiV1ExtractionCandidatesCandidateIdShotPost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.createFromConfirmedCandidateApiV1ExtractionCandidatesCandidateIdShotPostParams,
  options?: RequestOptions
) {
  const { candidate_id: param0, ...queryParams } = params;
  return request<API.ApiResponseShotResponse_>(
    `/api/v1/extraction-candidates/${param0}/shot`,
    {
      method: "POST",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Get Spec Version GET /api/v1/shot-spec-versions/${param0} */
export async function getSpecVersionApiV1ShotSpecVersionsVersionIdGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getSpecVersionApiV1ShotSpecVersionsVersionIdGetParams,
  options?: RequestOptions
) {
  const { version_id: param0, ...queryParams } = params;
  return request<API.ApiResponseShotSpecVersionResponse_>(
    `/api/v1/shot-spec-versions/${param0}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Get Shot GET /api/v1/shots/${param0} */
export async function getShotApiV1ShotsShotIdGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getShotApiV1ShotsShotIdGetParams,
  options?: RequestOptions
) {
  const { shot_id: param0, ...queryParams } = params;
  return request<API.ApiResponseShotResponse_>(`/api/v1/shots/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** Delete Shot DELETE /api/v1/shots/${param0} */
export async function deleteShotApiV1ShotsShotIdDelete(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.deleteShotApiV1ShotsShotIdDeleteParams,
  options?: RequestOptions
) {
  const { shot_id: param0, ...queryParams } = params;
  return request<API.ApiResponseShotDeleteResponse_>(
    `/api/v1/shots/${param0}`,
    {
      method: "DELETE",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    }
  );
}

/** Update Shot PATCH /api/v1/shots/${param0} */
export async function updateShotApiV1ShotsShotIdPatch(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.updateShotApiV1ShotsShotIdPatchParams,
  body: API.ShotUpdateRequest,
  options?: RequestOptions
) {
  const { shot_id: param0, ...queryParams } = params;
  return request<API.ApiResponseShotResponse_>(`/api/v1/shots/${param0}`, {
    method: "PATCH",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** Archive Shot POST /api/v1/shots/${param0}/archive */
export async function archiveShotApiV1ShotsShotIdArchivePost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.archiveShotApiV1ShotsShotIdArchivePostParams,
  body: API.ShotStateRequest,
  options?: RequestOptions
) {
  const { shot_id: param0, ...queryParams } = params;
  return request<API.ApiResponseShotStateResponse_>(
    `/api/v1/shots/${param0}/archive`,
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

/** Copy Shot POST /api/v1/shots/${param0}/copy */
export async function copyShotApiV1ShotsShotIdCopyPost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.copyShotApiV1ShotsShotIdCopyPostParams,
  body: API.CopyShotRequest,
  options?: RequestOptions
) {
  const { shot_id: param0, ...queryParams } = params;
  return request<API.ApiResponseShotTransformResponse_>(
    `/api/v1/shots/${param0}/copy`,
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

/** Set Current Spec Version POST /api/v1/shots/${param0}/current-spec-version */
export async function setCurrentSpecVersionApiV1ShotsShotIdCurrentSpecVersionPost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.setCurrentSpecVersionApiV1ShotsShotIdCurrentSpecVersionPostParams,
  body: API.ShotCurrentSpecRequest,
  options?: RequestOptions
) {
  const { shot_id: param0, ...queryParams } = params;
  return request<API.ApiResponseShotResponse_>(
    `/api/v1/shots/${param0}/current-spec-version`,
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

/** Shot Delete Preflight GET /api/v1/shots/${param0}/delete-preflight */
export async function shotDeletePreflightApiV1ShotsShotIdDeletePreflightGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.shotDeletePreflightApiV1ShotsShotIdDeletePreflightGetParams,
  options?: RequestOptions
) {
  const { shot_id: param0, ...queryParams } = params;
  return request<API.ApiResponseShotDeletePreflightResponse_>(
    `/api/v1/shots/${param0}/delete-preflight`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Get Readiness GET /api/v1/shots/${param0}/readiness */
export async function getReadinessApiV1ShotsShotIdReadinessGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getReadinessApiV1ShotsShotIdReadinessGetParams,
  options?: RequestOptions
) {
  const { shot_id: param0, ...queryParams } = params;
  return request<API.ApiResponseShotReadinessResponse_>(
    `/api/v1/shots/${param0}/readiness`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    }
  );
}

/** Restore Shot POST /api/v1/shots/${param0}/restore */
export async function restoreShotApiV1ShotsShotIdRestorePost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.restoreShotApiV1ShotsShotIdRestorePostParams,
  body: API.ShotStateRequest,
  options?: RequestOptions
) {
  const { shot_id: param0, ...queryParams } = params;
  return request<API.ApiResponseShotStateResponse_>(
    `/api/v1/shots/${param0}/restore`,
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

/** List Spec Versions GET /api/v1/shots/${param0}/spec-versions */
export async function listSpecVersionsApiV1ShotsShotIdSpecVersionsGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listSpecVersionsApiV1ShotsShotIdSpecVersionsGetParams,
  options?: RequestOptions
) {
  const { shot_id: param0, ...queryParams } = params;
  return request<API.ApiResponseListShotSpecVersionResponse_>(
    `/api/v1/shots/${param0}/spec-versions`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Append Spec Version POST /api/v1/shots/${param0}/spec-versions */
export async function appendSpecVersionApiV1ShotsShotIdSpecVersionsPost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.appendSpecVersionApiV1ShotsShotIdSpecVersionsPostParams,
  body: API.ShotSpecCreateRequest,
  options?: RequestOptions
) {
  const { shot_id: param0, ...queryParams } = params;
  return request<API.ApiResponseShotSpecCreateResponse_>(
    `/api/v1/shots/${param0}/spec-versions`,
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

/** Split Shot POST /api/v1/shots/${param0}/split */
export async function splitShotApiV1ShotsShotIdSplitPost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.splitShotApiV1ShotsShotIdSplitPostParams,
  body: API.SplitShotRequest,
  options?: RequestOptions
) {
  const { shot_id: param0, ...queryParams } = params;
  return request<API.ApiResponseShotTransformResponse_>(
    `/api/v1/shots/${param0}/split`,
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

/** Split Preflight POST /api/v1/shots/${param0}/split-preflight */
export async function splitPreflightApiV1ShotsShotIdSplitPreflightPost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.splitPreflightApiV1ShotsShotIdSplitPreflightPostParams,
  body: API.SplitPreflightRequest,
  options?: RequestOptions
) {
  const { shot_id: param0, ...queryParams } = params;
  return request<API.ApiResponseShotTransformPreflightResponse_>(
    `/api/v1/shots/${param0}/split-preflight`,
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

/** Merge Shots POST /api/v1/shots/merge */
export async function mergeShotsApiV1ShotsMergePost(
  body: API.MergeShotRequest,
  options?: RequestOptions
) {
  return request<API.ApiResponseShotTransformResponse_>("/api/v1/shots/merge", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** Merge Preflight POST /api/v1/shots/merge-preflight */
export async function mergePreflightApiV1ShotsMergePreflightPost(
  body: API.MergePreflightRequest,
  options?: RequestOptions
) {
  return request<API.ApiResponseShotTransformPreflightResponse_>(
    "/api/v1/shots/merge-preflight",
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
