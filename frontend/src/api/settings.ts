// @ts-ignore
/* eslint-disable */
import { request } from "@/lib/request";
import type { RequestOptions } from "@/lib/request";

/** Start development AI trial POST /api/admin/ai-trials */
export async function startAiTrial(
  body: API.TrialRequest,
  options?: RequestOptions,
) {
  return request<any>("/api/admin/ai-trials", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** Read development AI trial GET /api/admin/ai-trials/${param0} */
export async function getAiTrial(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getAiTrialParams,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.TrialState>(`/api/admin/ai-trials/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** Read development AI artifact GET /api/admin/ai-trials/${param0}/artifacts/${param1} */
export async function getAiTrialArtifact(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getAiTrialArtifactParams,
  options?: RequestOptions,
) {
  const { id: param0, sequence: param1, ...queryParams } = params;
  return request<Blob>(`/api/admin/ai-trials/${param0}/artifacts/${param1}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || { responseType: "blob" }),
  });
}

/** Stop development AI trial POST /api/admin/ai-trials/${param0}/cancel */
export async function cancelAiTrial(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.cancelAiTrialParams,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<any>(`/api/admin/ai-trials/${param0}/cancel`, {
    method: "POST",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** Recover development AI result POST /api/admin/ai-trials/${param0}/recover */
export async function recoverAiTrial(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.recoverAiTrialParams,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<any>(`/api/admin/ai-trials/${param0}/recover`, {
    method: "POST",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** Continue development Codex chat POST /api/admin/ai-trials/${param0}/turns */
export async function continueAiTrial(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.continueAiTrialParams,
  body: API.TrialMessage,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<any>(`/api/admin/ai-trials/${param0}/turns`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 模型能力声明目录 GET /api/admin/capabilities */
export async function listAdminCapabilities(options?: RequestOptions) {
  return request<API.CapabilitySummary[]>("/api/admin/capabilities", {
    method: "GET",
    ...(options || {}),
  });
}

/** 平台能力默认模型 GET /api/admin/capability-defaults */
export async function listCapabilityDefaults(options?: RequestOptions) {
  return request<API.DefaultPage>("/api/admin/capability-defaults", {
    method: "GET",
    ...(options || {}),
  });
}

/** 查询配置请求回执 GET /api/admin/config-requests/${param0} */
export async function getConfigReceipt(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getConfigReceiptParams,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.ConfigReceipt>(`/api/admin/config-requests/${param0}`, {
    method: "GET",
    params: {
      ...queryParams,
    },
    ...(options || {}),
  });
}

/** 激活已验证模型发布 POST /api/admin/model-releases/${param0}/activate */
export async function activateModelRelease(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.activateModelReleaseParams,
  body: API.ActivateReleaseInput,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.ModelRelease>(
    `/api/admin/model-releases/${param0}/activate`,
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

/** 准备模型试运行报价（不调用供应商） POST /api/admin/model-releases/${param0}/trial-quotes */
export async function createProviderTrialQuote(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.createProviderTrialQuoteParams,
  body: API.ProviderTrialQuote,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.QuoteResponse>(
    `/api/admin/model-releases/${param0}/trial-quotes`,
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

/** 管理员模型目录 GET /api/admin/models */
export async function listAdminModels(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listAdminModelsParams,
  options?: RequestOptions,
) {
  return request<API.AdminModelPage>("/api/admin/models", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** 创建模型 POST /api/admin/models */
export async function createAdminModel(
  body: API.CreateModelRequest,
  options?: RequestOptions,
) {
  return request<API.CreatedModel>("/api/admin/models", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 模型配置与价格版本详情 GET /api/admin/models/${param0} */
export async function getAdminModel(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getAdminModelParams,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.AdminModelDetail>(`/api/admin/models/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 发布模型价格版本 POST /api/admin/models/${param0}/prices */
export async function publishAdminModelPrice(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.publishAdminModelPriceParams,
  body: API.PublishPriceRequest,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.PublishedPriceRule>(`/api/admin/models/${param0}/prices`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 模型发布历史 GET /api/admin/models/${param0}/releases */
export async function listModelReleases(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listModelReleasesParams,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.ReleasePage>(`/api/admin/models/${param0}/releases`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 创建待验证模型发布 POST /api/admin/models/${param0}/releases */
export async function createModelRelease(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.createModelReleaseParams,
  body: API.CreateReleaseInput,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.ModelRelease>(`/api/admin/models/${param0}/releases`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 启用或禁用模型 PATCH /api/admin/models/${param0}/status */
export async function setAdminModelStatus(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.setAdminModelStatusParams,
  body: API.SetModelStatusRequest,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.ChangedModelStatus>(`/api/admin/models/${param0}/status`, {
    method: "PATCH",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 发布模型配置版本 POST /api/admin/models/${param0}/versions */
export async function publishAdminModelVersion(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.publishAdminModelVersionParams,
  body: API.PublishVersionRequest,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.PublishedModelVersion>(
    `/api/admin/models/${param0}/versions`,
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

/** 连接检查与候选模型 GET /api/admin/provider-checks/${param0} */
export async function getProviderCheck(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getProviderCheckParams,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.ProviderCheck>(`/api/admin/provider-checks/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 平台预设列表 GET /api/admin/provider-presets */
export async function listProviderPresets(options?: RequestOptions) {
  return request<API.PresetPage>("/api/admin/provider-presets", {
    method: "GET",
    ...(options || {}),
  });
}

/** 保存自定义预设 POST /api/admin/provider-presets */
export async function createProviderPreset(
  body: API.SavePresetInput,
  options?: RequestOptions,
) {
  return request<API.Preset>("/api/admin/provider-presets", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 从预设创建禁用连接 POST /api/admin/provider-presets/${param0}/connections */
export async function connectProviderPreset(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.connectProviderPresetParams,
  body: API.ConnectPresetInput,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.PresetConnection>(
    `/api/admin/provider-presets/${param0}/connections`,
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

/** 导出无凭据预设 GET /api/admin/provider-presets/${param0}/export */
export async function exportProviderPreset(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.exportProviderPresetParams,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.PresetDocument>(
    `/api/admin/provider-presets/${param0}/export`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 更新预设生命周期 PATCH /api/admin/provider-presets/${param0}/state */
export async function updateProviderPresetState(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.updateProviderPresetStateParams,
  body: API.PresetStateInput,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.Preset>(`/api/admin/provider-presets/${param0}/state`, {
    method: "PATCH",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 创建预设版本 POST /api/admin/provider-presets/${param0}/versions */
export async function publishProviderPresetVersion(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.publishProviderPresetVersionParams,
  body: API.SavePresetInput,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.Preset>(`/api/admin/provider-presets/${param0}/versions`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 预览预设导入 POST /api/admin/provider-presets/import-preview */
export async function previewProviderPreset(
  body: API.PresetImportRequest,
  options?: RequestOptions,
) {
  return request<API.PresetPreview>(
    "/api/admin/provider-presets/import-preview",
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      data: body,
      ...(options || {}),
    },
  );
}

/** 管理员渠道列表 GET /api/admin/providers */
export async function listAdminProviders(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listAdminProvidersParams,
  options?: RequestOptions,
) {
  return request<API.ProviderPage>("/api/admin/providers", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** 创建渠道 POST /api/admin/providers */
export async function createAdminProvider(
  body: API.CreateProviderRequest,
  options?: RequestOptions,
) {
  return request<API.CreatedProvider>("/api/admin/providers", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 渠道详情与凭据字段 GET /api/admin/providers/${param0} */
export async function getAdminProvider(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getAdminProviderParams,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.ProviderDetail>(`/api/admin/providers/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 更新或禁用渠道 PATCH /api/admin/providers/${param0} */
export async function updateAdminProvider(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.updateAdminProviderParams,
  body: API.UpdateProviderRequest,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.CreatedProvider>(`/api/admin/providers/${param0}`, {
    method: "PATCH",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 应用已审阅的连接配置 POST /api/admin/providers/${param0}/apply-preset */
export async function applyProviderPreset(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.applyProviderPresetParams,
  body: API.ApplyConfigInput,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.ConnectionConfig>(
    `/api/admin/providers/${param0}/apply-preset`,
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

/** 读取连接配置快照 GET /api/admin/providers/${param0}/config */
export async function getProviderConfig(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getProviderConfigParams,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.ConnectionConfig>(
    `/api/admin/providers/${param0}/config`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 平台 Key 轮换记录 GET /api/admin/providers/${param0}/credentials */
export async function listProviderCredentials(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listProviderCredentialsParams,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.ProviderCredentialSummary[]>(
    `/api/admin/providers/${param0}/credentials`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 替换渠道凭据 PUT /api/admin/providers/${param0}/credentials */
export async function setAdminCredential(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.setAdminCredentialParams,
  body: API.SetCredentialRequest,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.SavedCredential>(
    `/api/admin/providers/${param0}/credentials`,
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

/** 禁用渠道当前凭据 POST /api/admin/providers/${param0}/credentials/${param1}/disable */
export async function disableAdminCredential(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.disableAdminCredentialParams,
  options?: RequestOptions,
) {
  const { id: param0, credential_id: param1, ...queryParams } = params;
  return request<API.SavedCredential>(
    `/api/admin/providers/${param0}/credentials/${param1}/disable`,
    {
      method: "POST",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 验证一个所选模型 POST /api/admin/providers/${param0}/credentials/${param1}/models/${param2}/test */
export async function testProviderOnboardingModel(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.testProviderOnboardingModelParams,
  options?: RequestOptions,
) {
  const {
    id: param0,
    credential_id: param1,
    model_id: param2,
    ...queryParams
  } = params;
  return request<any>(
    `/api/admin/providers/${param0}/credentials/${param1}/models/${param2}/test`,
    {
      method: "POST",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 请求测试渠道凭据 POST /api/admin/providers/${param0}/credentials/${param1}/test */
export async function testAdminCredential(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.testAdminCredentialParams,
  options?: RequestOptions,
) {
  const { id: param0, credential_id: param1, ...queryParams } = params;
  return request<any>(
    `/api/admin/providers/${param0}/credentials/${param1}/test`,
    {
      method: "POST",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 读取模型预设接入进度 GET /api/admin/providers/${param0}/onboarding */
export async function getProviderOnboarding(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getProviderOnboardingParams,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.ProviderOnboardingState>(
    `/api/admin/providers/${param0}/onboarding`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 验证后启用所选模型 POST /api/admin/providers/${param0}/onboarding/activate */
export async function activateProviderOnboarding(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.activateProviderOnboardingParams,
  body: API.ActivateOnboardingInput,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.ProviderOnboardingState>(
    `/api/admin/providers/${param0}/onboarding/activate`,
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

/** 预览连接预设升级 POST /api/admin/providers/${param0}/upgrade-preview */
export async function previewProviderUpgrade(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.previewProviderUpgradeParams,
  body: API.ConfigUpgradeInput,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.ConfigUpgradePreview>(
    `/api/admin/providers/${param0}/upgrade-preview`,
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

/** 项目默认模型 GET /api/projects/${param0}/model-defaults */
export async function getProjectModelDefaults(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getProjectModelDefaultsParams,
  options?: RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.ModelDefaults>(`/api/projects/${param0}/model-defaults`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 保存项目默认模型 PATCH /api/projects/${param0}/model-defaults */
export async function saveProjectModelDefaults(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.saveProjectModelDefaultsParams,
  body: API.ModelDefaultsRequest,
  options?: RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.ModelDefaults>(`/api/projects/${param0}/model-defaults`, {
    method: "PATCH",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 工作区提示词偏好 GET /api/settings/prompt-preferences */
export async function listPromptPreferences(options?: RequestOptions) {
  return request<API.PreferencesResponse>("/api/settings/prompt-preferences", {
    method: "GET",
    ...(options || {}),
  });
}

/** 保存或恢复工作区提示词偏好 PUT /api/settings/prompt-preferences/${param0} */
export async function savePromptPreference(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.savePromptPreferenceParams,
  body: API.SaveRequest,
  options?: RequestOptions,
) {
  const { operation: param0, ...queryParams } = params;
  return request<API.Customization>(
    `/api/settings/prompt-preferences/${param0}`,
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
