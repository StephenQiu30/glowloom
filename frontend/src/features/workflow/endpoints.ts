import {
  getWorkflowRunApiWorkflowRunsWorkflowRunIdGet,
} from "@/api/workflows";
import { appApi, runRequest } from "@/lib/server-state";

export const workflowApi = appApi.injectEndpoints({
  endpoints: (builder) => ({
    workflowRun: builder.query<API.WorkflowRunViewResponse, string>({
      queryFn: (workflowRunId) =>
        runRequest(() =>
          getWorkflowRunApiWorkflowRunsWorkflowRunIdGet({
            workflow_run_id: workflowRunId,
          }),
        ),
      providesTags: (_result, _error, workflowRunId) => [
        { type: "WorkflowRuns", id: workflowRunId },
      ],
    }),
  }),
});

export const {
  useWorkflowRunQuery,
} = workflowApi;
