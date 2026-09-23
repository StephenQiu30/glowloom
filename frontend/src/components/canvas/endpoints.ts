import { applyProjectCanvasOperations, getProjectCanvas } from "@/api/generated/backend";
import { appApi, runRequest } from "@/lib/server-state";

export const canvasApi = appApi.injectEndpoints({
  endpoints: (builder) => ({
    canvasDocument: builder.query<GeneratedAPI.CanvasDocument, string>({
      queryFn: (projectId) => runRequest(() => getProjectCanvas({ project_id: projectId })),
      providesTags: (_result, _error, projectId) => [{ type: "Canvas", id: projectId }],
    }),
    applyCanvasOperations: builder.mutation<
      GeneratedAPI.CanvasApplyResponse,
      { projectId: string; body: GeneratedAPI.CanvasOperationRequest }
    >({
      queryFn: ({ projectId, body }) =>
        runRequest(() => applyProjectCanvasOperations({ project_id: projectId }, body)),
      async onQueryStarted({ projectId }, { dispatch, queryFulfilled }) {
        try {
          const { data } = await queryFulfilled;
          dispatch(canvasApi.util.upsertQueryData("canvasDocument", projectId, data.document));
        } catch {
          // The caller displays the error; the existing query remains authoritative.
        }
      },
      invalidatesTags: (_result, _error, { projectId }) => [{ type: "Canvas", id: projectId }],
    }),
  }),
});

export const { useCanvasDocumentQuery, useApplyCanvasOperationsMutation } = canvasApi;
