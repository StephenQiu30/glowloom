// @ts-ignore
/* eslint-disable */
import request, { type RequestOptions } from "@/lib/request";

/** Create Episode Plan POST /api/v1/document-revisions/${param0}/episode-plans */
export async function createEpisodePlanApiV1DocumentRevisionsRevisionIdEpisodePlansPost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.createEpisodePlanApiV1DocumentRevisionsRevisionIdEpisodePlansPostParams,
  body: API.EpisodePlanCreateRequest,
  options?: RequestOptions
) {
  const { revision_id: param0, ...queryParams } = params;
  return request<API.ApiResponseEpisodePlanDetailResponse_>(
    `/api/v1/document-revisions/${param0}/episode-plans`,
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

/** Get Episode Plan GET /api/v1/episode-plans/${param0} */
export async function getEpisodePlanApiV1EpisodePlansPlanIdGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getEpisodePlanApiV1EpisodePlansPlanIdGetParams,
  options?: RequestOptions
) {
  const { plan_id: param0, ...queryParams } = params;
  return request<API.ApiResponseEpisodePlanDetailResponse_>(
    `/api/v1/episode-plans/${param0}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Confirm Episode Plan POST /api/v1/episode-plans/${param0}/confirm */
export async function confirmEpisodePlanApiV1EpisodePlansPlanIdConfirmPost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.confirmEpisodePlanApiV1EpisodePlansPlanIdConfirmPostParams,
  body: API.ConfirmEpisodePlanRequest,
  options?: RequestOptions
) {
  const { plan_id: param0, ...queryParams } = params;
  return request<API.ApiResponseEpisodePlanDetailResponse_>(
    `/api/v1/episode-plans/${param0}/confirm`,
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

/** Materialize Episode Plan POST /api/v1/episode-plans/${param0}/materializations */
export async function materializeEpisodePlanApiV1EpisodePlansPlanIdMaterializationsPost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.materializeEpisodePlanApiV1EpisodePlansPlanIdMaterializationsPostParams,
  body: API.MaterializeEpisodePlanRequest,
  options?: RequestOptions
) {
  const { plan_id: param0, ...queryParams } = params;
  return request<API.ApiResponseImportCommitDetailResponse_>(
    `/api/v1/episode-plans/${param0}/materializations`,
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

/** Merge Episode Proposals POST /api/v1/episode-plans/${param0}/merge */
export async function mergeEpisodeProposalsApiV1EpisodePlansPlanIdMergePost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.mergeEpisodeProposalsApiV1EpisodePlansPlanIdMergePostParams,
  body: API.MergeEpisodeProposalRequest,
  options?: RequestOptions
) {
  const { plan_id: param0, ...queryParams } = params;
  return request<API.ApiResponseEpisodePlanDetailResponse_>(
    `/api/v1/episode-plans/${param0}/merge`,
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

/** Move Episode Boundary POST /api/v1/episode-plans/${param0}/move-boundary */
export async function moveEpisodeBoundaryApiV1EpisodePlansPlanIdMoveBoundaryPost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.moveEpisodeBoundaryApiV1EpisodePlansPlanIdMoveBoundaryPostParams,
  body: API.MoveEpisodeBoundaryRequest,
  options?: RequestOptions
) {
  const { plan_id: param0, ...queryParams } = params;
  return request<API.ApiResponseEpisodePlanDetailResponse_>(
    `/api/v1/episode-plans/${param0}/move-boundary`,
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

/** Rename Episode Proposal POST /api/v1/episode-plans/${param0}/rename */
export async function renameEpisodeProposalApiV1EpisodePlansPlanIdRenamePost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.renameEpisodeProposalApiV1EpisodePlansPlanIdRenamePostParams,
  body: API.RenameEpisodeProposalRequest,
  options?: RequestOptions
) {
  const { plan_id: param0, ...queryParams } = params;
  return request<API.ApiResponseEpisodePlanDetailResponse_>(
    `/api/v1/episode-plans/${param0}/rename`,
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

/** Split Episode Proposal POST /api/v1/episode-plans/${param0}/split */
export async function splitEpisodeProposalApiV1EpisodePlansPlanIdSplitPost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.splitEpisodeProposalApiV1EpisodePlansPlanIdSplitPostParams,
  body: API.SplitEpisodeProposalRequest,
  options?: RequestOptions
) {
  const { plan_id: param0, ...queryParams } = params;
  return request<API.ApiResponseEpisodePlanDetailResponse_>(
    `/api/v1/episode-plans/${param0}/split`,
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

/** Publish Import Commit POST /api/v1/import-commits/${param0}/publish */
export async function publishImportCommitApiV1ImportCommitsCommitIdPublishPost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.publishImportCommitApiV1ImportCommitsCommitIdPublishPostParams,
  body: API.PublishImportCommitRequest,
  options?: RequestOptions
) {
  const { commit_id: param0, ...queryParams } = params;
  return request<API.ApiResponseImportCommitDetailResponse_>(
    `/api/v1/import-commits/${param0}/publish`,
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
