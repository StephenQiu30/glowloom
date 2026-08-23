import request, { type RequestOptions } from "@/lib/request";

/** Get Revision GET /api/v1/document-revisions/{revision_id} */
export async function getRevisionApiV1DocumentRevisionsRevisionIdGet(
  params: API.getRevisionApiV1DocumentRevisionsRevisionIdGetParams,
  options?: RequestOptions,
) {
  const { revision_id: path0 } = params;
  return request<API.ApiResponseScriptDocumentAnalysisResponse_>(`/api/v1/document-revisions/${path0}`, {
    method: "GET",
    ...(options ?? {}),
  });
}

/** List Documents GET /api/v1/projects/{project_id}/script-documents */
export async function listDocumentsApiV1ProjectsProjectIdScriptDocumentsGet(
  params: API.listDocumentsApiV1ProjectsProjectIdScriptDocumentsGetParams,
  options?: RequestOptions,
) {
  const { project_id: path0, ...queryParams } = params;
  return request<API.ApiResponsePaginatedScriptDocuments_>(`/api/v1/projects/${path0}/script-documents`, {
    method: "GET",
    params: queryParams,
    ...(options ?? {}),
  });
}

/** Preview Document POST /api/v1/projects/{project_id}/script-import-previews */
export async function previewDocumentApiV1ProjectsProjectIdScriptImportPreviewsPost(
  params: API.previewDocumentApiV1ProjectsProjectIdScriptImportPreviewsPostParams,
  body: API.ScriptDocumentPreviewRequest,
  options?: RequestOptions,
) {
  const { project_id: path0 } = params;
  return request<API.ApiResponseScriptDocumentPreviewResponse_>(`/api/v1/projects/${path0}/script-import-previews`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Import Document POST /api/v1/projects/{project_id}/script-imports */
export async function importDocumentApiV1ProjectsProjectIdScriptImportsPost(
  params: API.importDocumentApiV1ProjectsProjectIdScriptImportsPostParams,
  body: API.ScriptDocumentImportRequest,
  options?: RequestOptions,
) {
  const { project_id: path0 } = params;
  return request<API.ApiResponseScriptDocumentAnalysisResponse_>(`/api/v1/projects/${path0}/script-imports`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}
