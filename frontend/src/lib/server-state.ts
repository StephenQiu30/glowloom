import { createApi, fakeBaseQuery } from "@reduxjs/toolkit/query/react";

import {
  confirmEpisodePlanApiEpisodePlansPlanIdConfirmPost,
  createEpisodePlanApiDocumentRevisionsRevisionIdEpisodePlansPost,
  getEpisodePlanApiEpisodePlansPlanIdGet,
  materializeEpisodePlanApiEpisodePlansPlanIdMaterializationsPost,
  publishImportCommitApiImportCommitsCommitIdPublishPost,
} from "@/api/episodePlanning";
import {
  archiveWorkspaceApiWorkspacesWorkspaceIdArchivePost,
  changePasswordApiAuthChangePasswordPost,
  confirmRegistrationVerificationApiAuthRegistrationVerificationsConfirmPost,
  createWorkspaceApiWorkspacesPost,
  deactivateMeApiMeDeactivatePost,
  listWorkspacesApiWorkspacesGet,
  loginApiAuthLoginPost,
  logoutApiAuthLogoutPost,
  meApiMeGet,
  registerApiAuthRegisterPost,
  requestRegistrationVerificationApiAuthRegistrationVerificationsPost,
  restoreWorkspaceApiWorkspacesWorkspaceIdRestorePost,
  updateMeApiMePatch,
  updateWorkspaceApiWorkspacesWorkspaceIdPatch,
} from "@/api/identity";
import {
  completeUploadApiMediaUploadsUploadSessionIdCompletePost,
  getMediaApiMediaVersionIdGet,
  initializeUploadApiMediaUploadsPost,
} from "@/api/media";
import {
  claimHumanTaskApiHumanTasksHumanTaskIdClaimsPost,
  decideHumanTaskApiHumanTasksHumanTaskIdDecisionsPost,
  getHumanTaskApiHumanTasksHumanTaskIdGet,
  listHumanTasksApiProjectsProjectIdHumanTasksGet,
  releaseHumanTaskClaimApiHumanTasksHumanTaskIdClaimReleasesPost,
  renewHumanTaskClaimApiHumanTasksHumanTaskIdClaimRenewalsPost,
  resumeHumanGateApiReviewDecisionsReviewDecisionIdResumePost,
} from "@/api/humanReviews";
import {
  createBibleApiDocumentRevisionsRevisionIdProductionBiblesPost,
  getBibleApiProductionBiblesBibleIdGet,
  getCurrentBibleApiProjectsProjectIdProductionBibleGet,
  resumeBibleApiProductionBiblesBibleIdResumePost,
} from "@/api/productionBibles";
import {
  createProjectApiProjectsPost,
  getProjectApiProjectsProjectIdGet,
  listEpisodesApiProjectsProjectIdEpisodesGet,
  listProjectsApiProjectsGet,
} from "@/api/projects";
import {
  getCurrentDocumentApiProjectsProjectIdCurrentScriptDocumentGet,
  importDocumentApiProjectsProjectIdScriptImportsPost,
  previewDocumentApiProjectsProjectIdScriptImportPreviewsPost,
} from "@/api/scriptDocuments";
import { getWorkflowRunApiWorkflowRunsWorkflowRunIdGet } from "@/api/workflows";
import request, { ApiClientError } from "@/lib/request";

export type AppApiError = {
  message: string;
  code: string;
  nextAction?: string;
  details?: unknown;
};

export type ProductionBibleReviewDecision = "accepted" | "rejected";
export type ProductionBibleWithDecisions = API.ProductionBibleResponse & {
  review_decisions?: Record<string, ProductionBibleReviewDecision>;
};

const errorMessages: Record<string, string> = {
  dependency_unavailable: "注册服务暂时不可用，请稍后重试。",
  invalid_verification_code: "验证码不正确，请检查后重新输入。",
  rate_limited: "验证码发送过于频繁，请等待倒计时结束后重试。",
  resource_conflict: "请求所依据的服务端版本已经变化，请刷新后重试。",
  validation_failed: "提交内容与服务端契约不一致，请刷新后重试。",
  unauthenticated: "邮箱或密码不正确，请重新输入。",
  verification_expired: "验证码或注册凭证已失效，请重新发送验证码。",
};

