// @ts-ignore
/* eslint-disable */
import request, { type RequestOptions } from "@/lib/request";

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

/** Get Narrative Dependency GET /api/v1/episodes/${param0}/narrative-dependency */
export async function getNarrativeDependencyApiV1EpisodesEpisodeIdNarrativeDependencyGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getNarrativeDependencyApiV1EpisodesEpisodeIdNarrativeDependencyGetParams,
  options?: RequestOptions
) {
  const { episode_id: param0, ...queryParams } = params;
  return request<API.ApiResponseNarrativeDependencyResponse_>(
    `/api/v1/episodes/${param0}/narrative-dependency`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    }
  );
}

/** Get Latest Narrative Impact GET /api/v1/episodes/${param0}/narrative-impacts/latest */
export async function getLatestNarrativeImpactApiV1EpisodesEpisodeIdNarrativeImpactsLatestGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getLatestNarrativeImpactApiV1EpisodesEpisodeIdNarrativeImpactsLatestGetParams,
  options?: RequestOptions
) {
  const { episode_id: param0, ...queryParams } = params;
  return request<API.ApiResponseNarrativeImpactResponse_>(
    `/api/v1/episodes/${param0}/narrative-impacts/latest`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** List Sources GET /api/v1/episodes/${param0}/script-sources */
export async function listSourcesApiV1EpisodesEpisodeIdScriptSourcesGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listSourcesApiV1EpisodesEpisodeIdScriptSourcesGetParams,
  options?: RequestOptions
) {
  const { episode_id: param0, ...queryParams } = params;
  return request<API.ApiResponsePaginatedScriptSources_>(
    `/api/v1/episodes/${param0}/script-sources`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
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

/** Get Extraction Batch GET /api/v1/extraction-batches/${param0} */
export async function getExtractionBatchApiV1ExtractionBatchesBatchIdGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getExtractionBatchApiV1ExtractionBatchesBatchIdGetParams,
  options?: RequestOptions
) {
  const { batch_id: param0, ...queryParams } = params;
  return request<API.ApiResponseExtractionBatchResponse_>(
    `/api/v1/extraction-batches/${param0}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** List Extraction Candidates GET /api/v1/extraction-batches/${param0}/candidates */
export async function listExtractionCandidatesApiV1ExtractionBatchesBatchIdCandidatesGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listExtractionCandidatesApiV1ExtractionBatchesBatchIdCandidatesGetParams,
  options?: RequestOptions
) {
  const { batch_id: param0, ...queryParams } = params;
  return request<API.ApiResponsePaginatedExtractionCandidates_>(
    `/api/v1/extraction-batches/${param0}/candidates`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    }
  );
}

/** Confirm Structure POST /api/v1/extraction-batches/${param0}/confirm-structure */
export async function confirmStructureApiV1ExtractionBatchesBatchIdConfirmStructurePost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.confirmStructureApiV1ExtractionBatchesBatchIdConfirmStructurePostParams,
  options?: RequestOptions
) {
  const { batch_id: param0, ...queryParams } = params;
  return request<API.ApiResponseStructureConfirmationResponse_>(
    `/api/v1/extraction-batches/${param0}/confirm-structure`,
    {
      method: "POST",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Get Extraction Candidate GET /api/v1/extraction-candidates/${param0} */
export async function getExtractionCandidateApiV1ExtractionCandidatesCandidateIdGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getExtractionCandidateApiV1ExtractionCandidatesCandidateIdGetParams,
  options?: RequestOptions
) {
  const { candidate_id: param0, ...queryParams } = params;
  return request<API.ApiResponseExtractionCandidateResponse_>(
    `/api/v1/extraction-candidates/${param0}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** List Candidate Decisions GET /api/v1/extraction-candidates/${param0}/decisions */
export async function listCandidateDecisionsApiV1ExtractionCandidatesCandidateIdDecisionsGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listCandidateDecisionsApiV1ExtractionCandidatesCandidateIdDecisionsGetParams,
  options?: RequestOptions
) {
  const { candidate_id: param0, ...queryParams } = params;
  return request<API.ApiResponsePaginatedCandidateDecisions_>(
    `/api/v1/extraction-candidates/${param0}/decisions`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    }
  );
}

/** Decide Extraction Candidate POST /api/v1/extraction-candidates/${param0}/decisions */
export async function decideExtractionCandidateApiV1ExtractionCandidatesCandidateIdDecisionsPost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.decideExtractionCandidateApiV1ExtractionCandidatesCandidateIdDecisionsPostParams,
  body: API.CandidateDecisionRequest,
  options?: RequestOptions
) {
  const { candidate_id: param0, ...queryParams } = params;
  return request<API.ApiResponseCandidateDecisionResultResponse_>(
    `/api/v1/extraction-candidates/${param0}/decisions`,
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

/** Revise Narrative Structure POST /api/v1/narrative-structures/${param0}/revisions */
export async function reviseNarrativeStructureApiV1NarrativeStructuresStructureIdRevisionsPost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.reviseNarrativeStructureApiV1NarrativeStructuresStructureIdRevisionsPostParams,
  body: API.NarrativeStructureRevisionRequest,
  options?: RequestOptions
) {
  const { structure_id: param0, ...queryParams } = params;
  return request<API.ApiResponseNarrativeRevisionResponse_>(
    `/api/v1/narrative-structures/${param0}/revisions`,
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

/** Delete Draft Version DELETE /api/v1/script-versions/${param0} */
export async function deleteDraftVersionApiV1ScriptVersionsVersionIdDelete(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.deleteDraftVersionApiV1ScriptVersionsVersionIdDeleteParams,
  options?: RequestOptions
) {
  const { version_id: param0, ...queryParams } = params;
  return request<API.ApiResponseScriptVersionDeleteResponse_>(
    `/api/v1/script-versions/${param0}`,
    {
      method: "DELETE",
      params: {
        ...queryParams,
      },
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

/** Start Extraction POST /api/v1/script-versions/${param0}/extractions */
export async function startExtractionApiV1ScriptVersionsVersionIdExtractionsPost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.startExtractionApiV1ScriptVersionsVersionIdExtractionsPostParams,
  body: API.ScriptExtractionRequest,
  options?: RequestOptions
) {
  const { version_id: param0, ...queryParams } = params;
  return request<API.ApiResponseExtractionBatchResponse_>(
    `/api/v1/script-versions/${param0}/extractions`,
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

/** Get Narrative Structure GET /api/v1/script-versions/${param0}/narrative-structure */
export async function getNarrativeStructureApiV1ScriptVersionsVersionIdNarrativeStructureGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getNarrativeStructureApiV1ScriptVersionsVersionIdNarrativeStructureGetParams,
  options?: RequestOptions
) {
  const { version_id: param0, ...queryParams } = params;
  return request<API.ApiResponseNarrativeStructureResponse_>(
    `/api/v1/script-versions/${param0}/narrative-structure`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    }
  );
}

/** Get Confirmed Structure GET /api/v1/script-versions/${param0}/structure */
export async function getConfirmedStructureApiV1ScriptVersionsVersionIdStructureGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getConfirmedStructureApiV1ScriptVersionsVersionIdStructureGetParams,
  options?: RequestOptions
) {
  const { version_id: param0, ...queryParams } = params;
  return request<API.ApiResponseConfirmedStructureResponse_>(
    `/api/v1/script-versions/${param0}/structure`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}
