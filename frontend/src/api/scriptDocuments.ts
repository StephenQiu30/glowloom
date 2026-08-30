import request, { type RequestOptions } from "@/lib/request";

type Envelope<T> = { data: T };

export function getCurrentDocumentApiProjectsProjectIdCurrentScriptDocumentGet(
  params: { project_id: string },
  options?: RequestOptions,
) {
  return request<Envelope<API.ScriptDocumentAnalysisResponse>>(
    `/api/projects/${params.project_id}/current-script-document`,
    { method: "GET", ...(options ?? {}) },
  );
}

export function previewDocumentApiProjectsProjectIdScriptImportPreviewsPost(
  params: { project_id: string },
  body: API.ScriptDocumentPreviewRequest,
  options?: RequestOptions,
) {
  return request<Envelope<API.ScriptDocumentPreviewResponse>>(
    `/api/projects/${params.project_id}/script-import-previews`,
    { method: "POST", data: body, ...(options ?? {}) },
  );
}

export function importDocumentApiProjectsProjectIdScriptImportsPost(
  params: { project_id: string },
  body: API.ScriptDocumentImportRequest,
  options?: RequestOptions,
) {
  return request<Envelope<API.ScriptDocumentAnalysisResponse>>(
    `/api/projects/${params.project_id}/script-imports`,
    { method: "POST", data: body, ...(options ?? {}) },
  );
}
