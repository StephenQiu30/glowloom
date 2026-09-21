// @ts-ignore
/* eslint-disable */
import request, { type RequestOptions } from "@/lib/request";

/** 此处后端没有提供注释 POST /api/auth/change-password */
export async function changePassword(options?: RequestOptions) {
  return request<any>("/api/auth/change-password", {
    method: "POST",
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/auth/login */
export async function login(options?: RequestOptions) {
  return request<any>("/api/auth/login", {
    method: "POST",
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/auth/logout */
export async function logout(options?: RequestOptions) {
  return request<any>("/api/auth/logout", {
    method: "POST",
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/auth/refresh */
export async function refresh(options?: RequestOptions) {
  return request<any>("/api/auth/refresh", {
    method: "POST",
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/auth/register */
export async function register(options?: RequestOptions) {
  return request<any>("/api/auth/register", {
    method: "POST",
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/auth/registration-verifications */
export async function requestRegistrationVerification(options?: RequestOptions) {
  return request<any>("/api/auth/registration-verifications", {
    method: "POST",
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/auth/registration-verifications/confirm */
export async function confirmRegistrationVerification(options?: RequestOptions) {
  return request<any>("/api/auth/registration-verifications/confirm", {
    method: "POST",
    ...(options || {}),
  });
}

/** 校验当前项目权限后读取已持久化的运行交接状态和固定接受回执。 GET /api/creation-runs/${param0} */
export async function getCreationRun(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.getCreationRunParams,
  options?: RequestOptions,
) {
  const { run_id: param0, ...queryParams } = params;
  return request<{ data: GeneratedAPI.CreationRunResponse }>(`/api/creation-runs/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 按当前读权限和固定Agent路由查询持久执行状态；不产生提案或模型调用。 GET /api/creation-runs/${param0}/execution */
export async function getCreationExecution(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.getCreationExecutionParams,
  options?: RequestOptions,
) {
  const { run_id: param0, ...queryParams } = params;
  return request<{ data: GeneratedAPI.CreationExecution }>(
    `/api/creation-runs/${param0}/execution`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 当前用户与项目鉴权后的固定版本查询；GET 无请求体和查询参数；服务异常不会触发重跑。 GET /api/creation-runs/${param0}/manifest */
export async function getCreationManifest(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.getCreationManifestParams,
  options?: RequestOptions,
) {
  const { run_id: param0, ...queryParams } = params;
  return request<{ data: GeneratedAPI.CreationManifestSnapshot }>(
    `/api/creation-runs/${param0}/manifest`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 从 Go 持久化读取提案与正式采纳回执，不向 Agent 发请求。 GET /api/creation-runs/${param0}/proposals */
export async function listCreationProposals(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.listCreationProposalsParams,
  options?: RequestOptions,
) {
  const { run_id: param0, ...queryParams } = params;
  return request<{ data: GeneratedAPI.CreationProposal[] }>(
    `/api/creation-runs/${param0}/proposals`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 按固定提案版本和人工决定操作；采纳与正式 Owner 回执同事务。 GET /api/creation-runs/${param0}/proposals/${param1} */
export async function getCreationProposal(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.getCreationProposalParams,
  options?: RequestOptions,
) {
  const { run_id: param0, proposal_id: param1, ...queryParams } = params;
  return request<{ data: GeneratedAPI.CreationProposal }>(
    `/api/creation-runs/${param0}/proposals/${param1}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 按固定提案版本和人工决定操作；采纳与正式 Owner 回执同事务。 POST /api/creation-runs/${param0}/proposals/${param1}/adopt */
export async function adoptCreationProposal(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.adoptCreationProposalParams,
  body: GeneratedAPI.AdoptCreationProposalRequest,
  options?: RequestOptions,
) {
  const { run_id: param0, proposal_id: param1, ...queryParams } = params;
  return request<{ data: GeneratedAPI.CreationAdoptionReceipt }>(
    `/api/creation-runs/${param0}/proposals/${param1}/adopt`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 仅原创建者按当前写权限及运行revision请求恢复可恢复的blocked。保留原Workflow、输入、草案与预算；unknown/不可恢复/非blocked返回409。202只表示恢复请求已接受。 POST /api/creation-runs/${param0}/resume */
export async function resumeCreationRun(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.resumeCreationRunParams,
  body: GeneratedAPI.ResumeCreationRunRequest,
  options?: RequestOptions,
) {
  const { run_id: param0, ...queryParams } = params;
  return request<any>(`/api/creation-runs/${param0}/resume`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 仅原创建者可使用当前权限对 delivery_blocked 重投；expected_revision 做 CAS。保留命令、原稿、目标路由和 Workflow ID；不新建创作运行。重复旧 revision 返回 409，可查询原运行恢复。 POST /api/creation-runs/${param0}/retry-delivery */
export async function retryCreationDelivery(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.retryCreationDeliveryParams,
  body: GeneratedAPI.RetryCreationDeliveryRequest,
  options?: RequestOptions,
) {
  const { run_id: param0, ...queryParams } = params;
  return request<{ data: GeneratedAPI.CreationRunResponse }>(
    `/api/creation-runs/${param0}/retry-delivery`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 当前用户与项目鉴权后的固定版本查询；GET 无请求体和查询参数；服务异常不会触发重跑。 GET /api/creation-runs/${param0}/steps/${param1}/attempts */
export async function getCreationAttemptHistory(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.getCreationAttemptHistoryParams,
  options?: RequestOptions,
) {
  const { run_id: param0, step_id: param1, ...queryParams } = params;
  return request<{ data: GeneratedAPI.CreationAttemptHistory }>(
    `/api/creation-runs/${param0}/steps/${param1}/attempts`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 校验当前写权限后，按固定 Agent 路由读取并验证执行及草案，幂等同步人工审核对象；不触发模型重试。 POST /api/creation-runs/${param0}/sync-proposals */
export async function syncCreationProposals(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.syncCreationProposalsParams,
  body: {},
  options?: RequestOptions,
) {
  const { run_id: param0, ...queryParams } = params;
  return request<{ data: GeneratedAPI.CreationSyncResult }>(
    `/api/creation-runs/${param0}/sync-proposals`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 GET /api/document-revisions/${param0} */
export async function getDocumentRevision(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.getDocumentRevisionParams,
  options?: RequestOptions,
) {
  const { revision_id: param0, ...queryParams } = params;
  return request<any>(`/api/document-revisions/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/document-revisions/${param0}/episode-plans */
export async function createEpisodePlan(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.createEpisodePlanParams,
  options?: RequestOptions,
) {
  const { revision_id: param0, ...queryParams } = params;
  return request<any>(`/api/document-revisions/${param0}/episode-plans`, {
    method: "POST",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/document-revisions/${param0}/production-bibles */
export async function createProductionBible(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.createProductionBibleParams,
  options?: RequestOptions,
) {
  const { revision_id: param0, ...queryParams } = params;
  return request<any>(`/api/document-revisions/${param0}/production-bibles`, {
    method: "POST",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/episode-plans/${param0} */
export async function getEpisodePlan(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.getEpisodePlanParams,
  options?: RequestOptions,
) {
  const { plan_id: param0, ...queryParams } = params;
  return request<any>(`/api/episode-plans/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/episode-plans/${param0}/confirm */
export async function confirmEpisodePlan(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.confirmEpisodePlanParams,
  options?: RequestOptions,
) {
  const { plan_id: param0, ...queryParams } = params;
  return request<any>(`/api/episode-plans/${param0}/confirm`, {
    method: "POST",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/episode-plans/${param0}/materializations */
export async function materializeEpisodePlan(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.materializeEpisodePlanParams,
  options?: RequestOptions,
) {
  const { plan_id: param0, ...queryParams } = params;
  return request<any>(`/api/episode-plans/${param0}/materializations`, {
    method: "POST",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/episode-structures/${param0}/confirm */
export async function confirmEpisodeStructure(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.confirmEpisodeStructureParams,
  options?: RequestOptions,
) {
  const { structure_id: param0, ...queryParams } = params;
  return request<any>(`/api/episode-structures/${param0}/confirm`, {
    method: "POST",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/episode-structures/${param0}/tasks/${param1}/accept */
export async function acceptProductionTask(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.acceptProductionTaskParams,
  options?: RequestOptions,
) {
  const { structure_id: param0, task_id: param1, ...queryParams } = params;
  return request<any>(`/api/episode-structures/${param0}/tasks/${param1}/accept`, {
    method: "POST",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/episodes/${param0} */
export async function getEpisode(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.getEpisodeParams,
  options?: RequestOptions,
) {
  const { episode_id: param0, ...queryParams } = params;
  return request<any>(`/api/episodes/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/episodes/${param0}/shots */
export async function listStoryboardShots(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.listStoryboardShotsParams,
  options?: RequestOptions,
) {
  const { episode_id: param0, ...queryParams } = params;
  return request<any>(`/api/episodes/${param0}/shots`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/episodes/${param0}/storyboard-draft */
export async function getLatestStoryboardDraft(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.getLatestStoryboardDraftParams,
  options?: RequestOptions,
) {
  const { episode_id: param0, ...queryParams } = params;
  return request<any>(`/api/episodes/${param0}/storyboard-draft`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/episodes/${param0}/storyboard-drafts */
export async function createStoryboardDraft(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.createStoryboardDraftParams,
  options?: RequestOptions,
) {
  const { episode_id: param0, ...queryParams } = params;
  return request<any>(`/api/episodes/${param0}/storyboard-drafts`, {
    method: "POST",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/episodes/${param0}/storyboard-export */
export async function getLatestStoryboardExport(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.getLatestStoryboardExportParams,
  options?: RequestOptions,
) {
  const { episode_id: param0, ...queryParams } = params;
  return request<any>(`/api/episodes/${param0}/storyboard-export`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/episodes/${param0}/storyboard-exports */
export async function createStoryboardExport(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.createStoryboardExportParams,
  options?: RequestOptions,
) {
  const { episode_id: param0, ...queryParams } = params;
  return request<any>(`/api/episodes/${param0}/storyboard-exports`, {
    method: "POST",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/episodes/${param0}/storyboard-exports/preflight */
export async function storyboardExportPreflight(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.storyboardExportPreflightParams,
  options?: RequestOptions,
) {
  const { episode_id: param0, ...queryParams } = params;
  return request<any>(`/api/episodes/${param0}/storyboard-exports/preflight`, {
    method: "POST",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/episodes/${param0}/structure */
export async function getEpisodeStructure(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.getEpisodeStructureParams,
  options?: RequestOptions,
) {
  const { episode_id: param0, ...queryParams } = params;
  return request<any>(`/api/episodes/${param0}/structure`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/human-tasks/${param0} */
export async function getHumanTask(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.getHumanTaskParams,
  options?: RequestOptions,
) {
  const { human_task_id: param0, ...queryParams } = params;
  return request<GeneratedAPI.HumanTaskDetailEnvelope>(`/api/human-tasks/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/human-tasks/${param0}/claim-releases */
export async function releaseHumanTaskClaim(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.releaseHumanTaskClaimParams,
  body: GeneratedAPI.HumanTaskClaimTokenRequest,
  options?: RequestOptions,
) {
  const { human_task_id: param0, ...queryParams } = params;
  return request<GeneratedAPI.HumanTaskCommandEnvelope>(
    `/api/human-tasks/${param0}/claim-releases`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 POST /api/human-tasks/${param0}/claim-renewals */
export async function renewHumanTaskClaim(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.renewHumanTaskClaimParams,
  body: GeneratedAPI.HumanTaskClaimTokenRequest,
  options?: RequestOptions,
) {
  const { human_task_id: param0, ...queryParams } = params;
  return request<GeneratedAPI.HumanTaskCommandEnvelope>(
    `/api/human-tasks/${param0}/claim-renewals`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 POST /api/human-tasks/${param0}/claims */
export async function claimHumanTask(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.claimHumanTaskParams,
  body: GeneratedAPI.HumanTaskClaimRequest,
  options?: RequestOptions,
) {
  const { human_task_id: param0, ...queryParams } = params;
  return request<GeneratedAPI.HumanTaskCommandEnvelope>(`/api/human-tasks/${param0}/claims`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/human-tasks/${param0}/decisions */
export async function decideHumanTask(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.decideHumanTaskParams,
  body: GeneratedAPI.HumanTaskDecisionRequest,
  options?: RequestOptions,
) {
  const { human_task_id: param0, ...queryParams } = params;
  return request<GeneratedAPI.HumanGateDecisionEnvelope>(`/api/human-tasks/${param0}/decisions`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/import-commits/${param0}/publish */
export async function publishEpisodeScripts(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.publishEpisodeScriptsParams,
  options?: RequestOptions,
) {
  const { commit_id: param0, ...queryParams } = params;
  return request<any>(`/api/import-commits/${param0}/publish`, {
    method: "POST",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/me */
export async function getMe(options?: RequestOptions) {
  return request<any>("/api/me", {
    method: "GET",
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 PATCH /api/me */
export async function updateMe(options?: RequestOptions) {
  return request<any>("/api/me", {
    method: "PATCH",
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/me/deactivate */
export async function deactivateMe(options?: RequestOptions) {
  return request<any>("/api/me/deactivate", {
    method: "POST",
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/media/${param0} */
export async function getMediaVersion(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.getMediaVersionParams,
  options?: RequestOptions,
) {
  const { version_id: param0, ...queryParams } = params;
  return request<any>(`/api/media/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/media/uploads */
export async function initializeMediaUpload(options?: RequestOptions) {
  return request<any>("/api/media/uploads", {
    method: "POST",
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/media/uploads/${param0}/complete */
export async function completeMediaUpload(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.completeMediaUploadParams,
  options?: RequestOptions,
) {
  const { upload_session_id: param0, ...queryParams } = params;
  return request<any>(`/api/media/uploads/${param0}/complete`, {
    method: "POST",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 返回 Backend 内置、内容定址的策展 Preset Release；不接受 current/latest，也不读取 Provider 配置。 GET /api/presets */
export async function listPresets(options?: RequestOptions) {
  return request<{ data: { items: GeneratedAPI.PresetRelease[] } }>("/api/presets", {
    method: "GET",
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/production-bibles/${param0} */
export async function getProductionBible(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.getProductionBibleParams,
  options?: RequestOptions,
) {
  const { bible_id: param0, ...queryParams } = params;
  return request<any>(`/api/production-bibles/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/production-bibles/${param0}/resume */
export async function resumeProductionBible(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.resumeProductionBibleParams,
  options?: RequestOptions,
) {
  const { bible_id: param0, ...queryParams } = params;
  return request<any>(`/api/production-bibles/${param0}/resume`, {
    method: "POST",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/production-bibles/${param0}/review-decisions */
export async function decideProductionBibleReviewIssue(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.decideProductionBibleReviewIssueParams,
  body: GeneratedAPI.ProductionBibleReviewDecisionRequest,
  options?: RequestOptions,
) {
  const { bible_id: param0, ...queryParams } = params;
  return request<GeneratedAPI.ProductionBibleEnvelope>(
    `/api/production-bibles/${param0}/review-decisions`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 GET /api/projects */
export async function listProjects(options?: RequestOptions) {
  return request<any>("/api/projects", {
    method: "GET",
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/projects */
export async function createProject(options?: RequestOptions) {
  return request<any>("/api/projects", {
    method: "POST",
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/projects/${param0} */
export async function getProject(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.getProjectParams,
  options?: RequestOptions,
) {
  const { project_id: param0, ...queryParams } = params;
  return request<any>(`/api/projects/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 DELETE /api/projects/${param0} */
export async function deleteProject(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.deleteProjectParams,
  options?: RequestOptions,
) {
  const { project_id: param0, ...queryParams } = params;
  return request<any>(`/api/projects/${param0}`, {
    method: "DELETE",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 PATCH /api/projects/${param0} */
export async function updateProject(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.updateProjectParams,
  options?: RequestOptions,
) {
  const { project_id: param0, ...queryParams } = params;
  return request<any>(`/api/projects/${param0}`, {
    method: "PATCH",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/projects/${param0}/archive */
export async function archiveProject(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.archiveProjectParams,
  options?: RequestOptions,
) {
  const { project_id: param0, ...queryParams } = params;
  return request<any>(`/api/projects/${param0}/archive`, {
    method: "POST",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/projects/${param0}/cost-budget */
export async function getCostBudget(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.getCostBudgetParams,
  options?: RequestOptions,
) {
  const { project_id: param0, ...queryParams } = params;
  return request<{ data: GeneratedAPI.CostBudgetResponse }>(`/api/projects/${param0}/cost-budget`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/projects/${param0}/cost-budget */
export async function setCostBudget(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.setCostBudgetParams,
  body: GeneratedAPI.CostBudgetSetRequest,
  options?: RequestOptions,
) {
  const { project_id: param0, ...queryParams } = params;
  return request<{ data: GeneratedAPI.CostBudgetResponse }>(`/api/projects/${param0}/cost-budget`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 按创建时间倒序返回项目最近的创作交接记录，最多 100 条。 GET /api/projects/${param0}/creation-runs */
export async function listCreationRuns(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.listCreationRunsParams,
  options?: RequestOptions,
) {
  const { project_id: param0, ...queryParams } = params;
  return request<{ data: GeneratedAPI.CreationRunResponse[] }>(
    `/api/projects/${param0}/creation-runs`,
    {
      method: "GET",
      params: {
        // limit has a default value: 20
        limit: "20",
        ...queryParams,
      },
      ...(options || {}),
    },
  );
}

/** 持久化新 Python 创作命令和固定路由。202 表示 Go 接受交接；accepted 状态只表示 Agent 已持久接受，均不表示创作完成。未配置新流程返回 503。同项目及用户下同幂等键同输入返回原运行，异输入返回 409。 POST /api/projects/${param0}/creation-runs */
export async function createCreationRun(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.createCreationRunParams,
  body: GeneratedAPI.CreateCreationRunRequest,
  options?: RequestOptions,
) {
  const { project_id: param0, ...queryParams } = params;
  return request<any>(`/api/projects/${param0}/creation-runs`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/projects/${param0}/current-script-document */
export async function getCurrentScriptDocument(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.getCurrentScriptDocumentParams,
  options?: RequestOptions,
) {
  const { project_id: param0, ...queryParams } = params;
  return request<any>(`/api/projects/${param0}/current-script-document`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 读取已接受的当前原稿及 Head CAS 身份；导入新文档不会自动替换它。无已接受源返回404。 GET /api/projects/${param0}/current-script-source */
export async function getCurrentScriptSource(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.getCurrentScriptSourceParams,
  options?: RequestOptions,
) {
  const { project_id: param0, ...queryParams } = params;
  return request<{ data: GeneratedAPI.AcceptedScriptSourceResponse }>(
    `/api/projects/${param0}/current-script-source`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 POST /api/projects/${param0}/delete-preflight */
export async function projectDeletePreflight(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.projectDeletePreflightParams,
  options?: RequestOptions,
) {
  const { project_id: param0, ...queryParams } = params;
  return request<any>(`/api/projects/${param0}/delete-preflight`, {
    method: "POST",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/projects/${param0}/episodes */
export async function listEpisodes(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.listEpisodesParams,
  options?: RequestOptions,
) {
  const { project_id: param0, ...queryParams } = params;
  return request<any>(`/api/projects/${param0}/episodes`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/projects/${param0}/human-tasks */
export async function listHumanTasks(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.listHumanTasksParams,
  options?: RequestOptions,
) {
  const { project_id: param0, ...queryParams } = params;
  return request<GeneratedAPI.HumanTaskListEnvelope>(`/api/projects/${param0}/human-tasks`, {
    method: "GET",
    params: {
      // status has a default value: active
      status: "active",

      // limit has a default value: 50
      limit: "50",
      ...queryParams,
    },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/projects/${param0}/media-model-profiles/${param1}/cost-price */
export async function getCurrentCostPriceQuote(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.getCurrentCostPriceQuoteParams,
  options?: RequestOptions,
) {
  const { project_id: param0, profile_version_id: param1, ...queryParams } = params;
  return request<{ data: GeneratedAPI.CostPriceQuoteResponse }>(
    `/api/projects/${param0}/media-model-profiles/${param1}/cost-price`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 POST /api/projects/${param0}/media-model-profiles/${param1}/cost-price */
export async function setCostPriceQuote(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.setCostPriceQuoteParams,
  body: GeneratedAPI.CostPriceQuoteSetRequest,
  options?: RequestOptions,
) {
  const { project_id: param0, profile_version_id: param1, ...queryParams } = params;
  return request<{ data: GeneratedAPI.CostPriceQuoteResponse }>(
    `/api/projects/${param0}/media-model-profiles/${param1}/cost-price`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 按当前项目读权限返回 Preset Selection Current Head 指向的精确不可变版本。 GET /api/projects/${param0}/preset-selection */
export async function getProjectPresetSelection(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.getProjectPresetSelectionParams,
  options?: RequestOptions,
) {
  const { project_id: param0, ...queryParams } = params;
  return request<{ data: GeneratedAPI.ProjectPresetSelection }>(
    `/api/projects/${param0}/preset-selection`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 以 expected_revision CAS 和 idempotency_key 原子发布新的 Project Preset Selection；重复同一命令返回原结果。 PUT /api/projects/${param0}/preset-selection */
export async function selectProjectPreset(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.selectProjectPresetParams,
  body: GeneratedAPI.ProjectPresetSelectionRequest,
  options?: RequestOptions,
) {
  const { project_id: param0, ...queryParams } = params;
  return request<{ data: GeneratedAPI.ProjectPresetSelection }>(
    `/api/projects/${param0}/preset-selection`,
    {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 GET /api/projects/${param0}/production-bible */
export async function getCurrentProductionBible(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.getCurrentProductionBibleParams,
  options?: RequestOptions,
) {
  const { project_id: param0, ...queryParams } = params;
  return request<any>(`/api/projects/${param0}/production-bible`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 读取已持久化整组视觉审核候选 Bundle，重验当前权限、精确输入和审核 Control；不代表权利批准、人工选择或资产发布。 GET /api/projects/${param0}/reference-candidate-bundles/${param1} */
export async function getReferenceCandidateBundle(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.getReferenceCandidateBundleParams,
  options?: RequestOptions,
) {
  const { project_id: param0, bundle_id: param1, ...queryParams } = params;
  return request<{ data: GeneratedAPI.ReferenceCandidateBundleResponse }>(
    `/api/projects/${param0}/reference-candidate-bundles/${param1}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 精确读取候选集并重验当前权限、执行进度和审核 Control；结果未知只用于对账，不代表可选择或发布。 GET /api/projects/${param0}/reference-candidate-sets/${param1} */
export async function getReferenceCandidateSet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.getReferenceCandidateSetParams,
  options?: RequestOptions,
) {
  const { project_id: param0, set_id: param1, ...queryParams } = params;
  return request<{ data: GeneratedAPI.ReferenceCandidateSetResponse }>(
    `/api/projects/${param0}/reference-candidate-sets/${param1}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 从当前 Approved Reference Plan 与精确 Reference Brief 执行事实读取只读覆盖矩阵；不触发生成、重跑或状态写入。 GET /api/projects/${param0}/reference-coverage */
export async function getReferenceCoverageMatrix(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.getReferenceCoverageMatrixParams,
  options?: RequestOptions,
) {
  const { project_id: param0, ...queryParams } = params;
  return request<{ data: GeneratedAPI.ReferenceCoverageMatrixResponse }>(
    `/api/projects/${param0}/reference-coverage`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 读取精确 Execution 完整冻结 Call 的传输进度；不触发发送、媒体读取或正式资产发布，传输成功不代表 Bundle/QC/Selection 通过。 GET /api/projects/${param0}/reference-executions/${param1} */
export async function getReferenceExecutionProgress(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.getReferenceExecutionProgressParams,
  options?: RequestOptions,
) {
  const { project_id: param0, execution_id: param1, ...queryParams } = params;
  return request<{ data: GeneratedAPI.ReferenceExecutionProgressResponse }>(
    `/api/projects/${param0}/reference-executions/${param1}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 读取完整明确结果的同组 Bundle Input 和确定性 QC 派生快照；未决结果拒绝，权利未评估保持 blocked，不代表 Vision 或资产发布。 GET /api/projects/${param0}/reference-executions/${param1}/bundle-inputs */
export async function getReferenceBundleInputs(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.getReferenceBundleInputsParams,
  options?: RequestOptions,
) {
  const { project_id: param0, execution_id: param1, ...queryParams } = params;
  return request<{ data: GeneratedAPI.ReferenceBundleInputsResponse }>(
    `/api/projects/${param0}/reference-executions/${param1}/bundle-inputs`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 按冻结的完整执行进度 Hash 和已审核 Bundle 引用原子生成候选集；拒绝缺失审核或陈旧进度，同一输入重试收敛到同一事实。 POST /api/projects/${param0}/reference-executions/${param1}/candidate-sets */
export async function materializeReferenceCandidateSet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.materializeReferenceCandidateSetParams,
  body: GeneratedAPI.ReferenceCandidateSetRequest,
  options?: RequestOptions,
) {
  const { project_id: param0, execution_id: param1, ...queryParams } = params;
  return request<{ data: GeneratedAPI.ReferenceCandidateSetResponse }>(
    `/api/projects/${param0}/reference-executions/${param1}/candidate-sets`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 启动已准备 Execution 的完整 Call 观察 DAG；明确失败继续收集，未知结果停止对账。收集成功不代表媒体或 Bundle/QC 合格。 POST /api/projects/${param0}/reference-executions/${param1}/workflow-runs */
export async function startReferenceExecutionWorkflow(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.startReferenceExecutionWorkflowParams,
  body: GeneratedAPI.ReferenceExecutionWorkflowStartRequest,
  options?: RequestOptions,
) {
  const { project_id: param0, execution_id: param1, ...queryParams } = params;
  return request<any>(`/api/projects/${param0}/reference-executions/${param1}/workflow-runs`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 从已授权 accepted Brief 发布首轮 Target；不授权执行。 POST /api/projects/${param0}/reference-generation-targets */
export async function buildInitialReferenceGenerationTarget(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.buildInitialReferenceGenerationTargetParams,
  body: GeneratedAPI.ReferenceGenerationTargetBuildRequest,
  options?: RequestOptions,
) {
  const { project_id: param0, ...queryParams } = params;
  return request<{ data: GeneratedAPI.ReferenceGenerationTargetBuildResponse }>(
    `/api/projects/${param0}/reference-generation-targets`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 读取已发布生成 Target 的冻结来源身份及当前 Execution Head 对应的完整传输进度。execution 为 null 表示尚未准备，不代表任何审核或执行许可。 GET /api/projects/${param0}/reference-generation-targets/${param1} */
export async function getReferenceGenerationProgress(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.getReferenceGenerationProgressParams,
  options?: RequestOptions,
) {
  const { project_id: param0, generation_target_id: param1, ...queryParams } = params;
  return request<{ data: GeneratedAPI.ReferenceGenerationProgressResponse }>(
    `/api/projects/${param0}/reference-generation-targets/${param1}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 用户明确选择 Provider Binding，记录独立执行授权，不发送请求。 POST /api/projects/${param0}/reference-generation-targets/${param1}/execution-authorizations */
export async function authorizeInitialReferenceExecution(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.authorizeInitialReferenceExecutionParams,
  body: GeneratedAPI.ReferenceExecutionAuthorizationRequest,
  options?: RequestOptions,
) {
  const { project_id: param0, generation_target_id: param1, ...queryParams } = params;
  return request<{ data: GeneratedAPI.ReferenceExecutionAuthorizationResponse }>(
    `/api/projects/${param0}/reference-generation-targets/${param1}/execution-authorizations`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 准备首轮 Execution 和全量 PENDING Call；须另行显式启动工作流。 POST /api/projects/${param0}/reference-generation-targets/${param1}/executions */
export async function prepareInitialReferenceExecution(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.prepareInitialReferenceExecutionParams,
  body: GeneratedAPI.ReferenceExecutionPreparationRequest,
  options?: RequestOptions,
) {
  const { project_id: param0, generation_target_id: param1, ...queryParams } = params;
  return request<{ data: GeneratedAPI.ReferenceExecutionPreparationResponse }>(
    `/api/projects/${param0}/reference-generation-targets/${param1}/executions`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 按当前 Approved Plan 内的精确 Target Version ID 读取不可变 Target 与同一覆盖行；不接受 current/latest。 GET /api/projects/${param0}/reference-targets/${param1} */
export async function getReferenceTargetDetail(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.getReferenceTargetDetailParams,
  options?: RequestOptions,
) {
  const { project_id: param0, target_version_id: param1, ...queryParams } = params;
  return request<{ data: GeneratedAPI.ReferenceTargetDetailResponse }>(
    `/api/projects/${param0}/reference-targets/${param1}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 记录独立首次生成授权，不构建 Target 或发送请求。 POST /api/projects/${param0}/reference-targets/${param1}/generation-authorizations */
export async function authorizeInitialReferenceGeneration(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.authorizeInitialReferenceGenerationParams,
  body: GeneratedAPI.ReferenceGenerationAuthorizationRequest,
  options?: RequestOptions,
) {
  const { project_id: param0, target_version_id: param1, ...queryParams } = params;
  return request<{ data: GeneratedAPI.ReferenceGenerationAuthorizationResponse }>(
    `/api/projects/${param0}/reference-targets/${param1}/generation-authorizations`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 POST /api/projects/${param0}/restore */
export async function restoreProject(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.restoreProjectParams,
  options?: RequestOptions,
) {
  const { project_id: param0, ...queryParams } = params;
  return request<any>(`/api/projects/${param0}/restore`, {
    method: "POST",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/projects/${param0}/scene-analysis-candidates/${param1} */
export async function getSceneAnalysisCandidate(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.getSceneAnalysisCandidateParams,
  options?: RequestOptions,
) {
  const { project_id: param0, candidate_id: param1, ...queryParams } = params;
  return request<{ data: GeneratedAPI.SceneAnalysisCandidateResponse }>(
    `/api/projects/${param0}/scene-analysis-candidates/${param1}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 GET /api/projects/${param0}/script-documents */
export async function listScriptDocuments(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.listScriptDocumentsParams,
  options?: RequestOptions,
) {
  const { project_id: param0, ...queryParams } = params;
  return request<any>(`/api/projects/${param0}/script-documents`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/projects/${param0}/script-import-previews */
export async function previewScriptImport(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.previewScriptImportParams,
  options?: RequestOptions,
) {
  const { project_id: param0, ...queryParams } = params;
  return request<any>(`/api/projects/${param0}/script-import-previews`, {
    method: "POST",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/projects/${param0}/script-imports */
export async function commitScriptImport(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.commitScriptImportParams,
  options?: RequestOptions,
) {
  const { project_id: param0, ...queryParams } = params;
  return request<any>(`/api/projects/${param0}/script-imports`, {
    method: "POST",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/projects/${param0}/script-sources */
export async function acceptScriptSource(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.acceptScriptSourceParams,
  body: GeneratedAPI.AcceptScriptSourceRequest,
  options?: RequestOptions,
) {
  const { project_id: param0, ...queryParams } = params;
  return request<{ data: GeneratedAPI.AcceptedScriptSourceResponse }>(
    `/api/projects/${param0}/script-sources`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 GET /api/projects/${param0}/script-sources/${param1} */
export async function getAcceptedScriptSource(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.getAcceptedScriptSourceParams,
  options?: RequestOptions,
) {
  const { project_id: param0, revision_id: param1, ...queryParams } = params;
  return request<{ data: GeneratedAPI.AcceptedScriptSourceResponse }>(
    `/api/projects/${param0}/script-sources/${param1}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 读取已接受固定版本的 Unicode code point 半开区间，最多 16384 字符。每个查询参数只接受一个值；不会自动切换到最新源。 GET /api/projects/${param0}/script-sources/${param1}/spans */
export async function getScriptSourceSpan(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.getScriptSourceSpanParams,
  options?: RequestOptions,
) {
  const { project_id: param0, revision_id: param1, ...queryParams } = params;
  return request<{ data: GeneratedAPI.ScriptSourceSpanResponse }>(
    `/api/projects/${param0}/script-sources/${param1}/spans`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 GET /api/projects/${param0}/search/scripts */
export async function searchScripts(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.searchScriptsParams,
  options?: RequestOptions,
) {
  const { project_id: param0, ...queryParams } = params;
  return request<GeneratedAPI.SearchEnvelope>(`/api/projects/${param0}/search/scripts`, {
    method: "GET",
    params: {
      ...queryParams,
    },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/projects/${param0}/search/storygraph */
export async function searchStoryGraph(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.searchStoryGraphParams,
  options?: RequestOptions,
) {
  const { project_id: param0, ...queryParams } = params;
  return request<GeneratedAPI.SearchEnvelope>(`/api/projects/${param0}/search/storygraph`, {
    method: "GET",
    params: {
      ...queryParams,
    },
    ...(options || {}),
  });
}

/** 根据已有 approved ReviewDecision 原子冻结精确候选及导演意图。幂等重放返回原回执；视觉资产可以仍为 needs_asset。当前接口读取 Go-owned 候选；新 Python 提案接入另行验收。 POST /api/projects/${param0}/storyboard-intent-acceptances */
export async function freezeStoryboardIntents(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.freezeStoryboardIntentsParams,
  body: GeneratedAPI.FreezeStoryboardIntentsRequest,
  options?: RequestOptions,
) {
  const { project_id: param0, ...queryParams } = params;
  return request<{ data: GeneratedAPI.StoryboardIntentAcceptanceResponse }>(
    `/api/projects/${param0}/storyboard-intent-acceptances`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 GET /api/projects/${param0}/storygraph/current */
export async function getCurrentStoryGraphVersion(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.getCurrentStoryGraphVersionParams,
  options?: RequestOptions,
) {
  const { project_id: param0, ...queryParams } = params;
  return request<GeneratedAPI.StoryGraphVersionEnvelope>(
    `/api/projects/${param0}/storygraph/current`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 GET /api/projects/${param0}/storygraph/diff */
export async function diffStoryGraphVersions(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.diffStoryGraphVersionsParams,
  options?: RequestOptions,
) {
  const { project_id: param0, ...queryParams } = params;
  return request<GeneratedAPI.StoryGraphDiffEnvelope>(`/api/projects/${param0}/storygraph/diff`, {
    method: "GET",
    params: {
      ...queryParams,
    },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/projects/${param0}/storygraph/versions/${param1} */
export async function getStoryGraphVersion(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.getStoryGraphVersionParams,
  options?: RequestOptions,
) {
  const { project_id: param0, version_id: param1, ...queryParams } = params;
  return request<GeneratedAPI.StoryGraphVersionEnvelope>(
    `/api/projects/${param0}/storygraph/versions/${param1}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 GET /api/projects/${param0}/storygraph/versions/${param1}/lens */
export async function queryStoryGraphLens(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.queryStoryGraphLensParams,
  options?: RequestOptions,
) {
  const { project_id: param0, version_ref: param1, ...queryParams } = params;
  return request<GeneratedAPI.StoryGraphSubgraphEnvelope>(
    `/api/projects/${param0}/storygraph/versions/${param1}/lens`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 GET /api/projects/${param0}/storygraph/versions/${param1}/nodes/${param2}/trace */
export async function traceStoryGraphNode(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.traceStoryGraphNodeParams,
  options?: RequestOptions,
) {
  const {
    project_id: param0,
    version_ref: param1,
    story_node_key: param2,
    ...queryParams
  } = params;
  return request<GeneratedAPI.StoryGraphSubgraphEnvelope>(
    `/api/projects/${param0}/storygraph/versions/${param1}/nodes/${param2}/trace`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    },
  );
}

/** 读取 Gate 1 已发布的当前结构身份版本和精确业务 Receipt；无请求体和查询参数，不触发分析或重跑。 GET /api/projects/${param0}/structure-identity */
export async function getCurrentStructureIdentity(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.getCurrentStructureIdentityParams,
  options?: RequestOptions,
) {
  const { project_id: param0, ...queryParams } = params;
  return request<{ data: GeneratedAPI.StructureIdentitySnapshotResponse }>(
    `/api/projects/${param0}/structure-identity`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 当前用户与项目鉴权后的固定版本查询；GET 无请求体和查询参数；服务异常不会触发重跑。 GET /api/projects/${param0}/text-intent-versions/${param1} */
export async function getTextIntentVersion(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.getTextIntentVersionParams,
  options?: RequestOptions,
) {
  const { project_id: param0, version_id: param1, ...queryParams } = params;
  return request<{ data: GeneratedAPI.StoryboardTextIntentVersion }>(
    `/api/projects/${param0}/text-intent-versions/${param1}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 当前用户与项目鉴权后的固定版本查询；GET 无请求体和查询参数；服务异常不会触发重跑。 GET /api/projects/${param0}/text-world-versions/${param1} */
export async function getTextWorldVersion(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.getTextWorldVersionParams,
  options?: RequestOptions,
) {
  const { project_id: param0, version_id: param1, ...queryParams } = params;
  return request<{ data: GeneratedAPI.BibleTextWorldVersion }>(
    `/api/projects/${param0}/text-world-versions/${param1}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 POST /api/review-decisions/${param0}/resume */
export async function resumeHumanGateFromReviewDecision(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.resumeHumanGateFromReviewDecisionParams,
  options?: RequestOptions,
) {
  const { review_decision_id: param0, ...queryParams } = params;
  return request<GeneratedAPI.HumanGateResumeEnvelope>(`/api/review-decisions/${param0}/resume`, {
    method: "POST",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/storyboard-draft-batches/${param0} */
export async function getStoryboardDraft(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.getStoryboardDraftParams,
  options?: RequestOptions,
) {
  const { batch_id: param0, ...queryParams } = params;
  return request<any>(`/api/storyboard-draft-batches/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/storyboard-draft-batches/${param0}/apply */
export async function applyStoryboardDraft(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.applyStoryboardDraftParams,
  options?: RequestOptions,
) {
  const { batch_id: param0, ...queryParams } = params;
  return request<any>(`/api/storyboard-draft-batches/${param0}/apply`, {
    method: "POST",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/storyboard-draft-batches/${param0}/apply-preflight */
export async function storyboardApplyPreflight(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.storyboardApplyPreflightParams,
  options?: RequestOptions,
) {
  const { batch_id: param0, ...queryParams } = params;
  return request<any>(`/api/storyboard-draft-batches/${param0}/apply-preflight`, {
    method: "POST",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/storyboard-draft-batches/${param0}/approve */
export async function approveStoryboardDraft(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.approveStoryboardDraftParams,
  options?: RequestOptions,
) {
  const { batch_id: param0, ...queryParams } = params;
  return request<any>(`/api/storyboard-draft-batches/${param0}/approve`, {
    method: "POST",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/storyboard-draft-batches/${param0}/decisions */
export async function acceptStoryboardDraftShot(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.acceptStoryboardDraftShotParams,
  options?: RequestOptions,
) {
  const { batch_id: param0, ...queryParams } = params;
  return request<any>(`/api/storyboard-draft-batches/${param0}/decisions`, {
    method: "POST",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 读取分镜草案集合、固定候选引用与当前状态。 GET /api/storyboard-draft-sets/${param0} */
export async function getStoryboardDraftSet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.getStoryboardDraftSetParams,
  options?: RequestOptions,
) {
  const { set_id: param0, ...queryParams } = params;
  return request<{ data: GeneratedAPI.StoryboardDraftSetResponse }>(
    `/api/storyboard-draft-sets/${param0}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 按已提交 Owner 回执恢复正式文本分镜，不触发模型或工作流。未采纳返回 409；每次查询重新检查项目权限。 GET /api/storyboard-draft-sets/${param0}/approved-intents */
export async function getApprovedStoryboardIntents(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.getApprovedStoryboardIntentsParams,
  options?: RequestOptions,
) {
  const { set_id: param0, ...queryParams } = params;
  return request<{ data: GeneratedAPI.StoryboardIntentAcceptanceResponse }>(
    `/api/storyboard-draft-sets/${param0}/approved-intents`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 GET /api/storyboard-exports/${param0} */
export async function getStoryboardExport(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.getStoryboardExportParams,
  options?: RequestOptions,
) {
  const { export_id: param0, ...queryParams } = params;
  return request<any>(`/api/storyboard-exports/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/storyboard-exports/${param0}/download */
export async function downloadStoryboardExport(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.downloadStoryboardExportParams,
  options?: RequestOptions,
) {
  const { export_id: param0, ...queryParams } = params;
  return request<string>(`/api/storyboard-exports/${param0}/download`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/workflow-runs */
export async function startWorkflowRun(
  body: GeneratedAPI.WorkflowStartRequest,
  options?: RequestOptions,
) {
  return request<any>("/api/workflow-runs", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/workflow-runs/${param0} */
export async function getWorkflowRun(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.getWorkflowRunParams,
  options?: RequestOptions,
) {
  const { workflow_run_id: param0, ...queryParams } = params;
  return request<GeneratedAPI.WorkflowRunEnvelope>(`/api/workflow-runs/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/workflow-runs/${param0}/controls */
export async function controlWorkflowRun(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.controlWorkflowRunParams,
  body: GeneratedAPI.WorkflowControlRequest,
  options?: RequestOptions,
) {
  const { workflow_run_id: param0, ...queryParams } = params;
  return request<any>(`/api/workflow-runs/${param0}/controls`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/workflow-runs/${param0}/reruns */
export async function rerunWorkflowFromNode(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.rerunWorkflowFromNodeParams,
  body: GeneratedAPI.WorkflowRerunRequest,
  options?: RequestOptions,
) {
  const { workflow_run_id: param0, ...queryParams } = params;
  return request<any>(`/api/workflow-runs/${param0}/reruns`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/workflow-runs/${param0}/story-analysis-recoveries */
export async function recoverStoryAnalysisShard(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.recoverStoryAnalysisShardParams,
  body: GeneratedAPI.StoryAnalysisRecoveryRequest,
  options?: RequestOptions,
) {
  const { workflow_run_id: param0, ...queryParams } = params;
  return request<any>(`/api/workflow-runs/${param0}/story-analysis-recoveries`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/workspaces */
export async function listWorkspaces(options?: RequestOptions) {
  return request<any>("/api/workspaces", {
    method: "GET",
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/workspaces */
export async function createWorkspace(options?: RequestOptions) {
  return request<any>("/api/workspaces", {
    method: "POST",
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/workspaces/${param0} */
export async function getWorkspace(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.getWorkspaceParams,
  options?: RequestOptions,
) {
  const { workspace_id: param0, ...queryParams } = params;
  return request<any>(`/api/workspaces/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 PATCH /api/workspaces/${param0} */
export async function updateWorkspace(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.updateWorkspaceParams,
  options?: RequestOptions,
) {
  const { workspace_id: param0, ...queryParams } = params;
  return request<any>(`/api/workspaces/${param0}`, {
    method: "PATCH",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/workspaces/${param0}/archive */
export async function archiveWorkspace(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.archiveWorkspaceParams,
  options?: RequestOptions,
) {
  const { workspace_id: param0, ...queryParams } = params;
  return request<any>(`/api/workspaces/${param0}/archive`, {
    method: "POST",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/workspaces/${param0}/restore */
export async function restoreWorkspace(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: GeneratedAPI.restoreWorkspaceParams,
  options?: RequestOptions,
) {
  const { workspace_id: param0, ...queryParams } = params;
  return request<any>(`/api/workspaces/${param0}/restore`, {
    method: "POST",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /healthz */
export async function health(options?: RequestOptions) {
  return request<any>("/healthz", {
    method: "GET",
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /openapi.json */
export async function openapi(options?: RequestOptions) {
  return request<any>("/openapi.json", {
    method: "GET",
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /readyz */
export async function readiness(options?: RequestOptions) {
  return request<any>("/readyz", {
    method: "GET",
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /version */
export async function version(options?: RequestOptions) {
  return request<any>("/version", {
    method: "GET",
    ...(options || {}),
  });
}
