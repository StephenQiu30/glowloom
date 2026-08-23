// @ts-ignore
/* eslint-disable */
import request, { type RequestOptions } from "@/lib/request";

/** Get Revision GET /api/v1/document-revisions/${param0} */
export async function getRevisionApiV1DocumentRevisionsRevisionIdGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getRevisionApiV1DocumentRevisionsRevisionIdGetParams,
  options?: RequestOptions
) {
  const { revision_id: param0, ...queryParams } = params;
  return request<API.ApiResponseScriptDocumentAnalysisResponse_>(
    `/api/v1/document-revisions/${param0}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** List Documents GET /api/v1/projects/${param0}/script-documents */
export async function listDocumentsApiV1ProjectsProjectIdScriptDocumentsGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listDocumentsApiV1ProjectsProjectIdScriptDocumentsGetParams,
  options?: RequestOptions
) {
  const { project_id: param0, ...queryParams } = params;
  return request<API.ApiResponsePaginatedScriptDocuments_>(
    `/api/v1/projects/${param0}/script-documents`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    }
  );
}

/** Preview Document POST /api/v1/projects/${param0}/script-import-previews */
export async function previewDocumentApiV1ProjectsProjectIdScriptImportPreviewsPost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.previewDocumentApiV1ProjectsProjectIdScriptImportPreviewsPostParams,
  body: API.ScriptDocumentPreviewRequest,
  options?: RequestOptions
) {
  const { project_id: param0, ...queryParams } = params;
  return request<API.ApiResponseScriptDocumentPreviewResponse_>(
    `/api/v1/projects/${param0}/script-import-previews`,
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

/** Import Document POST /api/v1/projects/${param0}/script-imports */
export async function importDocumentApiV1ProjectsProjectIdScriptImportsPost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.importDocumentApiV1ProjectsProjectIdScriptImportsPostParams,
  body: API.ScriptDocumentImportRequest,
  options?: RequestOptions
) {
  const { project_id: param0, ...queryParams } = params;
  return request<API.ApiResponseScriptDocumentAnalysisResponse_>(
    `/api/v1/projects/${param0}/script-imports`,
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
