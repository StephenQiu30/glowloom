import {
  previewDocumentApiProjectsProjectIdScriptImportPreviewsPost,
  getCurrentDocumentApiProjectsProjectIdCurrentScriptDocumentGet,
  importDocumentApiProjectsProjectIdScriptImportsPost,
} from "@/api/scriptDocuments";
import { appApi, runRequest } from "@/lib/server-state";

export const scriptApi = appApi.injectEndpoints({
  endpoints: (builder) => ({
    previewScriptDocument: builder.mutation<
      API.ScriptDocumentPreviewResponse,
      { projectId: string; body: API.ScriptDocumentPreviewRequest }
    >({
      queryFn: ({ projectId, body }) =>
        runRequest(() =>
          previewDocumentApiProjectsProjectIdScriptImportPreviewsPost(
            { project_id: projectId },
            body,
          ),
        ),
    }),
    currentScriptDocument: builder.query<
      API.ScriptDocumentAnalysisResponse,
      string
    >({
      queryFn: (projectId) =>
        runRequest(() =>
          getCurrentDocumentApiProjectsProjectIdCurrentScriptDocumentGet({
            project_id: projectId,
          }),
        ),
      providesTags: (_result, _error, projectId) => [
        { type: "ScriptDocuments", id: projectId },
      ],
    }),
    importScriptDocument: builder.mutation<
      API.ScriptDocumentAnalysisResponse,
      { projectId: string; body: API.ScriptDocumentImportRequest }
    >({
      queryFn: ({ projectId, body }) =>
        runRequest(() =>
          importDocumentApiProjectsProjectIdScriptImportsPost(
            { project_id: projectId },
            body,
          ),
        ),
      invalidatesTags: (_result, _error, { projectId }) => [
        { type: "Project", id: projectId },
        { type: "ScriptDocuments", id: projectId },
      ],
    }),
  }),
});

export const {
  useCurrentScriptDocumentQuery,
  useImportScriptDocumentMutation,
  usePreviewScriptDocumentMutation,
} = scriptApi;
