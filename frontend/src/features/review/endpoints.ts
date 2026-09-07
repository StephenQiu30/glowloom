import {
  listHumanTasksApiProjectsProjectIdHumanTasksGet,
  getHumanTaskApiHumanTasksHumanTaskIdGet,
  claimHumanTaskApiHumanTasksHumanTaskIdClaimsPost,
  renewHumanTaskClaimApiHumanTasksHumanTaskIdClaimRenewalsPost,
  releaseHumanTaskClaimApiHumanTasksHumanTaskIdClaimReleasesPost,
  decideHumanTaskApiHumanTasksHumanTaskIdDecisionsPost,
  resumeHumanGateApiReviewDecisionsReviewDecisionIdResumePost,
} from "@/api/humanReviews";
import { appApi, runRequest } from "@/lib/server-state";

export const reviewApi = appApi.injectEndpoints({
  endpoints: (builder) => ({
    humanTasks: builder.query<
      API.HumanTaskListEnvelope["data"],
      {
        projectId: string;
        status: "active" | API.HumanTaskBaseResponse["status"];
        subjectType?: string;
        after?: string;
      }
    >({
      queryFn: ({ projectId, status, subjectType, after }) =>
        runRequest(() =>
          listHumanTasksApiProjectsProjectIdHumanTasksGet({
            project_id: projectId,
            status,
            subject_type: subjectType ?? null,
            limit: 50,
            after: after ?? null,
          }),
        ),
      providesTags: (result, _error, { projectId }) => [
        { type: "HumanTasks", id: `project:${projectId}` },
        ...(result?.items.map((task) => ({
          type: "HumanTasks" as const,
          id: task.id,
        })) ?? []),
      ],
    }),
    humanTask: builder.query<API.HumanTaskDetailEnvelope["data"], string>({
      queryFn: (taskId) =>
        runRequest(() =>
          getHumanTaskApiHumanTasksHumanTaskIdGet({ human_task_id: taskId }),
        ),
      providesTags: (_result, _error, taskId) => [
        { type: "HumanTasks", id: taskId },
      ],
    }),
    claimHumanTask: builder.mutation<
      API.HumanTaskCommandEnvelope["data"],
      { projectId: string; taskId: string; body: API.HumanTaskClaimRequest }
    >({
      queryFn: ({ taskId, body }) =>
        runRequest(() =>
          claimHumanTaskApiHumanTasksHumanTaskIdClaimsPost(
            { human_task_id: taskId },
            body,
          ),
        ),
      invalidatesTags: (_result, _error, { projectId, taskId }) => [
        { type: "HumanTasks", id: taskId },
        { type: "HumanTasks", id: `project:${projectId}` },
      ],
    }),
    renewHumanTaskClaim: builder.mutation<
      API.HumanTaskCommandEnvelope["data"],
      { projectId: string; taskId: string; body: API.HumanTaskClaimTokenRequest }
    >({
      queryFn: ({ taskId, body }) =>
        runRequest(() =>
          renewHumanTaskClaimApiHumanTasksHumanTaskIdClaimRenewalsPost(
            { human_task_id: taskId },
            body,
          ),
        ),
      invalidatesTags: (_result, _error, { projectId, taskId }) => [
        { type: "HumanTasks", id: taskId },
        { type: "HumanTasks", id: `project:${projectId}` },
      ],
    }),
    releaseHumanTaskClaim: builder.mutation<
      API.HumanTaskCommandEnvelope["data"],
      { projectId: string; taskId: string; body: API.HumanTaskClaimTokenRequest }
    >({
      queryFn: ({ taskId, body }) =>
        runRequest(() =>
          releaseHumanTaskClaimApiHumanTasksHumanTaskIdClaimReleasesPost(
            { human_task_id: taskId },
            body,
          ),
        ),
      invalidatesTags: (_result, _error, { projectId, taskId }) => [
        { type: "HumanTasks", id: taskId },
        { type: "HumanTasks", id: `project:${projectId}` },
      ],
    }),
    decideHumanTask: builder.mutation<
      API.HumanGateDecisionEnvelope["data"],
      {
        projectId: string;
        taskId: string;
        workflowRunId: string;
        body: API.HumanTaskDecisionRequest;
      }
    >({
      queryFn: ({ taskId, body }) =>
        runRequest(() =>
          decideHumanTaskApiHumanTasksHumanTaskIdDecisionsPost(
            { human_task_id: taskId },
            body,
          ),
        ),
      invalidatesTags: (_result, _error, { projectId, taskId, workflowRunId }) => [
        { type: "HumanTasks", id: taskId },
        { type: "HumanTasks", id: `project:${projectId}` },
        { type: "WorkflowRuns", id: workflowRunId },
      ],
    }),
    resumeHumanGate: builder.mutation<
      API.HumanGateResumeEnvelope["data"],
      {
        projectId: string;
        taskId: string;
        decisionId: string;
        workflowRunId: string;
      }
    >({
      queryFn: ({ decisionId }) =>
        runRequest(() =>
          resumeHumanGateApiReviewDecisionsReviewDecisionIdResumePost({
            review_decision_id: decisionId,
          }),
        ),
      invalidatesTags: (_result, _error, { projectId, taskId, workflowRunId }) => [
        { type: "HumanTasks", id: taskId },
        { type: "HumanTasks", id: `project:${projectId}` },
        { type: "WorkflowRuns", id: workflowRunId },
      ],
    }),
  }),
});

export const {
  useClaimHumanTaskMutation,
  useDecideHumanTaskMutation,
  useHumanTaskQuery,
  useHumanTasksQuery,
  useReleaseHumanTaskClaimMutation,
  useResumeHumanGateMutation,
  useRenewHumanTaskClaimMutation,
} = reviewApi;
