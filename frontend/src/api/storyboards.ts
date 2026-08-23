import request, { type RequestOptions } from "@/lib/request";

/** List Asset Shot Usages GET /api/v1/asset-versions/{asset_version_id}/shot-usages */
export async function listAssetShotUsagesApiV1AssetVersionsAssetVersionIdShotUsagesGet(
  params: API.listAssetShotUsagesApiV1AssetVersionsAssetVersionIdShotUsagesGetParams,
  options?: RequestOptions,
) {
  const { asset_version_id: path0, ...queryParams } = params;
  return request<API.ApiResponsePaginatedAssetShotUsages_>(`/api/v1/asset-versions/${path0}/shot-usages`, {
    method: "GET",
    params: queryParams,
    ...(options ?? {}),
  });
}

/** Apply Asset Upgrade POST /api/v1/asset-versions/{asset_version_id}/upgrade */
export async function applyAssetUpgradeApiV1AssetVersionsAssetVersionIdUpgradePost(
  params: API.applyAssetUpgradeApiV1AssetVersionsAssetVersionIdUpgradePostParams,
  body: API.AssetUpgradeApplyRequest,
  options?: RequestOptions,
) {
  const { asset_version_id: path0 } = params;
  return request<API.ApiResponseAssetUpgradeApplyResponse_>(`/api/v1/asset-versions/${path0}/upgrade`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Preflight Asset Upgrade POST /api/v1/asset-versions/{asset_version_id}/upgrade-preflight */
export async function preflightAssetUpgradeApiV1AssetVersionsAssetVersionIdUpgradePreflightPost(
  params: API.preflightAssetUpgradeApiV1AssetVersionsAssetVersionIdUpgradePreflightPostParams,
  body: API.AssetUpgradePreflightRequest,
  options?: RequestOptions,
) {
  const { asset_version_id: path0 } = params;
  return request<API.ApiResponseAssetUpgradePreflightResponse_>(`/api/v1/asset-versions/${path0}/upgrade-preflight`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** List Archived Shots GET /api/v1/episodes/{episode_id}/archived-shots */
export async function listArchivedShotsApiV1EpisodesEpisodeIdArchivedShotsGet(
  params: API.listArchivedShotsApiV1EpisodesEpisodeIdArchivedShotsGetParams,
  options?: RequestOptions,
) {
  const { episode_id: path0 } = params;
  return request<API.ApiResponseListShotResponse_>(`/api/v1/episodes/${path0}/archived-shots`, {
    method: "GET",
    ...(options ?? {}),
  });
}

/** Get Coverage GET /api/v1/episodes/{episode_id}/coverage */
export async function getCoverageApiV1EpisodesEpisodeIdCoverageGet(
  params: API.getCoverageApiV1EpisodesEpisodeIdCoverageGetParams,
  options?: RequestOptions,
) {
  const { episode_id: path0 } = params;
  return request<API.ApiResponseCoverageReportResponse_>(`/api/v1/episodes/${path0}/coverage`, {
    method: "GET",
    ...(options ?? {}),
  });
}

/** Decide Coverage POST /api/v1/episodes/{episode_id}/coverage-decisions */
export async function decideCoverageApiV1EpisodesEpisodeIdCoverageDecisionsPost(
  params: API.decideCoverageApiV1EpisodesEpisodeIdCoverageDecisionsPostParams,
  body: API.CoverageDecisionRequest,
  options?: RequestOptions,
) {
  const { episode_id: path0 } = params;
  return request<API.ApiResponseCoverageDecisionApplyResponse_>(`/api/v1/episodes/${path0}/coverage-decisions`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Get Episode Readiness GET /api/v1/episodes/{episode_id}/shot-readiness */
export async function getEpisodeReadinessApiV1EpisodesEpisodeIdShotReadinessGet(
  params: API.getEpisodeReadinessApiV1EpisodesEpisodeIdShotReadinessGetParams,
  options?: RequestOptions,
) {
  const { episode_id: path0 } = params;
  return request<API.ApiResponseShotReadinessBatchResponse_>(`/api/v1/episodes/${path0}/shot-readiness`, {
    method: "GET",
    ...(options ?? {}),
  });
}

/** List Shots GET /api/v1/episodes/{episode_id}/shots */
export async function listShotsApiV1EpisodesEpisodeIdShotsGet(
  params: API.listShotsApiV1EpisodesEpisodeIdShotsGetParams,
  options?: RequestOptions,
) {
  const { episode_id: path0 } = params;
  return request<API.ApiResponseShotOrderResponse_>(`/api/v1/episodes/${path0}/shots`, {
    method: "GET",
    ...(options ?? {}),
  });
}

/** Create Manual Shot POST /api/v1/episodes/{episode_id}/shots */
export async function createManualShotApiV1EpisodesEpisodeIdShotsPost(
  params: API.createManualShotApiV1EpisodesEpisodeIdShotsPostParams,
  body: API.ShotCreateRequest,
  options?: RequestOptions,
) {
  const { episode_id: path0 } = params;
  return request<API.ApiResponseShotResponse_>(`/api/v1/episodes/${path0}/shots`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Reorder Shots POST /api/v1/episodes/{episode_id}/shots/reorder */
export async function reorderShotsApiV1EpisodesEpisodeIdShotsReorderPost(
  params: API.reorderShotsApiV1EpisodesEpisodeIdShotsReorderPostParams,
  body: API.ShotReorderRequest,
  options?: RequestOptions,
) {
  const { episode_id: path0 } = params;
  return request<API.ApiResponseShotOrderResponse_>(`/api/v1/episodes/${path0}/shots/reorder`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Create Batch POST /api/v1/episodes/{episode_id}/storyboard-draft-batches */
export async function createBatchApiV1EpisodesEpisodeIdStoryboardDraftBatchesPost(
  params: API.createBatchApiV1EpisodesEpisodeIdStoryboardDraftBatchesPostParams,
  body: API.DraftBatchCreateRequest,
  options?: RequestOptions,
) {
  const { episode_id: path0 } = params;
  return request<API.ApiResponseDraftBatchResponse_>(`/api/v1/episodes/${path0}/storyboard-draft-batches`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** List Exports GET /api/v1/episodes/{episode_id}/storyboard-exports */
export async function listExportsApiV1EpisodesEpisodeIdStoryboardExportsGet(
  params: API.listExportsApiV1EpisodesEpisodeIdStoryboardExportsGetParams,
  options?: RequestOptions,
) {
  const { episode_id: path0 } = params;
  return request<API.ApiResponseExportHistoryResponse_>(`/api/v1/episodes/${path0}/storyboard-exports`, {
    method: "GET",
    ...(options ?? {}),
  });
}

/** Request Export POST /api/v1/episodes/{episode_id}/storyboard-exports */
export async function requestExportApiV1EpisodesEpisodeIdStoryboardExportsPost(
  params: API.requestExportApiV1EpisodesEpisodeIdStoryboardExportsPostParams,
  body: API.ExportRequest,
  options?: RequestOptions,
) {
  const { episode_id: path0 } = params;
  return request<API.ApiResponseExportResponse_>(`/api/v1/episodes/${path0}/storyboard-exports`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Preflight Export POST /api/v1/episodes/{episode_id}/storyboard-exports/preflight */
export async function preflightExportApiV1EpisodesEpisodeIdStoryboardExportsPreflightPost(
  params: API.preflightExportApiV1EpisodesEpisodeIdStoryboardExportsPreflightPostParams,
  options?: RequestOptions,
) {
  const { episode_id: path0 } = params;
  return request<API.ApiResponseExportPreflightResponse_>(`/api/v1/episodes/${path0}/storyboard-exports/preflight`, {
    method: "POST",
    ...(options ?? {}),
  });
}

/** Create From Confirmed Candidate POST /api/v1/extraction-candidates/{candidate_id}/shot */
export async function createFromConfirmedCandidateApiV1ExtractionCandidatesCandidateIdShotPost(
  params: API.createFromConfirmedCandidateApiV1ExtractionCandidatesCandidateIdShotPostParams,
  options?: RequestOptions,
) {
  const { candidate_id: path0 } = params;
  return request<API.ApiResponseShotResponse_>(`/api/v1/extraction-candidates/${path0}/shot`, {
    method: "POST",
    ...(options ?? {}),
  });
}

/** Get Spec Version GET /api/v1/shot-spec-versions/{version_id} */
export async function getSpecVersionApiV1ShotSpecVersionsVersionIdGet(
  params: API.getSpecVersionApiV1ShotSpecVersionsVersionIdGetParams,
  options?: RequestOptions,
) {
  const { version_id: path0 } = params;
  return request<API.ApiResponseShotSpecVersionResponse_>(`/api/v1/shot-spec-versions/${path0}`, {
    method: "GET",
    ...(options ?? {}),
  });
}

/** Delete Shot DELETE /api/v1/shots/{shot_id} */
export async function deleteShotApiV1ShotsShotIdDelete(
  params: API.deleteShotApiV1ShotsShotIdDeleteParams,
  options?: RequestOptions,
) {
  const { shot_id: path0, ...queryParams } = params;
  return request<API.ApiResponseShotDeleteResponse_>(`/api/v1/shots/${path0}`, {
    method: "DELETE",
    params: queryParams,
    ...(options ?? {}),
  });
}

/** Get Shot GET /api/v1/shots/{shot_id} */
export async function getShotApiV1ShotsShotIdGet(
  params: API.getShotApiV1ShotsShotIdGetParams,
  options?: RequestOptions,
) {
  const { shot_id: path0 } = params;
  return request<API.ApiResponseShotResponse_>(`/api/v1/shots/${path0}`, {
    method: "GET",
    ...(options ?? {}),
  });
}

/** Update Shot PATCH /api/v1/shots/{shot_id} */
export async function updateShotApiV1ShotsShotIdPatch(
  params: API.updateShotApiV1ShotsShotIdPatchParams,
  body: API.ShotUpdateRequest,
  options?: RequestOptions,
) {
  const { shot_id: path0 } = params;
  return request<API.ApiResponseShotResponse_>(`/api/v1/shots/${path0}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Archive Shot POST /api/v1/shots/{shot_id}/archive */
export async function archiveShotApiV1ShotsShotIdArchivePost(
  params: API.archiveShotApiV1ShotsShotIdArchivePostParams,
  body: API.ShotStateRequest,
  options?: RequestOptions,
) {
  const { shot_id: path0 } = params;
  return request<API.ApiResponseShotStateResponse_>(`/api/v1/shots/${path0}/archive`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Copy Shot POST /api/v1/shots/{shot_id}/copy */
export async function copyShotApiV1ShotsShotIdCopyPost(
  params: API.copyShotApiV1ShotsShotIdCopyPostParams,
  body: API.CopyShotRequest,
  options?: RequestOptions,
) {
  const { shot_id: path0 } = params;
  return request<API.ApiResponseShotTransformResponse_>(`/api/v1/shots/${path0}/copy`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Set Current Spec Version POST /api/v1/shots/{shot_id}/current-spec-version */
export async function setCurrentSpecVersionApiV1ShotsShotIdCurrentSpecVersionPost(
  params: API.setCurrentSpecVersionApiV1ShotsShotIdCurrentSpecVersionPostParams,
  body: API.ShotCurrentSpecRequest,
  options?: RequestOptions,
) {
  const { shot_id: path0 } = params;
  return request<API.ApiResponseShotResponse_>(`/api/v1/shots/${path0}/current-spec-version`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Shot Delete Preflight GET /api/v1/shots/{shot_id}/delete-preflight */
export async function shotDeletePreflightApiV1ShotsShotIdDeletePreflightGet(
  params: API.shotDeletePreflightApiV1ShotsShotIdDeletePreflightGetParams,
  options?: RequestOptions,
) {
  const { shot_id: path0 } = params;
  return request<API.ApiResponseShotDeletePreflightResponse_>(`/api/v1/shots/${path0}/delete-preflight`, {
    method: "GET",
    ...(options ?? {}),
  });
}

/** Replace References POST /api/v1/shots/{shot_id}/narrative-references */
export async function replaceReferencesApiV1ShotsShotIdNarrativeReferencesPost(
  params: API.replaceReferencesApiV1ShotsShotIdNarrativeReferencesPostParams,
  body: API.NarrativeReferenceReplaceRequest,
  options?: RequestOptions,
) {
  const { shot_id: path0 } = params;
  return request<API.ApiResponseNarrativeReferenceReplaceResponse_>(`/api/v1/shots/${path0}/narrative-references`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Get Readiness GET /api/v1/shots/{shot_id}/readiness */
export async function getReadinessApiV1ShotsShotIdReadinessGet(
  params: API.getReadinessApiV1ShotsShotIdReadinessGetParams,
  options?: RequestOptions,
) {
  const { shot_id: path0, ...queryParams } = params;
  return request<API.ApiResponseShotReadinessResponse_>(`/api/v1/shots/${path0}/readiness`, {
    method: "GET",
    params: queryParams,
    ...(options ?? {}),
  });
}

/** Restore Shot POST /api/v1/shots/{shot_id}/restore */
export async function restoreShotApiV1ShotsShotIdRestorePost(
  params: API.restoreShotApiV1ShotsShotIdRestorePostParams,
  body: API.ShotStateRequest,
  options?: RequestOptions,
) {
  const { shot_id: path0 } = params;
  return request<API.ApiResponseShotStateResponse_>(`/api/v1/shots/${path0}/restore`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** List Spec Versions GET /api/v1/shots/{shot_id}/spec-versions */
export async function listSpecVersionsApiV1ShotsShotIdSpecVersionsGet(
  params: API.listSpecVersionsApiV1ShotsShotIdSpecVersionsGetParams,
  options?: RequestOptions,
) {
  const { shot_id: path0 } = params;
  return request<API.ApiResponseListShotSpecVersionResponse_>(`/api/v1/shots/${path0}/spec-versions`, {
    method: "GET",
    ...(options ?? {}),
  });
}

/** Append Spec Version POST /api/v1/shots/{shot_id}/spec-versions */
export async function appendSpecVersionApiV1ShotsShotIdSpecVersionsPost(
  params: API.appendSpecVersionApiV1ShotsShotIdSpecVersionsPostParams,
  body: API.ShotSpecCreateRequest,
  options?: RequestOptions,
) {
  const { shot_id: path0 } = params;
  return request<API.ApiResponseShotSpecCreateResponse_>(`/api/v1/shots/${path0}/spec-versions`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Split Shot POST /api/v1/shots/{shot_id}/split */
export async function splitShotApiV1ShotsShotIdSplitPost(
  params: API.splitShotApiV1ShotsShotIdSplitPostParams,
  body: API.SplitShotRequest,
  options?: RequestOptions,
) {
  const { shot_id: path0 } = params;
  return request<API.ApiResponseShotTransformResponse_>(`/api/v1/shots/${path0}/split`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Split Preflight POST /api/v1/shots/{shot_id}/split-preflight */
export async function splitPreflightApiV1ShotsShotIdSplitPreflightPost(
  params: API.splitPreflightApiV1ShotsShotIdSplitPreflightPostParams,
  body: API.SplitPreflightRequest,
  options?: RequestOptions,
) {
  const { shot_id: path0 } = params;
  return request<API.ApiResponseShotTransformPreflightResponse_>(`/api/v1/shots/${path0}/split-preflight`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Merge Shots POST /api/v1/shots/merge */
export async function mergeShotsApiV1ShotsMergePost(
  body: API.MergeShotRequest,
  options?: RequestOptions,
) {
  return request<API.ApiResponseShotTransformResponse_>(`/api/v1/shots/merge`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Merge Preflight POST /api/v1/shots/merge-preflight */
export async function mergePreflightApiV1ShotsMergePreflightPost(
  body: API.MergePreflightRequest,
  options?: RequestOptions,
) {
  return request<API.ApiResponseShotTransformPreflightResponse_>(`/api/v1/shots/merge-preflight`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Get Batch GET /api/v1/storyboard-draft-batches/{batch_id} */
export async function getBatchApiV1StoryboardDraftBatchesBatchIdGet(
  params: API.getBatchApiV1StoryboardDraftBatchesBatchIdGetParams,
  options?: RequestOptions,
) {
  const { batch_id: path0 } = params;
  return request<API.ApiResponseDraftBatchResponse_>(`/api/v1/storyboard-draft-batches/${path0}`, {
    method: "GET",
    ...(options ?? {}),
  });
}

/** Apply Batch POST /api/v1/storyboard-draft-batches/{batch_id}/apply */
export async function applyBatchApiV1StoryboardDraftBatchesBatchIdApplyPost(
  params: API.applyBatchApiV1StoryboardDraftBatchesBatchIdApplyPostParams,
  body: API.DraftApplyRequest,
  options?: RequestOptions,
) {
  const { batch_id: path0 } = params;
  return request<API.ApiResponseDraftApplyResponse_>(`/api/v1/storyboard-draft-batches/${path0}/apply`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Preflight Apply POST /api/v1/storyboard-draft-batches/{batch_id}/apply-preflight */
export async function preflightApplyApiV1StoryboardDraftBatchesBatchIdApplyPreflightPost(
  params: API.preflightApplyApiV1StoryboardDraftBatchesBatchIdApplyPreflightPostParams,
  body: API.DraftApplyPreflightRequest,
  options?: RequestOptions,
) {
  const { batch_id: path0 } = params;
  return request<API.ApiResponseDraftApplyPreflightResponse_>(`/api/v1/storyboard-draft-batches/${path0}/apply-preflight`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Approve Batch POST /api/v1/storyboard-draft-batches/{batch_id}/approve */
export async function approveBatchApiV1StoryboardDraftBatchesBatchIdApprovePost(
  params: API.approveBatchApiV1StoryboardDraftBatchesBatchIdApprovePostParams,
  body: API.DraftApproveRequest,
  options?: RequestOptions,
) {
  const { batch_id: path0 } = params;
  return request<API.ApiResponseDraftBatchResponse_>(`/api/v1/storyboard-draft-batches/${path0}/approve`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Decide Draft POST /api/v1/storyboard-drafts/{draft_id}/decisions */
export async function decideDraftApiV1StoryboardDraftsDraftIdDecisionsPost(
  params: API.decideDraftApiV1StoryboardDraftsDraftIdDecisionsPostParams,
  body: API.DraftDecisionRequest,
  options?: RequestOptions,
) {
  const { draft_id: path0 } = params;
  return request<API.ApiResponseDraftDecisionResult_>(`/api/v1/storyboard-drafts/${path0}/decisions`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}
