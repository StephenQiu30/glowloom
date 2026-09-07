import {
  getCurrentBibleApiProjectsProjectIdProductionBibleGet,
  getBibleApiProductionBiblesBibleIdGet,
  createBibleApiDocumentRevisionsRevisionIdProductionBiblesPost,
  decideProductionBibleReviewIssue,
  resumeBibleApiProductionBiblesBibleIdResumePost,
} from "@/api/productionBibles";
import { appApi, runRequest } from "@/lib/server-state";

export const productionBibleApi = appApi.injectEndpoints({
  endpoints: (builder) => ({
    currentProductionBible: builder.query<API.ProductionBibleResponse, string>({
      queryFn: (projectId) =>
        runRequest(() =>
          getCurrentBibleApiProjectsProjectIdProductionBibleGet({
            project_id: projectId,
          }),
        ),
      providesTags: (_result, _error, projectId) => [
        { type: "ProductionBible", id: projectId },
      ],
    }),
    productionBible: builder.query<API.ProductionBibleResponse, string>({
      queryFn: (bibleId) =>
        runRequest(() =>
          getBibleApiProductionBiblesBibleIdGet({
            bible_id: bibleId,
          }),
        ),
      providesTags: (_result, _error, bibleId) => [
        { type: "ProductionBible", id: bibleId },
      ],
    }),
    createProductionBible: builder.mutation<
      API.ProductionBibleResponse,
      { projectId: string; revisionId: string; body: API.ProductionBibleCreateRequest }
    >({
      queryFn: ({ revisionId, body }) =>
        runRequest(() =>
          createBibleApiDocumentRevisionsRevisionIdProductionBiblesPost(
            { revision_id: revisionId },
            body,
          ),
        ),
      invalidatesTags: (_result, _error, { projectId }) => [
        { type: "ProductionBible", id: projectId },
      ],
    }),
    decideProductionBibleReviewIssue: builder.mutation<
      API.ProductionBibleResponse,
      {
        projectId: string;
        bibleId: string;
        body: API.ProductionBibleReviewDecisionRequest;
      }
    >({
      queryFn: ({ bibleId, body }) =>
        runRequest(() =>
          decideProductionBibleReviewIssue(
            { bible_id: bibleId },
            body,
          ),
        ),
      invalidatesTags: (_result, _error, { projectId, bibleId }) => [
        { type: "ProductionBible", id: projectId },
        { type: "ProductionBible", id: bibleId },
      ],
    }),
    resumeProductionBible: builder.mutation<
      API.ProductionBibleResponse,
      { projectId: string; bibleId: string; body: API.ProductionBibleResumeRequest }
    >({
      queryFn: ({ bibleId, body }) =>
        runRequest(() =>
          resumeBibleApiProductionBiblesBibleIdResumePost(
            { bible_id: bibleId },
            body,
          ),
        ),
      invalidatesTags: (_result, _error, { projectId, bibleId }) => [
        { type: "ProductionBible", id: projectId },
        { type: "ProductionBible", id: bibleId },
      ],
    }),
  }),
});

export const {
  useCreateProductionBibleMutation,
  useCurrentProductionBibleQuery,
  useDecideProductionBibleReviewIssueMutation,
  useProductionBibleQuery,
  useResumeProductionBibleMutation,
} = productionBibleApi;
