import request, { type RequestOptions } from "@/lib/request";

/** Create Episode Plan POST /api/v1/document-revisions/{revision_id}/episode-plans */
export async function createEpisodePlanApiV1DocumentRevisionsRevisionIdEpisodePlansPost(
  params: API.createEpisodePlanApiV1DocumentRevisionsRevisionIdEpisodePlansPostParams,
  body: API.EpisodePlanCreateRequest,
  options?: RequestOptions,
) {
  const { revision_id: path0 } = params;
  return request<API.ApiResponseEpisodePlanDetailResponse_>(`/api/v1/document-revisions/${path0}/episode-plans`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Get Episode Plan GET /api/v1/episode-plans/{plan_id} */
export async function getEpisodePlanApiV1EpisodePlansPlanIdGet(
  params: API.getEpisodePlanApiV1EpisodePlansPlanIdGetParams,
  options?: RequestOptions,
) {
  const { plan_id: path0 } = params;
  return request<API.ApiResponseEpisodePlanDetailResponse_>(`/api/v1/episode-plans/${path0}`, {
    method: "GET",
    ...(options ?? {}),
  });
}

/** Confirm Episode Plan POST /api/v1/episode-plans/{plan_id}/confirm */
export async function confirmEpisodePlanApiV1EpisodePlansPlanIdConfirmPost(
  params: API.confirmEpisodePlanApiV1EpisodePlansPlanIdConfirmPostParams,
  body: API.ConfirmEpisodePlanRequest,
  options?: RequestOptions,
) {
  const { plan_id: path0 } = params;
  return request<API.ApiResponseEpisodePlanDetailResponse_>(`/api/v1/episode-plans/${path0}/confirm`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Materialize Episode Plan POST /api/v1/episode-plans/{plan_id}/materializations */
export async function materializeEpisodePlanApiV1EpisodePlansPlanIdMaterializationsPost(
  params: API.materializeEpisodePlanApiV1EpisodePlansPlanIdMaterializationsPostParams,
  body: API.MaterializeEpisodePlanRequest,
  options?: RequestOptions,
) {
  const { plan_id: path0 } = params;
  return request<API.ApiResponseImportCommitDetailResponse_>(`/api/v1/episode-plans/${path0}/materializations`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Merge Episode Proposals POST /api/v1/episode-plans/{plan_id}/merge */
export async function mergeEpisodeProposalsApiV1EpisodePlansPlanIdMergePost(
  params: API.mergeEpisodeProposalsApiV1EpisodePlansPlanIdMergePostParams,
  body: API.MergeEpisodeProposalRequest,
  options?: RequestOptions,
) {
  const { plan_id: path0 } = params;
  return request<API.ApiResponseEpisodePlanDetailResponse_>(`/api/v1/episode-plans/${path0}/merge`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Move Episode Boundary POST /api/v1/episode-plans/{plan_id}/move-boundary */
export async function moveEpisodeBoundaryApiV1EpisodePlansPlanIdMoveBoundaryPost(
  params: API.moveEpisodeBoundaryApiV1EpisodePlansPlanIdMoveBoundaryPostParams,
  body: API.MoveEpisodeBoundaryRequest,
  options?: RequestOptions,
) {
  const { plan_id: path0 } = params;
  return request<API.ApiResponseEpisodePlanDetailResponse_>(`/api/v1/episode-plans/${path0}/move-boundary`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Rename Episode Proposal POST /api/v1/episode-plans/{plan_id}/rename */
export async function renameEpisodeProposalApiV1EpisodePlansPlanIdRenamePost(
  params: API.renameEpisodeProposalApiV1EpisodePlansPlanIdRenamePostParams,
  body: API.RenameEpisodeProposalRequest,
  options?: RequestOptions,
) {
  const { plan_id: path0 } = params;
  return request<API.ApiResponseEpisodePlanDetailResponse_>(`/api/v1/episode-plans/${path0}/rename`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Split Episode Proposal POST /api/v1/episode-plans/{plan_id}/split */
export async function splitEpisodeProposalApiV1EpisodePlansPlanIdSplitPost(
  params: API.splitEpisodeProposalApiV1EpisodePlansPlanIdSplitPostParams,
  body: API.SplitEpisodeProposalRequest,
  options?: RequestOptions,
) {
  const { plan_id: path0 } = params;
  return request<API.ApiResponseEpisodePlanDetailResponse_>(`/api/v1/episode-plans/${path0}/split`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Publish Import Commit POST /api/v1/import-commits/{commit_id}/publish */
export async function publishImportCommitApiV1ImportCommitsCommitIdPublishPost(
  params: API.publishImportCommitApiV1ImportCommitsCommitIdPublishPostParams,
  body: API.PublishImportCommitRequest,
  options?: RequestOptions,
) {
  const { commit_id: path0 } = params;
  return request<API.ApiResponseImportCommitDetailResponse_>(`/api/v1/import-commits/${path0}/publish`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}
