// @ts-ignore
/* eslint-disable */
import { request } from "@/lib/request";

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
