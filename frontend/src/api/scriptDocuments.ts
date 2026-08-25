import request, { type RequestOptions } from "@/lib/request";

type Envelope<T> = { data: T };

export function previewDocumentApiV1ProjectsProjectIdScriptImportPreviewsPost(
  params: { project_id: string },
  body: API.ScriptDocumentPreviewRequest,
  options?: RequestOptions,
) {
  return request<Envelope<API.ScriptDocumentPreviewResponse>>(
    `/api/v1/projects/${params.project_id}/script-import-previews`,
    { method: "POST", data: body, ...(options ?? {}) },
  );
}

export function importDocumentApiV1ProjectsProjectIdScriptImportsPost(
  params: { project_id: string },
  body: API.ScriptDocumentImportRequest,
  options?: RequestOptions,
) {
  return request<Envelope<API.ScriptDocumentAnalysisResponse>>(
    `/api/v1/projects/${params.project_id}/script-imports`,
    { method: "POST", data: body, ...(options ?? {}) },
  );
}
