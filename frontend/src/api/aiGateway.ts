// @ts-ignore
/* eslint-disable */
import { request } from "@/lib/request";
import type { RequestOptions } from "@/lib/request";

/** 按方舟公开规则估算视频费用 POST /api/ai/admin/ark-video-estimate */
export async function estimateArkVideo(
  body: API.ArkVideoEstimateInput,
  options?: RequestOptions,
) {
  return request<API.ArkVideoEstimate>("/api/ai/admin/ark-video-estimate", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 设置项目积分上限 PUT /api/ai/admin/budgets */
export async function setAiProjectBudget(
  body: Record<string, any>,
  options?: RequestOptions,
) {
  return request<any>("/api/ai/admin/budgets", {
    method: "PUT",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 读取模型、路由与脱敏账号池 GET /api/ai/admin/configuration */
export async function readAiConfiguration(options?: RequestOptions) {
  return request<Record<string, any>>("/api/ai/admin/configuration", {
    method: "GET",
    ...(options || {}),
  });
}

/** 调整账号可用状态 PUT /api/ai/admin/credentials/${param0} */
export async function changeAiCredentialState(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.changeAiCredentialStateParams,
  body: Record<string, any>,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<any>(`/api/ai/admin/credentials/${param0}`, {
    method: "PUT",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 提交积分赠送单 POST /api/ai/admin/credits */
export async function submitAiCreditGrant(
  body: API.CreditGrantInput,
  options?: RequestOptions,
) {
  return request<API.CreditGrantOrder>("/api/ai/admin/credits", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 复核并处理积分赠送单 POST /api/ai/admin/credits/${param0}/review */
export async function reviewAiCreditGrant(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.reviewAiCreditGrantParams,
  body: API.CreditGrantReview,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.CreditGrantOrder>(
    `/api/ai/admin/credits/${param0}/review`,
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

/** 保存未启用的模型预设与销售价 POST /api/ai/admin/models */
export async function prepareAiModel(
  body: Record<string, any>,
  options?: RequestOptions,
) {
  return request<any>("/api/ai/admin/models", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 读取候选的验证计划或回执 GET /api/ai/admin/onboarding/${param0} */
export async function getAiOnboarding(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getAiOnboardingParams,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.OnboardingReview>(`/api/ai/admin/onboarding/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 回退已启用候选 POST /api/ai/admin/onboarding/${param0}/rollback */
export async function rollbackAiOnboarding(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.rollbackAiOnboardingParams,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<any>(`/api/ai/admin/onboarding/${param0}/rollback`, {
    method: "POST",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 确认预算并提交密钥验证 POST /api/ai/admin/onboarding/${param0}/verify */
export async function verifyAiOnboarding(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.verifyAiOnboardingParams,
  body: Record<string, any>,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<any>(`/api/ai/admin/onboarding/${param0}/verify`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 依审核预设准备无密钥验证候选 POST /api/ai/admin/onboarding/preset */
export async function prepareAiOnboardingPreset(
  body: API.QuickOnboardingInput,
  options?: RequestOptions,
) {
  return request<API.OnboardingReview>("/api/ai/admin/onboarding/preset", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 按原供应商证据处理未知调用 POST /api/ai/admin/reconciliations */
export async function reconcileAiRequest(
  body: Record<string, any>,
  options?: RequestOptions,
) {
  return request<Record<string, any>>("/api/ai/admin/reconciliations", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 保存采购路由与验证计划 POST /api/ai/admin/routes */
export async function prepareAiRoute(
  body: Record<string, any>,
  options?: RequestOptions,
) {
  return request<API.OnboardingReview>("/api/ai/admin/routes", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 删除选定的供应商渠道 POST /api/ai/admin/routes/delete */
export async function deleteAiRoutes(
  body: API.DeleteRoutesInput,
  options?: RequestOptions,
) {
  return request<any>("/api/ai/admin/routes/delete", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 读取统一模型账单 GET /api/ai/bills */
export async function listAiBills(options?: RequestOptions) {
  return request<Record<string, any>>("/api/ai/bills", {
    method: "GET",
    ...(options || {}),
  });
}

/** 签发限定模型和预算的调用凭据 POST /api/ai/grants */
export async function issueAiIntegrationGrant(
  body: Record<string, any>,
  options?: RequestOptions,
) {
  return request<any>("/api/ai/grants", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 撤销调用凭据 DELETE /api/ai/grants/${param0} */
export async function revokeAiIntegrationGrant(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.revokeAiIntegrationGrantParams,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<any>(`/api/ai/grants/${param0}`, {
    method: "DELETE",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 读取原调用和结算事实 GET /api/ai/requests/${param0} */
export async function readAiRequest(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.readAiRequestParams,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<Record<string, any>>(`/api/ai/requests/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** OpenAI Chat 原生调用 POST /api/ai/v1/chat/completions */
export async function createAiChatCompletion(
  body: Record<string, any>,
  options?: RequestOptions,
) {
  return request<Record<string, any>>("/api/ai/v1/chat/completions", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** Anthropic Messages 原生调用 POST /api/ai/v1/messages */
export async function createAiMessage(
  body: Record<string, any>,
  options?: RequestOptions,
) {
  return request<Record<string, any>>("/api/ai/v1/messages", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 列出调用凭据允许的原生协议模型 GET /api/ai/v1/models */
export async function listAiIntegrationModels(options?: RequestOptions) {
  return request<Record<string, any>>("/api/ai/v1/models", {
    method: "GET",
    ...(options || {}),
  });
}

/** OpenAI Responses 原生调用 POST /api/ai/v1/responses */
export async function createAiResponse(
  body: Record<string, any>,
  options?: RequestOptions,
) {
  return request<Record<string, any>>("/api/ai/v1/responses", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 读取统一积分余额 GET /api/ai/wallet */
export async function readAiWallet(options?: RequestOptions) {
  return request<Record<string, any>>("/api/ai/wallet", {
    method: "GET",
    ...(options || {}),
  });
}
