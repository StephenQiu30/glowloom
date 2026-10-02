// @ts-ignore
/* eslint-disable */
import { request } from "@/lib/request";

/** 个人或项目完整素材库分页 GET /api/media/library */
export async function listMediaLibrary(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listMediaLibraryParams,
  options?: import("@/lib/request").RequestOptions,
) {
  return request<API.githubComStephenQiu30LanverseBackendInternalMediaApplicationLibraryPage>(
    "/api/media/library",
    {
      method: "GET",
      params: {
        ...params,
      },
      ...(options || {}),
    },
  );
}

/** 修改素材库目录或元数据 POST /api/media/library/commands */
export async function applyMediaLibraryCommand(
  body: API.githubComStephenQiu30LanverseBackendInternalMediaApplicationLibraryCommand,
  options?: import("@/lib/request").RequestOptions,
) {
  return request<API.githubComStephenQiu30LanverseBackendInternalMediaApplicationLibraryReceipt>(
    "/api/media/library/commands",
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

/** 当前素材库条目详情 GET /api/media/library/items/${param0} */
export async function getMediaLibraryItem(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getMediaLibraryItemParams,
  options?: import("@/lib/request").RequestOptions,
) {
  const { item_id: param0, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalMediaApplicationLibraryItemDetail>(
    `/api/media/library/items/${param0}`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    },
  );
}

/** 下载素材库原件附件 GET /api/media/library/items/${param0}/download */
export async function downloadLibraryMediaAsset(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.downloadLibraryMediaAssetParams,
  options?: import("@/lib/request").RequestOptions,
) {
  const { item_id: param0, ...queryParams } = params;
  return request<string>(`/api/media/library/items/${param0}/download`, {
    method: "GET",
    params: {
      ...queryParams,
    },
    ...(options || {}),
  });
}

/** 获取个人或项目素材库媒体预览 GET /api/media/library/items/${param0}/preview */
export async function previewLibraryMediaAsset(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.previewLibraryMediaAssetParams,
  options?: import("@/lib/request").RequestOptions,
) {
  const { item_id: param0, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalMediaApplicationLibraryMediaPreview>(
    `/api/media/library/items/${param0}/preview`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    },
  );
}

/** 下载完整素材库ZIP GET /api/media/library/packages/export */
export async function exportMediaLibraryPackage(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.exportMediaLibraryPackageParams,
  options?: import("@/lib/request").RequestOptions,
) {
  return request<string>("/api/media/library/packages/export", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** 素材包导入历史分页 GET /api/media/library/packages/imports */
export async function listMediaLibraryPackages(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listMediaLibraryPackagesParams,
  options?: import("@/lib/request").RequestOptions,
) {
  return request<API.githubComStephenQiu30LanverseBackendInternalMediaApplicationPackagePage>(
    "/api/media/library/packages/imports",
    {
      method: "GET",
      params: {
        ...params,
      },
      ...(options || {}),
    },
  );
}

/** 导入完整素材包及目录与正文 POST /api/media/library/packages/imports */
export async function importMediaLibraryPackage(
  body: {
    /** 首个part：PackageImportRequest闭合JSON，含scope、expected_revision、expected_project_revision、local_review_confirmed */
    request: string;
  },
  file?: File,
  options?: import("@/lib/request").RequestOptions,
) {
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

  return request<any>("/api/media/library/packages/imports", {
    method: "POST",
    data: formData,
    requestType: "form",
    ...(options || {}),
  });
}

/** 当前素材包导入状态 GET /api/media/library/packages/imports/${param0} */
export async function getMediaLibraryPackage(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getMediaLibraryPackageParams,
  options?: import("@/lib/request").RequestOptions,
) {
  const { job_id: param0, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalMediaApplicationPackageJob>(
    `/api/media/library/packages/imports/${param0}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 取消未发布素材包并核验清理 POST /api/media/library/packages/imports/${param0}/cancel */
export async function cancelMediaLibraryPackage(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.cancelMediaLibraryPackageParams,
  body: API.githubComStephenQiu30LanverseBackendInternalMediaApplicationPackageControl,
  options?: import("@/lib/request").RequestOptions,
) {
  const { job_id: param0, ...queryParams } = params;
  return request<any>(`/api/media/library/packages/imports/${param0}/cancel`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 核验并恢复素材包导入 POST /api/media/library/packages/imports/${param0}/reconcile */
export async function reconcileMediaLibraryPackage(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.reconcileMediaLibraryPackageParams,
  body: API.githubComStephenQiu30LanverseBackendInternalMediaApplicationPackageControl,
  options?: import("@/lib/request").RequestOptions,
) {
  const { job_id: param0, ...queryParams } = params;
  return request<any>(
    `/api/media/library/packages/imports/${param0}/reconcile`,
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

/** 永久清理记录分页 GET /api/media/library/purges */
export async function listMediaPurges(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listMediaPurgesParams,
  options?: import("@/lib/request").RequestOptions,
) {
  return request<API.internalMediaAdapterHttpPurgePage>(
    "/api/media/library/purges",
    {
      method: "GET",
      params: {
        ...params,
      },
      ...(options || {}),
    },
  );
}

/** 明确确认后永久清理回收素材 POST /api/media/library/purges */
export async function createMediaPurge(
  body: API.githubComStephenQiu30LanverseBackendInternalMediaApplicationPurgeInput,
  options?: import("@/lib/request").RequestOptions,
) {
  return request<any>("/api/media/library/purges", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 永久清理状态与逐行结果 GET /api/media/library/purges/${param0} */
export async function getMediaPurge(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getMediaPurgeParams,
  options?: import("@/lib/request").RequestOptions,
) {
  const { job_id: param0, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalMediaDomainPurgeJob>(
    `/api/media/library/purges/${param0}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 取消永久清理 POST /api/media/library/purges/${param0}/cancel */
export async function cancelMediaPurge(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.cancelMediaPurgeParams,
  body: API.internalMediaAdapterHttpPurgeControlRequest,
  options?: import("@/lib/request").RequestOptions,
) {
  const { job_id: param0, ...queryParams } = params;
  return request<any>(`/api/media/library/purges/${param0}/cancel`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 对账永久清理未知结果 POST /api/media/library/purges/${param0}/reconcile */
export async function reconcileMediaPurge(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.reconcileMediaPurgeParams,
  body: API.internalMediaAdapterHttpPurgeControlRequest,
  options?: import("@/lib/request").RequestOptions,
) {
  const { job_id: param0, ...queryParams } = params;
  return request<any>(`/api/media/library/purges/${param0}/reconcile`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 当前素材库实际私有存储用量 GET /api/media/library/storage-usage */
export async function getMediaLibraryStorageUsage(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getMediaLibraryStorageUsageParams,
  options?: import("@/lib/request").RequestOptions,
) {
  return request<API.githubComStephenQiu30LanverseBackendInternalMediaApplicationStorageUsage>(
    "/api/media/library/storage-usage",
    {
      method: "GET",
      params: {
        ...params,
      },
      ...(options || {}),
    },
  );
}

/** 素材转移记录分页 GET /api/media/library/transfers */
export async function listMediaTransfers(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listMediaTransfersParams,
  options?: import("@/lib/request").RequestOptions,
) {
  return request<API.internalMediaAdapterHttpTransferPage>(
    "/api/media/library/transfers",
    {
      method: "GET",
      params: {
        ...params,
      },
      ...(options || {}),
    },
  );
}

/** 在个人库与项目库间独立转移素材 POST /api/media/library/transfers */
export async function createMediaTransfer(
  body: API.githubComStephenQiu30LanverseBackendInternalMediaApplicationTransferInput,
  options?: import("@/lib/request").RequestOptions,
) {
  return request<any>("/api/media/library/transfers", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 素材转移状态与逐行结果 GET /api/media/library/transfers/${param0} */
export async function getMediaTransfer(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getMediaTransferParams,
  options?: import("@/lib/request").RequestOptions,
) {
  const { job_id: param0, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalMediaDomainTransferJob>(
    `/api/media/library/transfers/${param0}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 取消素材转移 POST /api/media/library/transfers/${param0}/cancel */
export async function cancelMediaTransfer(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.cancelMediaTransferParams,
  body: API.internalMediaAdapterHttpTransferControlRequest,
  options?: import("@/lib/request").RequestOptions,
) {
  const { job_id: param0, ...queryParams } = params;
  return request<any>(`/api/media/library/transfers/${param0}/cancel`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 对账素材转移未知结果 POST /api/media/library/transfers/${param0}/reconcile */
export async function reconcileMediaTransfer(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.reconcileMediaTransferParams,
  body: API.internalMediaAdapterHttpTransferControlRequest,
  options?: import("@/lib/request").RequestOptions,
) {
  const { job_id: param0, ...queryParams } = params;
  return request<any>(`/api/media/library/transfers/${param0}/reconcile`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 重试已清理失败行 POST /api/media/library/transfers/${param0}/retry */
export async function retryMediaTransfer(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.retryMediaTransferParams,
  body: API.internalMediaAdapterHttpTransferControlRequest,
  options?: import("@/lib/request").RequestOptions,
) {
  const { job_id: param0, ...queryParams } = params;
  return request<any>(`/api/media/library/transfers/${param0}/retry`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 上传人工确认的个人素材 POST /api/media/library/uploads */
export async function uploadPersonalMediaAsset(
  body: {
    /** 已确认内容和使用权限且不含需要授权的真人 */
    local_review_confirmed: boolean;
  },
  file?: File,
  options?: import("@/lib/request").RequestOptions,
) {
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

  return request<API.githubComStephenQiu30LanverseBackendInternalMediaApplicationPersonalUploadResult>(
    "/api/media/library/uploads",
    {
      method: "POST",
      data: formData,
      requestType: "form",
      ...(options || {}),
    },
  );
}

/** 项目可用媒体列表 GET /api/projects/${param0}/media */
export async function listMediaAssets(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listMediaAssetsParams,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalMediaApplicationAssetPage>(
    `/api/projects/${param0}/media`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    },
  );
}

/** 下载正式文档原件 GET /api/projects/${param0}/media/${param1}/download */
export async function downloadDocumentAsset(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.downloadDocumentAssetParams,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, asset_id: param1, ...queryParams } = params;
  return request<string>(`/api/projects/${param0}/media/${param1}/download`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 获取媒体预览 GET /api/projects/${param0}/media/${param1}/preview */
export async function getMediaPreview(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getMediaPreviewParams,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, asset_id: param1, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalMediaApplicationPreview>(
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
  params: API.uploadMediaAssetParams,
  body: {
    /** 已确认本地内容、素材使用权限且不含需要授权的真人素材 */
    local_review_confirmed: boolean;
  },
  file?: File,
  options?: import("@/lib/request").RequestOptions,
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

  return request<API.githubComStephenQiu30LanverseBackendInternalMediaApplicationUploadResult>(
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
