import {
  acceptScriptSource, adoptCreationProposal, createCreationRun, getCreationExecution, getCreationRun,
  getCurrentScriptSource, listCreationProposals, listCreationRuns, retryCreationDelivery,
  syncCreationProposals, resumeCreationRun,
} from "@/api/creation";
import { ApiClientError } from "@/lib/request";
import { appApi, runRequest } from "@/lib/server-state";

export const creationApi = appApi.injectEndpoints({
  endpoints: (builder) => ({
    resumeCreation: builder.mutation<API.CreationResumeResponse, { runId: string; revision: number }>({
      queryFn: ({ runId, revision }) => runRequest(() => resumeCreationRun(runId, { expected_revision: revision })),
      invalidatesTags: (_data, _error, { runId }) => [{ type: "CreationRuns", id: runId }, { type: "CreationRuns", id: `proposals:${runId}` }],
    }),
    creationRun: builder.query<API.CreationRunResponse, string>({
      queryFn: (id) => runRequest(() => getCreationRun(id)),
      providesTags: (_data, _error, id) => [{ type: "CreationRuns", id }],
    }),
    creationRuns: builder.query<API.CreationRunResponse[], string>({
      queryFn: (id) => runRequest(() => listCreationRuns(id)),
      providesTags: (_data, _error, id) => [{ type: "CreationRuns", id: `project:${id}` }],
    }),
    creationExecution: builder.query<API.CreationExecution, string>({
      queryFn: (id) => runRequest(() => getCreationExecution(id)),
      providesTags: (_data, _error, id) => [{ type: "CreationRuns", id }],
    }),
    creationProposals: builder.query<API.CreationProposal[], string>({
      queryFn: (id) => runRequest(() => listCreationProposals(id)),
      providesTags: (_data, _error, id) => [{ type: "CreationRuns", id: `proposals:${id}` }],
    }),
    currentScriptSource: builder.query<API.AcceptedScriptSourceResponse | null, string>({
      queryFn: (id) => runRequest(async () => {
        try { return await getCurrentScriptSource(id); }
        catch (error) {
          if (error instanceof ApiClientError && error.code === "not_found") return { data: null };
          throw error;
        }
      }),
      providesTags: (_data, _error, id) => [{ type: "ScriptDocuments", id: `source:${id}` }],
    }),
    acceptCreationSource: builder.mutation<API.AcceptedScriptSourceResponse, { projectId: string; body: API.AcceptScriptSourceRequest }>({
      queryFn: ({ projectId, body }) => runRequest(() => acceptScriptSource(projectId, body)),
      invalidatesTags: (_data, _error, { projectId }) => [{ type: "ScriptDocuments", id: `source:${projectId}` }],
    }),
    startCreation: builder.mutation<API.CreationRunResponse, { projectId: string; body: API.CreateCreationRunRequest }>({
      queryFn: ({ projectId, body }) => runRequest(() => createCreationRun(projectId, body)),
      invalidatesTags: (_data, _error, { projectId }) => [{ type: "CreationRuns", id: `project:${projectId}` }],
    }),
    retryCreationDelivery: builder.mutation<API.CreationRunResponse, { projectId: string; runId: string; revision: number }>({
      queryFn: ({ runId, revision }) => runRequest(() => retryCreationDelivery(runId, { expected_revision: revision })),
      invalidatesTags: (_data, _error, { projectId, runId }) => [{ type: "CreationRuns", id: `project:${projectId}` }, { type: "CreationRuns", id: runId }],
    }),
    syncCreation: builder.mutation<API.CreationSyncResult, string>({
      queryFn: (id) => runRequest(() => syncCreationProposals(id)),
      invalidatesTags: (_data, _error, id) => [{ type: "CreationRuns", id }, { type: "CreationRuns", id: `proposals:${id}` }, "HumanTasks"],
    }),
    adoptCreation: builder.mutation<API.CreationAdoptionReceipt, { projectId: string; runId: string; proposalId: string; body: API.AdoptCreationProposalRequest }>({
      queryFn: ({ runId, proposalId, body }) => runRequest(() => adoptCreationProposal(runId, proposalId, body)),
      invalidatesTags: (_data, _error, { projectId, runId }) => [{ type: "CreationRuns", id: runId }, { type: "CreationRuns", id: `proposals:${runId}` }, { type: "Episodes", id: projectId }, "HumanTasks"],
    }),
  }),
});

export const {
  useCreationRunQuery, useCreationRunsQuery, useCreationExecutionQuery, useCreationProposalsQuery,
  useCurrentScriptSourceQuery, useAcceptCreationSourceMutation, useStartCreationMutation,
  useRetryCreationDeliveryMutation, useSyncCreationMutation, useAdoptCreationMutation, useResumeCreationMutation,
} = creationApi;
