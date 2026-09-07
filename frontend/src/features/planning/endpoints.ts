import {
  getEpisodePlanApiEpisodePlansPlanIdGet,
  createEpisodePlanApiDocumentRevisionsRevisionIdEpisodePlansPost,
  confirmEpisodePlanApiEpisodePlansPlanIdConfirmPost,
  materializeEpisodePlanApiEpisodePlansPlanIdMaterializationsPost,
  publishImportCommitApiImportCommitsCommitIdPublishPost,
} from "@/api/episodePlanning";
import { appApi, runRequest } from "@/lib/server-state";

export const planningApi = appApi.injectEndpoints({
  endpoints: (builder) => ({
    episodePlan: builder.query<API.EpisodePlanDetailResponse, string>({
      queryFn: (planId) =>
        runRequest(() =>
          getEpisodePlanApiEpisodePlansPlanIdGet({ plan_id: planId }),
        ),
      providesTags: (_result, _error, planId) => [
        { type: "EpisodePlans", id: planId },
      ],
    }),
    createEpisodePlan: builder.mutation<
      API.EpisodePlanDetailResponse,
      { revisionId: string; body: API.EpisodePlanCreateRequest }
    >({
      queryFn: ({ revisionId, body }) =>
        runRequest(() =>
          createEpisodePlanApiDocumentRevisionsRevisionIdEpisodePlansPost(
            { revision_id: revisionId },
            body,
          ),
        ),
      invalidatesTags: ["EpisodePlans"],
    }),
    confirmEpisodePlan: builder.mutation<
      API.EpisodePlanDetailResponse,
      { planId: string; body: API.ConfirmEpisodePlanRequest }
    >({
      queryFn: ({ planId, body }) =>
        runRequest(() =>
          confirmEpisodePlanApiEpisodePlansPlanIdConfirmPost(
            { plan_id: planId },
            body,
          ),
        ),
      invalidatesTags: (_result, _error, { planId }) => [
        { type: "EpisodePlans", id: planId },
      ],
    }),
    materializeEpisodePlan: builder.mutation<
      API.ImportCommitDetailResponse,
      { planId: string; body: API.MaterializeEpisodePlanRequest }
    >({
      queryFn: ({ planId, body }) =>
        runRequest(() =>
          materializeEpisodePlanApiEpisodePlansPlanIdMaterializationsPost(
            { plan_id: planId },
            body,
          ),
        ),
      invalidatesTags: ["EpisodePlans", "Episodes", "Project"],
    }),
    publishImportCommit: builder.mutation<
      API.ImportCommitDetailResponse,
      { commitId: string; body: API.PublishImportCommitRequest }
    >({
      queryFn: ({ commitId, body }) =>
        runRequest(() =>
          publishImportCommitApiImportCommitsCommitIdPublishPost(
            { commit_id: commitId },
            body,
          ),
        ),
      invalidatesTags: ["EpisodePlans", "Episodes", "Project"],
    }),
  }),
});

export const {
  useConfirmEpisodePlanMutation,
  useCreateEpisodePlanMutation,
  useLazyEpisodePlanQuery,
  useMaterializeEpisodePlanMutation,
  usePublishImportCommitMutation,
} = planningApi;
