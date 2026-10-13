// @ts-ignore
/* eslint-disable */
import { request } from "../../auth/request";

/** 此处后端没有提供注释 GET /api/admin/users */
export async function listAdminUsers(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CreationAPI.listAdminUsersParams,
  options?: import("../../auth/request").RequestOptions,
) {
  return request<CreationAPI.AdminUserPage>("/api/admin/users", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/admin/users */
export async function createAdminUser(
  body: CreationAPI.AdminCreateRequest,
  options?: import("../../auth/request").RequestOptions,
) {
  return request<CreationAPI.AdminMutationReceipt>("/api/admin/users", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 PUT /api/admin/users/${param0} */
export async function updateAdminUser(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CreationAPI.updateAdminUserParams,
  body: CreationAPI.AdminUpdateRequest,
  options?: import("../../auth/request").RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<CreationAPI.AdminMutationReceipt>(
    `/api/admin/users/${param0}`,
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

/** 此处后端没有提供注释 POST /api/admin/users/${param0}/disable */
export async function disableAdminUser(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CreationAPI.disableAdminUserParams,
  body: CreationAPI.AdminRevisionRequest,
  options?: import("../../auth/request").RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<CreationAPI.AdminMutationReceipt>(
    `/api/admin/users/${param0}/disable`,
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

/** 此处后端没有提供注释 POST /api/admin/users/${param0}/enable */
export async function enableAdminUser(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CreationAPI.enableAdminUserParams,
  body: CreationAPI.AdminRevisionRequest,
  options?: import("../../auth/request").RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<CreationAPI.AdminMutationReceipt>(
    `/api/admin/users/${param0}/enable`,
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

/** 此处后端没有提供注释 POST /api/admin/users/${param0}/password */
export async function resetAdminPassword(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CreationAPI.resetAdminPasswordParams,
  body: CreationAPI.AdminPasswordRequest,
  options?: import("../../auth/request").RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<CreationAPI.AdminMutationReceipt>(
    `/api/admin/users/${param0}/password`,
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

/** 此处后端没有提供注释 POST /api/admin/users/${param0}/unlock */
export async function unlockAdminUser(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CreationAPI.unlockAdminUserParams,
  body: CreationAPI.AdminRevisionRequest,
  options?: import("../../auth/request").RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<CreationAPI.AdminMutationReceipt>(
    `/api/admin/users/${param0}/unlock`,
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

/** 项目模型目录与参数表单 GET /api/models */
export async function listProjectModels(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CreationAPI.listProjectModelsParams,
  options?: import("../../auth/request").RequestOptions,
) {
  return request<CreationAPI.ModelPage>("/api/models", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** 任务详情与候选 GET /api/operations/${param0} */
export async function getOperation(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CreationAPI.getOperationParams,
  options?: import("../../auth/request").RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<CreationAPI.TaskDetail>(`/api/operations/${param0}`, {
    method: "GET",
    params: {
      ...queryParams,
    },
    ...(options || {}),
  });
}

/** 请求取消任务 POST /api/operations/${param0}/cancel */
export async function cancelOperation(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CreationAPI.cancelOperationParams,
  body: CreationAPI.ProjectRequest,
  options?: import("../../auth/request").RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<any>(`/api/operations/${param0}/cancel`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 确认单项报价并预留预算 POST /api/operations/${param0}/confirm */
export async function confirmOperationQuote(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CreationAPI.confirmOperationQuoteParams,
  body: CreationAPI.ProjectRequest,
  options?: import("../../auth/request").RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<CreationAPI.ConfirmResponse>(
    `/api/operations/${param0}/confirm`,
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

/** 此处后端没有提供注释 GET /api/projects/${param0}/bible/${param1} */
export async function listBibleEntries(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CreationAPI.listBibleEntriesParams,
  options?: import("../../auth/request").RequestOptions,
) {
  const { pid: param0, kind: param1, ...queryParams } = params;
  return request<CreationAPI.PageResponse>(
    `/api/projects/${param0}/bible/${param1}`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 POST /api/projects/${param0}/bible/${param1} */
export async function createBibleEntry(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CreationAPI.createBibleEntryParams,
  body: CreationAPI.ContentRequest,
  options?: import("../../auth/request").RequestOptions,
) {
  const { pid: param0, kind: param1, ...queryParams } = params;
  return request<CreationAPI.Receipt>(
    `/api/projects/${param0}/bible/${param1}`,
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

/** 此处后端没有提供注释 GET /api/projects/${param0}/bible/${param1}/${param2} */
export async function getBibleEntry(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CreationAPI.getBibleEntryParams,
  options?: import("../../auth/request").RequestOptions,
) {
  const { pid: param0, kind: param1, id: param2, ...queryParams } = params;
  return request<CreationAPI.Detail>(
    `/api/projects/${param0}/bible/${param1}/${param2}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 PUT /api/projects/${param0}/bible/${param1}/${param2} */
export async function updateBibleEntry(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CreationAPI.updateBibleEntryParams,
  body: CreationAPI.ContentRequest,
  options?: import("../../auth/request").RequestOptions,
) {
  const { pid: param0, kind: param1, id: param2, ...queryParams } = params;
  return request<CreationAPI.Receipt>(
    `/api/projects/${param0}/bible/${param1}/${param2}`,
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

/** 此处后端没有提供注释 DELETE /api/projects/${param0}/bible/${param1}/${param2} */
export async function deleteBibleEntry(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CreationAPI.deleteBibleEntryParams,
  body: CreationAPI.ControlRequest,
  options?: import("../../auth/request").RequestOptions,
) {
  const { pid: param0, kind: param1, id: param2, ...queryParams } = params;
  return request<CreationAPI.Receipt>(
    `/api/projects/${param0}/bible/${param1}/${param2}`,
    {
      method: "DELETE",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 POST /api/projects/${param0}/bible/${param1}/${param2}/confirm */
export async function confirmBibleEntry(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CreationAPI.confirmBibleEntryParams,
  body: CreationAPI.ControlRequest,
  options?: import("../../auth/request").RequestOptions,
) {
  const { pid: param0, kind: param1, id: param2, ...queryParams } = params;
  return request<CreationAPI.Receipt>(
    `/api/projects/${param0}/bible/${param1}/${param2}/confirm`,
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

/** 此处后端没有提供注释 POST /api/projects/${param0}/bible/${param1}/${param2}/looks */
export async function createBibleLook(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CreationAPI.createBibleLookParams,
  body: CreationAPI.LookRequest,
  options?: import("../../auth/request").RequestOptions,
) {
  const { pid: param0, kind: param1, id: param2, ...queryParams } = params;
  return request<CreationAPI.Receipt>(
    `/api/projects/${param0}/bible/${param1}/${param2}/looks`,
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

/** 此处后端没有提供注释 PUT /api/projects/${param0}/bible/${param1}/${param2}/looks/${param3}/references */
export async function replaceBibleReferences(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CreationAPI.replaceBibleReferencesParams,
  body: CreationAPI.ReferencesRequest,
  options?: import("../../auth/request").RequestOptions,
) {
  const {
    pid: param0,
    kind: param1,
    id: param2,
    look: param3,
    ...queryParams
  } = params;
  return request<CreationAPI.Receipt>(
    `/api/projects/${param0}/bible/${param1}/${param2}/looks/${param3}/references`,
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

/** 此处后端没有提供注释 POST /api/projects/${param0}/bible/${param1}/${param2}/restore */
export async function restoreBibleEntry(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CreationAPI.restoreBibleEntryParams,
  body: CreationAPI.ControlRequest,
  options?: import("../../auth/request").RequestOptions,
) {
  const { pid: param0, kind: param1, id: param2, ...queryParams } = params;
  return request<CreationAPI.Receipt>(
    `/api/projects/${param0}/bible/${param1}/${param2}/restore`,
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

/** 此处后端没有提供注释 GET /api/projects/${param0}/bible/${param1}/${param2}/versions */
export async function listBibleVersions(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CreationAPI.listBibleVersionsParams,
  options?: import("../../auth/request").RequestOptions,
) {
  const { pid: param0, kind: param1, id: param2, ...queryParams } = params;
  return request<CreationAPI.HistoryResponse>(
    `/api/projects/${param0}/bible/${param1}/${param2}/versions`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    },
  );
}

/** 自由生成报价 POST /api/projects/${param0}/free-operations */
export async function createFreeQuote(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CreationAPI.createFreeQuoteParams,
  body: CreationAPI.QuoteItemRequest,
  options?: import("../../auth/request").RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<CreationAPI.QuoteResponse>(
    `/api/projects/${param0}/free-operations`,
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

/** 项目可用媒体列表 GET /api/projects/${param0}/media */
export async function listMediaAssets(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CreationAPI.listMediaAssetsParams,
  options?: import("../../auth/request").RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<CreationAPI.AssetPage>(`/api/projects/${param0}/media`, {
    method: "GET",
    params: {
      ...queryParams,
    },
    ...(options || {}),
  });
}

/** 获取媒体预览 GET /api/projects/${param0}/media/${param1}/preview */
export async function getMediaPreview(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CreationAPI.getMediaPreviewParams,
  options?: import("../../auth/request").RequestOptions,
) {
  const { pid: param0, asset_id: param1, ...queryParams } = params;
  return request<CreationAPI.Preview>(
    `/api/projects/${param0}/media/${param1}/preview`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 上传本地已人工确认的媒体 POST /api/projects/${param0}/media/uploads */
export async function uploadMediaAsset(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CreationAPI.uploadMediaAssetParams,
  body: {
    /** 已确认本地内容、素材使用权限且不含需要授权的真人素材 */
    local_review_confirmed: boolean;
  },
  file?: File,
  options?: import("../../auth/request").RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  const formData = new FormData();

  if (file) {
    formData.append("file", file);
  }

  Object.keys(body).forEach((ele) => {
    const item = (body as any)[ele];

    if (item !== undefined && item !== null) {
      if (typeof item === "object" && !(item instanceof File)) {
        if (item instanceof Array) {
          item.forEach((f) => formData.append(ele, f || ""));
        } else {
          formData.append(
            ele,
            new Blob([JSON.stringify(item)], { type: "application/json" }),
          );
        }
      } else {
        formData.append(ele, item);
      }
    }
  });

  return request<CreationAPI.UploadResult>(
    `/api/projects/${param0}/media/uploads`,
    {
      method: "POST",
      params: { ...queryParams },
      data: formData,
      requestType: "form",
      ...(options || {}),
    },
  );
}

/** 项目任务中心 GET /api/projects/${param0}/operations */
export async function listProjectOperations(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CreationAPI.listProjectOperationsParams,
  options?: import("../../auth/request").RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<CreationAPI.TaskPageResponse>(
    `/api/projects/${param0}/operations`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    },
  );
}
