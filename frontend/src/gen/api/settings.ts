// @ts-ignore
/* eslint-disable */
import { request } from "@/lib/request";

/** 模型能力声明目录 GET /api/admin/capabilities */
export async function listAdminCapabilities(
  options?: import("@/lib/request").RequestOptions,
) {
  return request<
    API.githubComStephenQiu30LanverseBackendInternalCatalogApplicationCapabilitySummary[]
  >("/api/admin/capabilities", {
    method: "GET",
    ...(options || {}),
  });
}

/** 管理员模型目录 GET /api/admin/models */
export async function listAdminModels(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listAdminModelsParams,
  options?: import("@/lib/request").RequestOptions,
) {
  return request<API.internalCatalogAdapterHttpAdminModelPage>(
    "/api/admin/models",
    {
      method: "GET",
      params: {
        ...params,
      },
      ...(options || {}),
    },
  );
}

/** 创建模型 POST /api/admin/models */
export async function createAdminModel(
  body: API.internalCatalogAdapterHttpCreateModelRequest,
  options?: import("@/lib/request").RequestOptions,
) {
  return request<API.githubComStephenQiu30LanverseBackendInternalCatalogApplicationCreatedModel>(
    "/api/admin/models",
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

/** 模型配置与价格版本详情 GET /api/admin/models/${param0} */
export async function getAdminModel(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getAdminModelParams,
  options?: import("@/lib/request").RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalCatalogApplicationAdminModelDetail>(
    `/api/admin/models/${param0}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 发布模型价格版本 POST /api/admin/models/${param0}/prices */
export async function publishAdminModelPrice(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.publishAdminModelPriceParams,
  body: API.internalCatalogAdapterHttpPublishPriceRequest,
  options?: import("@/lib/request").RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalCatalogApplicationPublishedPriceRule>(
    `/api/admin/models/${param0}/prices`,
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

/** 启用或禁用模型 PATCH /api/admin/models/${param0}/status */
export async function setAdminModelStatus(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.setAdminModelStatusParams,
  body: API.internalCatalogAdapterHttpSetModelStatusRequest,
  options?: import("@/lib/request").RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalCatalogApplicationChangedModelStatus>(
    `/api/admin/models/${param0}/status`,
    {
      method: "PATCH",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 发布模型配置版本 POST /api/admin/models/${param0}/versions */
export async function publishAdminModelVersion(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.publishAdminModelVersionParams,
  body: API.internalCatalogAdapterHttpPublishVersionRequest,
  options?: import("@/lib/request").RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalCatalogApplicationPublishedModelVersion>(
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

/** 管理员渠道列表 GET /api/admin/providers */
export async function listAdminProviders(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listAdminProvidersParams,
  options?: import("@/lib/request").RequestOptions,
) {
  return request<API.internalCatalogAdapterHttpProviderPage>(
    "/api/admin/providers",
    {
      method: "GET",
      params: {
        ...params,
      },
      ...(options || {}),
    },
  );
}

/** 创建渠道 POST /api/admin/providers */
export async function createAdminProvider(
  body: API.internalCatalogAdapterHttpCreateProviderRequest,
  options?: import("@/lib/request").RequestOptions,
) {
  return request<API.githubComStephenQiu30LanverseBackendInternalCatalogApplicationCreatedProvider>(
    "/api/admin/providers",
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

/** 渠道详情与凭据字段 GET /api/admin/providers/${param0} */
export async function getAdminProvider(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getAdminProviderParams,
  options?: import("@/lib/request").RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalCatalogApplicationProviderDetail>(
    `/api/admin/providers/${param0}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 更新或禁用渠道 PATCH /api/admin/providers/${param0} */
export async function updateAdminProvider(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.updateAdminProviderParams,
  body: API.internalCatalogAdapterHttpUpdateProviderRequest,
  options?: import("@/lib/request").RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalCatalogApplicationCreatedProvider>(
    `/api/admin/providers/${param0}`,
    {
      method: "PATCH",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 替换渠道凭据 PUT /api/admin/providers/${param0}/credentials */
export async function setAdminCredential(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.setAdminCredentialParams,
  body: API.internalCatalogAdapterHttpSetCredentialRequest,
  options?: import("@/lib/request").RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalCatalogApplicationSavedCredential>(
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
  options?: import("@/lib/request").RequestOptions,
) {
  const { id: param0, credential_id: param1, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalCatalogApplicationSavedCredential>(
    `/api/admin/providers/${param0}/credentials/${param1}/disable`,
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
  options?: import("@/lib/request").RequestOptions,
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

/** 项目默认模型 GET /api/projects/${param0}/model-defaults */
export async function getProjectModelDefaults(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getProjectModelDefaultsParams,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalWorkspaceApplicationModelDefaults>(
    `/api/projects/${param0}/model-defaults`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 保存项目默认模型 PATCH /api/projects/${param0}/model-defaults */
export async function saveProjectModelDefaults(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.saveProjectModelDefaultsParams,
  body: API.internalWorkspaceAdapterHttpModelDefaultsRequest,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalWorkspaceApplicationModelDefaults>(
    `/api/projects/${param0}/model-defaults`,
    {
      method: "PATCH",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 工作区提示词偏好 GET /api/settings/prompt-preferences */
export async function listPromptPreferences(
  options?: import("@/lib/request").RequestOptions,
) {
  return request<API.internalPromptAdapterHttpPreferencesResponse>(
    "/api/settings/prompt-preferences",
    {
      method: "GET",
      ...(options || {}),
    },
  );
}

/** 保存或恢复工作区提示词偏好 PUT /api/settings/prompt-preferences/${param0} */
export async function savePromptPreference(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.savePromptPreferenceParams,
  body: API.internalPromptAdapterHttpSaveRequest,
  options?: import("@/lib/request").RequestOptions,
) {
  const { operation: param0, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalPromptDomainCustomization>(
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