export function appApiErrorMessage(error: unknown): string {
  const apiError = error as Partial<AppApiError> | undefined;
  return apiError?.code
    ? (errorMessages[apiError.code] ?? apiError.message ?? "服务暂时不可用，请稍后重试。")
    : (apiError?.message ?? "服务暂时不可用，请稍后重试。");
}

async function runRequest<T>(
  operation: () => Promise<{ data: T }>,
): Promise<{ data: T } | { error: AppApiError }> {
  try {
    const response = await operation();
    return { data: response.data };
  } catch (error: unknown) {
    if (error instanceof ApiClientError) {
      return {
        error: {
          message: error.message,
          code: error.code,
          nextAction: error.nextAction,
          details: error.details,
        },
      };
    }
    return {
      error: { message: "服务暂时不可用，请稍后重试。", code: "request_failed" },
    };
  }
}

export const appApi = createApi({
  reducerPath: "appApi",
  baseQuery: fakeBaseQuery<AppApiError>(),
  tagTypes: [
    "Me",
    "Workspaces",
    "Projects",
    "Project",
    "Episodes",
    "Media",
    "ScriptDocuments",
    "ProductionBible",
    "EpisodePlans",
    "HumanTasks",
    "WorkflowRuns",
  ],
  endpoints: (builder) => ({
    login: builder.mutation<API.AuthResponse, API.LoginRequest>({
      queryFn: (body) => runRequest(() => loginApiAuthLoginPost(body)),
    }),
    register: builder.mutation<API.AuthResponse, API.RegisterRequest>({
      queryFn: (body) => runRequest(() => registerApiAuthRegisterPost(body)),
    }),
    requestRegistrationVerification: builder.mutation<
      API.RegistrationVerificationAccepted,
      API.RegistrationVerificationRequest
    >({
      queryFn: (body) =>
        runRequest(() =>
          requestRegistrationVerificationApiAuthRegistrationVerificationsPost(body),
        ),
    }),
    confirmRegistrationVerification: builder.mutation<
      API.RegistrationVerificationConfirmed,
      API.RegistrationVerificationConfirmRequest
    >({
      queryFn: (body) =>
        runRequest(() =>
          confirmRegistrationVerificationApiAuthRegistrationVerificationsConfirmPost(
            body,
          ),
        ),
    }),
    me: builder.query<API.MeResponse, void>({
      queryFn: () => runRequest(() => meApiMeGet()),
      providesTags: ["Me"],
    }),
    logout: builder.mutation<API.RevocationResponse, void>({
      queryFn: () => runRequest(() => logoutApiAuthLogoutPost()),
    }),
    updateProfile: builder.mutation<API.MeResponse, API.ProfileUpdateRequest>({
      queryFn: (body) => runRequest(() => updateMeApiMePatch(body)),
      invalidatesTags: ["Me"],
    }),
    changePassword: builder.mutation<API.RevocationResponse, API.ChangePasswordRequest>({
      queryFn: (body) => runRequest(() => changePasswordApiAuthChangePasswordPost(body)),
    }),
    deactivateAccount: builder.mutation<
      API.RevocationResponse,
      API.DeactivateAccountRequest
    >({
      queryFn: (body) => runRequest(() => deactivateMeApiMeDeactivatePost(body)),
    }),
    workspaces: builder.query<API.WorkspaceResponse[], void>({
      queryFn: () =>
        runRequest(() =>
          listWorkspacesApiWorkspacesGet({ include_archived: true }),
        ),
      providesTags: ["Workspaces"],
    }),
    createWorkspace: builder.mutation<
      API.WorkspaceResponse,
      API.WorkspaceCreateRequest
    >({
      queryFn: (body) => runRequest(() => createWorkspaceApiWorkspacesPost(body)),
      invalidatesTags: ["Workspaces"],
    }),
    updateWorkspace: builder.mutation<
      API.WorkspaceResponse,
      { workspaceId: string; body: API.WorkspaceUpdateRequest }
    >({
      queryFn: ({ workspaceId, body }) =>
        runRequest(() =>
          updateWorkspaceApiWorkspacesWorkspaceIdPatch(
            { workspace_id: workspaceId },
            body,
          ),
        ),
      invalidatesTags: ["Me", "Workspaces"],
    }),
    setWorkspaceArchived: builder.mutation<
      API.WorkspaceResponse,
      { workspaceId: string; expectedRevision: number; archived: boolean }
    >({
      queryFn: ({ workspaceId, expectedRevision, archived }) => {
        const params = { workspace_id: workspaceId };
        const body = { expected_revision: expectedRevision };
        return runRequest(() =>
          archived
            ? archiveWorkspaceApiWorkspacesWorkspaceIdArchivePost(params, body)
            : restoreWorkspaceApiWorkspacesWorkspaceIdRestorePost(params, body),
        );
      },
      invalidatesTags: ["Me", "Workspaces", "Projects"],
    }),
    projects: builder.query<API.PaginatedProjects, string>({
      queryFn: (workspaceId) =>
        runRequest(() =>
          listProjectsApiProjectsGet({
            workspace_id: workspaceId,
            include_archived: true,
            search: null,
            sort: "updated_at",
            order: "desc",
            limit: 50,
            offset: 0,
          }),
        ),
      providesTags: ["Projects"],
    }),
    createProject: builder.mutation<API.ProjectResponse, API.ProjectCreateRequest>({
      queryFn: (body) => runRequest(() => createProjectApiProjectsPost(body)),
      invalidatesTags: ["Projects"],
    }),
    project: builder.query<API.ProjectResponse, string>({
      queryFn: (projectId) =>
        runRequest(() =>
          getProjectApiProjectsProjectIdGet({ project_id: projectId }),
        ),
      providesTags: (_result, _error, projectId) => [
        { type: "Project", id: projectId },
      ],
    }),
    episodes: builder.query<API.EpisodeResponse[], string>({
      queryFn: (projectId) =>
        runRequest(() =>
          listEpisodesApiProjectsProjectIdEpisodesGet({
            project_id: projectId,
            include_archived: true,
          }),
        ),
      providesTags: (_result, _error, projectId) => [
        { type: "Episodes", id: projectId },
      ],
    }),
    initializeMediaUpload: builder.mutation<
      API.UploadInitializationResponse,
      API.UploadDeclaration
    >({
      queryFn: (body) =>
        runRequest(() => initializeUploadApiMediaUploadsPost(body)),
    }),
    completeMediaUpload: builder.mutation<
      API.UploadCompletionResponse,
      { uploadSessionId: string; workspaceId: string }
    >({
      queryFn: ({ uploadSessionId }) =>
        runRequest(() =>
          completeUploadApiMediaUploadsUploadSessionIdCompletePost({
            upload_session_id: uploadSessionId,
          }),
        ),
      invalidatesTags: (_result, _error, { workspaceId }) => [
        { type: "Media", id: workspaceId },
      ],
    }),
    mediaVersion: builder.query<API.MediaVersionResponse, string>({
      queryFn: (versionId) =>
        runRequest(() =>
          getMediaApiMediaVersionIdGet({ version_id: versionId }),
        ),
      providesTags: (_result, _error, versionId) => [
        { type: "Media", id: versionId },
      ],
    }),
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
    currentProductionBible: builder.query<ProductionBibleWithDecisions, string>({
      queryFn: (projectId) =>
        runRequest(() =>
          getCurrentBibleApiProjectsProjectIdProductionBibleGet({
            project_id: projectId,
          }) as Promise<{ data: ProductionBibleWithDecisions }>,
        ),
      providesTags: (_result, _error, projectId) => [
        { type: "ProductionBible", id: projectId },
      ],
    }),
    productionBible: builder.query<ProductionBibleWithDecisions, string>({
      queryFn: (bibleId) =>
        runRequest(() =>
          getBibleApiProductionBiblesBibleIdGet({
            bible_id: bibleId,
          }) as Promise<{ data: ProductionBibleWithDecisions }>,
        ),
      providesTags: (_result, _error, bibleId) => [
        { type: "ProductionBible", id: bibleId },
      ],
    }),
    createProductionBible: builder.mutation<
      ProductionBibleWithDecisions,
      { projectId: string; revisionId: string; body: API.ProductionBibleCreateRequest }
    >({
      queryFn: ({ revisionId, body }) =>
        runRequest(() =>
          createBibleApiDocumentRevisionsRevisionIdProductionBiblesPost(
            { revision_id: revisionId },
            body,
          ) as Promise<{ data: ProductionBibleWithDecisions }>,
        ),
      invalidatesTags: (_result, _error, { projectId }) => [
        { type: "ProductionBible", id: projectId },
      ],
    }),
    decideProductionBibleReviewIssue: builder.mutation<
      ProductionBibleWithDecisions,
      {
        projectId: string;
        bibleId: string;
        body: {
          issue_key: string;
          action: ProductionBibleReviewDecision;
          expected_revision: number;
          idempotency_key: string;
        };
      }
    >({
      queryFn: ({ bibleId, body }) =>
        runRequest(() =>
          request<{ data: ProductionBibleWithDecisions }>(
            `/api/production-bibles/${bibleId}/review-decisions`,
            { method: "POST", data: body },
          ),
        ),
      invalidatesTags: (_result, _error, { projectId, bibleId }) => [
        { type: "ProductionBible", id: projectId },
        { type: "ProductionBible", id: bibleId },
      ],
    }),
    resumeProductionBible: builder.mutation<
      ProductionBibleWithDecisions,
      { projectId: string; bibleId: string; body: API.ProductionBibleResumeRequest }
    >({
      queryFn: ({ bibleId, body }) =>
        runRequest(() =>
          resumeBibleApiProductionBiblesBibleIdResumePost(
            { bible_id: bibleId },
            body,
          ) as Promise<{ data: ProductionBibleWithDecisions }>,
        ),
      invalidatesTags: (_result, _error, { projectId, bibleId }) => [
        { type: "ProductionBible", id: projectId },
        { type: "ProductionBible", id: bibleId },
      ],
    }),
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
  useChangePasswordMutation,
  useClaimHumanTaskMutation,
  useCompleteMediaUploadMutation,
  useConfirmEpisodePlanMutation,
  useConfirmRegistrationVerificationMutation,
  useCreateEpisodePlanMutation,
  useCreateProductionBibleMutation,
  useCreateProjectMutation,
  useCreateWorkspaceMutation,
  useCurrentScriptDocumentQuery,
  useCurrentProductionBibleQuery,
  useDecideHumanTaskMutation,
  useDeactivateAccountMutation,
  useDecideProductionBibleReviewIssueMutation,
  useEpisodesQuery,
  useHumanTaskQuery,
  useHumanTasksQuery,
  useImportScriptDocumentMutation,
  useInitializeMediaUploadMutation,
  useLazyEpisodePlanQuery,
  useLazyMediaVersionQuery,
  useLoginMutation,
  useLogoutMutation,
  useMaterializeEpisodePlanMutation,
  useMeQuery,
  usePreviewScriptDocumentMutation,
  useProductionBibleQuery,
  useProjectQuery,
  useProjectsQuery,
  usePublishImportCommitMutation,
  useRegisterMutation,
  useReleaseHumanTaskClaimMutation,
  useRequestRegistrationVerificationMutation,
  useResumeProductionBibleMutation,
  useResumeHumanGateMutation,
  useRenewHumanTaskClaimMutation,
  useSetWorkspaceArchivedMutation,
  useUpdateProfileMutation,
  useUpdateWorkspaceMutation,
  useWorkspacesQuery,
  useWorkflowRunQuery,
} = appApi;
