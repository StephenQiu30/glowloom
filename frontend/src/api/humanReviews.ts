import request, { type RequestOptions } from "@/lib/request";

type Envelope<T> = { data: T };

type HumanTaskListParams = {
  project_id: string;
  status?: "active" | API.HumanTaskBaseResponse["status"];
  subject_type?: string | null;
  limit?: number;
  after?: string | null;
};

export function listHumanTasksApiProjectsProjectIdHumanTasksGet(
  params: HumanTaskListParams,
  options?: RequestOptions,
) {
  const {
    project_id: projectId,
    status,
    subject_type: subjectType,
    limit,
    after,
  } = params;
  return request<Envelope<API.HumanTaskListEnvelope["data"]>>(
    `/api/projects/${projectId}/human-tasks`,
    {
      method: "GET",
      params: {
        ...(status ? { status } : {}),
        ...(subjectType ? { subject_type: subjectType } : {}),
        ...(limit ? { limit } : {}),
        ...(after ? { after } : {}),
      },
      ...(options ?? {}),
    },
  );
}

export function getHumanTaskApiHumanTasksHumanTaskIdGet(
  params: { human_task_id: string },
  options?: RequestOptions,
) {
  return request<Envelope<API.HumanTaskDetailEnvelope["data"]>>(
    `/api/human-tasks/${params.human_task_id}`,
    { method: "GET", ...(options ?? {}) },
  );
}

export function claimHumanTaskApiHumanTasksHumanTaskIdClaimsPost(
  params: { human_task_id: string },
  body: API.HumanTaskClaimRequest,
  options?: RequestOptions,
) {
  return request<Envelope<API.HumanTaskCommandEnvelope["data"]>>(
    `/api/human-tasks/${params.human_task_id}/claims`,
    { method: "POST", data: body, ...(options ?? {}) },
  );
}

export function renewHumanTaskClaimApiHumanTasksHumanTaskIdClaimRenewalsPost(
  params: { human_task_id: string },
  body: API.HumanTaskClaimTokenRequest,
  options?: RequestOptions,
) {
  return request<Envelope<API.HumanTaskCommandEnvelope["data"]>>(
    `/api/human-tasks/${params.human_task_id}/claim-renewals`,
    { method: "POST", data: body, ...(options ?? {}) },
  );
}

export function releaseHumanTaskClaimApiHumanTasksHumanTaskIdClaimReleasesPost(
  params: { human_task_id: string },
  body: API.HumanTaskClaimTokenRequest,
  options?: RequestOptions,
) {
  return request<Envelope<API.HumanTaskCommandEnvelope["data"]>>(
    `/api/human-tasks/${params.human_task_id}/claim-releases`,
    { method: "POST", data: body, ...(options ?? {}) },
  );
}

export function decideHumanTaskApiHumanTasksHumanTaskIdDecisionsPost(
  params: { human_task_id: string },
  body: API.HumanTaskDecisionRequest,
  options?: RequestOptions,
) {
  return request<Envelope<API.HumanGateDecisionEnvelope["data"]>>(
    `/api/human-tasks/${params.human_task_id}/decisions`,
    { method: "POST", data: body, ...(options ?? {}) },
  );
}

export function resumeHumanGateApiReviewDecisionsReviewDecisionIdResumePost(
  params: { review_decision_id: string },
  options?: RequestOptions,
) {
  return request<Envelope<API.HumanGateResumeEnvelope["data"]>>(
    `/api/review-decisions/${params.review_decision_id}/resume`,
    { method: "POST", ...(options ?? {}) },
  );
}
