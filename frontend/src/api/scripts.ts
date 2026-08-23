import request, { type RequestOptions } from "@/lib/request";

/** Set Current Version POST /api/v1/episodes/{episode_id}/current-script-version */
export async function setCurrentVersionApiV1EpisodesEpisodeIdCurrentScriptVersionPost(
  params: API.setCurrentVersionApiV1EpisodesEpisodeIdCurrentScriptVersionPostParams,
  body: API.CurrentScriptVersionRequest,
  options?: RequestOptions,
) {
  const { episode_id: path0 } = params;
  return request<API.ApiResponseCurrentScriptVersionResponse_>(`/api/v1/episodes/${path0}/current-script-version`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Get Narrative Dependency GET /api/v1/episodes/{episode_id}/narrative-dependency */
export async function getNarrativeDependencyApiV1EpisodesEpisodeIdNarrativeDependencyGet(
  params: API.getNarrativeDependencyApiV1EpisodesEpisodeIdNarrativeDependencyGetParams,
  options?: RequestOptions,
) {
  const { episode_id: path0, ...queryParams } = params;
  return request<API.ApiResponseNarrativeDependencyResponse_>(`/api/v1/episodes/${path0}/narrative-dependency`, {
    method: "GET",
    params: queryParams,
    ...(options ?? {}),
  });
}

/** Get Latest Narrative Impact GET /api/v1/episodes/{episode_id}/narrative-impacts/latest */
export async function getLatestNarrativeImpactApiV1EpisodesEpisodeIdNarrativeImpactsLatestGet(
  params: API.getLatestNarrativeImpactApiV1EpisodesEpisodeIdNarrativeImpactsLatestGetParams,
  options?: RequestOptions,
) {
  const { episode_id: path0 } = params;
  return request<API.ApiResponseNarrativeImpactResponse_>(`/api/v1/episodes/${path0}/narrative-impacts/latest`, {
    method: "GET",
    ...(options ?? {}),
  });
}

/** List Sources GET /api/v1/episodes/{episode_id}/script-sources */
export async function listSourcesApiV1EpisodesEpisodeIdScriptSourcesGet(
  params: API.listSourcesApiV1EpisodesEpisodeIdScriptSourcesGetParams,
  options?: RequestOptions,
) {
  const { episode_id: path0, ...queryParams } = params;
  return request<API.ApiResponsePaginatedScriptSources_>(`/api/v1/episodes/${path0}/script-sources`, {
    method: "GET",
    params: queryParams,
    ...(options ?? {}),
  });
}

/** Import Text Source POST /api/v1/episodes/{episode_id}/script-sources */
export async function importTextSourceApiV1EpisodesEpisodeIdScriptSourcesPost(
  params: API.importTextSourceApiV1EpisodesEpisodeIdScriptSourcesPostParams,
  body: API.ScriptImportRequest,
  options?: RequestOptions,
) {
  const { episode_id: path0 } = params;
  return request<API.ApiResponseScriptImportResponse_>(`/api/v1/episodes/${path0}/script-sources`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Get Extraction Batch GET /api/v1/extraction-batches/{batch_id} */
export async function getExtractionBatchApiV1ExtractionBatchesBatchIdGet(
  params: API.getExtractionBatchApiV1ExtractionBatchesBatchIdGetParams,
  options?: RequestOptions,
) {
  const { batch_id: path0 } = params;
  return request<API.ApiResponseExtractionBatchResponse_>(`/api/v1/extraction-batches/${path0}`, {
    method: "GET",
    ...(options ?? {}),
  });
}

/** List Extraction Candidates GET /api/v1/extraction-batches/{batch_id}/candidates */
export async function listExtractionCandidatesApiV1ExtractionBatchesBatchIdCandidatesGet(
  params: API.listExtractionCandidatesApiV1ExtractionBatchesBatchIdCandidatesGetParams,
  options?: RequestOptions,
) {
  const { batch_id: path0, ...queryParams } = params;
  return request<API.ApiResponsePaginatedExtractionCandidates_>(`/api/v1/extraction-batches/${path0}/candidates`, {
    method: "GET",
    params: queryParams,
    ...(options ?? {}),
  });
}

/** Confirm Structure POST /api/v1/extraction-batches/{batch_id}/confirm-structure */
export async function confirmStructureApiV1ExtractionBatchesBatchIdConfirmStructurePost(
  params: API.confirmStructureApiV1ExtractionBatchesBatchIdConfirmStructurePostParams,
  options?: RequestOptions,
) {
  const { batch_id: path0 } = params;
  return request<API.ApiResponseStructureConfirmationResponse_>(`/api/v1/extraction-batches/${path0}/confirm-structure`, {
    method: "POST",
    ...(options ?? {}),
  });
}

/** Get Extraction Candidate GET /api/v1/extraction-candidates/{candidate_id} */
export async function getExtractionCandidateApiV1ExtractionCandidatesCandidateIdGet(
  params: API.getExtractionCandidateApiV1ExtractionCandidatesCandidateIdGetParams,
  options?: RequestOptions,
) {
  const { candidate_id: path0 } = params;
  return request<API.ApiResponseExtractionCandidateResponse_>(`/api/v1/extraction-candidates/${path0}`, {
    method: "GET",
    ...(options ?? {}),
  });
}

/** List Candidate Decisions GET /api/v1/extraction-candidates/{candidate_id}/decisions */
export async function listCandidateDecisionsApiV1ExtractionCandidatesCandidateIdDecisionsGet(
  params: API.listCandidateDecisionsApiV1ExtractionCandidatesCandidateIdDecisionsGetParams,
  options?: RequestOptions,
) {
  const { candidate_id: path0, ...queryParams } = params;
  return request<API.ApiResponsePaginatedCandidateDecisions_>(`/api/v1/extraction-candidates/${path0}/decisions`, {
    method: "GET",
    params: queryParams,
    ...(options ?? {}),
  });
}

/** Decide Extraction Candidate POST /api/v1/extraction-candidates/{candidate_id}/decisions */
export async function decideExtractionCandidateApiV1ExtractionCandidatesCandidateIdDecisionsPost(
  params: API.decideExtractionCandidateApiV1ExtractionCandidatesCandidateIdDecisionsPostParams,
  body: API.CandidateDecisionRequest,
  options?: RequestOptions,
) {
  const { candidate_id: path0 } = params;
  return request<API.ApiResponseCandidateDecisionResultResponse_>(`/api/v1/extraction-candidates/${path0}/decisions`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Revise Narrative Structure POST /api/v1/narrative-structures/{structure_id}/revisions */
export async function reviseNarrativeStructureApiV1NarrativeStructuresStructureIdRevisionsPost(
  params: API.reviseNarrativeStructureApiV1NarrativeStructuresStructureIdRevisionsPostParams,
  body: API.NarrativeStructureRevisionRequest,
  options?: RequestOptions,
) {
  const { structure_id: path0 } = params;
  return request<API.ApiResponseNarrativeRevisionResponse_>(`/api/v1/narrative-structures/${path0}/revisions`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Get Source GET /api/v1/script-sources/{source_id} */
export async function getSourceApiV1ScriptSourcesSourceIdGet(
  params: API.getSourceApiV1ScriptSourcesSourceIdGetParams,
  options?: RequestOptions,
) {
  const { source_id: path0 } = params;
  return request<API.ApiResponseScriptSourceResponse_>(`/api/v1/script-sources/${path0}`, {
    method: "GET",
    ...(options ?? {}),
  });
}

/** Archive Source POST /api/v1/script-sources/{source_id}/archive */
export async function archiveSourceApiV1ScriptSourcesSourceIdArchivePost(
  params: API.archiveSourceApiV1ScriptSourcesSourceIdArchivePostParams,
  body: API.ScriptSourceStateRequest,
  options?: RequestOptions,
) {
  const { source_id: path0 } = params;
  return request<API.ApiResponseScriptSourceResponse_>(`/api/v1/script-sources/${path0}/archive`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Restore Source POST /api/v1/script-sources/{source_id}/restore */
export async function restoreSourceApiV1ScriptSourcesSourceIdRestorePost(
  params: API.restoreSourceApiV1ScriptSourcesSourceIdRestorePostParams,
  body: API.ScriptSourceStateRequest,
  options?: RequestOptions,
) {
  const { source_id: path0 } = params;
  return request<API.ApiResponseScriptSourceResponse_>(`/api/v1/script-sources/${path0}/restore`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** List Versions GET /api/v1/script-sources/{source_id}/versions */
export async function listVersionsApiV1ScriptSourcesSourceIdVersionsGet(
  params: API.listVersionsApiV1ScriptSourcesSourceIdVersionsGetParams,
  options?: RequestOptions,
) {
  const { source_id: path0, ...queryParams } = params;
  return request<API.ApiResponsePaginatedScriptVersions_>(`/api/v1/script-sources/${path0}/versions`, {
    method: "GET",
    params: queryParams,
    ...(options ?? {}),
  });
}

/** Publish Version POST /api/v1/script-sources/{source_id}/versions */
export async function publishVersionApiV1ScriptSourcesSourceIdVersionsPost(
  params: API.publishVersionApiV1ScriptSourcesSourceIdVersionsPostParams,
  body: API.ScriptVersionPublishRequest,
  options?: RequestOptions,
) {
  const { source_id: path0 } = params;
  return request<API.ApiResponseScriptVersionPublishResponse_>(`/api/v1/script-sources/${path0}/versions`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Delete Draft Version DELETE /api/v1/script-versions/{version_id} */
export async function deleteDraftVersionApiV1ScriptVersionsVersionIdDelete(
  params: API.deleteDraftVersionApiV1ScriptVersionsVersionIdDeleteParams,
  options?: RequestOptions,
) {
  const { version_id: path0, ...queryParams } = params;
  return request<API.ApiResponseScriptVersionDeleteResponse_>(`/api/v1/script-versions/${path0}`, {
    method: "DELETE",
    params: queryParams,
    ...(options ?? {}),
  });
}

/** Get Version GET /api/v1/script-versions/{version_id} */
export async function getVersionApiV1ScriptVersionsVersionIdGet(
  params: API.getVersionApiV1ScriptVersionsVersionIdGetParams,
  options?: RequestOptions,
) {
  const { version_id: path0 } = params;
  return request<API.ApiResponseScriptVersionResponse_>(`/api/v1/script-versions/${path0}`, {
    method: "GET",
    ...(options ?? {}),
  });
}

/** Diff Versions GET /api/v1/script-versions/{version_id}/diff */
export async function diffVersionsApiV1ScriptVersionsVersionIdDiffGet(
  params: API.diffVersionsApiV1ScriptVersionsVersionIdDiffGetParams,
  options?: RequestOptions,
) {
  const { version_id: path0, ...queryParams } = params;
  return request<API.ApiResponseScriptVersionDiffResponse_>(`/api/v1/script-versions/${path0}/diff`, {
    method: "GET",
    params: queryParams,
    ...(options ?? {}),
  });
}

/** Start Extraction POST /api/v1/script-versions/{version_id}/extractions */
export async function startExtractionApiV1ScriptVersionsVersionIdExtractionsPost(
  params: API.startExtractionApiV1ScriptVersionsVersionIdExtractionsPostParams,
  body: API.ScriptExtractionRequest,
  options?: RequestOptions,
) {
  const { version_id: path0 } = params;
  return request<API.ApiResponseExtractionBatchResponse_>(`/api/v1/script-versions/${path0}/extractions`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Get Narrative Structure GET /api/v1/script-versions/{version_id}/narrative-structure */
export async function getNarrativeStructureApiV1ScriptVersionsVersionIdNarrativeStructureGet(
  params: API.getNarrativeStructureApiV1ScriptVersionsVersionIdNarrativeStructureGetParams,
  options?: RequestOptions,
) {
  const { version_id: path0, ...queryParams } = params;
  return request<API.ApiResponseNarrativeStructureResponse_>(`/api/v1/script-versions/${path0}/narrative-structure`, {
    method: "GET",
    params: queryParams,
    ...(options ?? {}),
  });
}

/** Get Confirmed Structure GET /api/v1/script-versions/{version_id}/structure */
export async function getConfirmedStructureApiV1ScriptVersionsVersionIdStructureGet(
  params: API.getConfirmedStructureApiV1ScriptVersionsVersionIdStructureGetParams,
  options?: RequestOptions,
) {
  const { version_id: path0 } = params;
  return request<API.ApiResponseConfirmedStructureResponse_>(`/api/v1/script-versions/${path0}/structure`, {
    method: "GET",
    ...(options ?? {}),
  });
}
