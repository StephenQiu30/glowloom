// @ts-ignore
/* eslint-disable */
import { request } from "../../auth/request";

/** 个人或项目完整素材库分页 GET /api/media/library */
export async function listMediaLibrary(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: MediaAPI.listMediaLibraryParams,
  options?: import("../../auth/request").RequestOptions,
) {
  return request<MediaAPI.LibraryPage>("/api/media/library", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** 修改素材库目录或元数据 POST /api/media/library/commands */
export async function applyMediaLibraryCommand(
  body: MediaAPI.LibraryCommand,
  options?: import("../../auth/request").RequestOptions,
) {
  return request<MediaAPI.LibraryReceipt>("/api/media/library/commands", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 当前素材库条目详情 GET /api/media/library/items/${param0} */
export async function getMediaLibraryItem(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: MediaAPI.getMediaLibraryItemParams,
  options?: import("../../auth/request").RequestOptions,
) {
  const { item_id: param0, ...queryParams } = params;
  return request<MediaAPI.LibraryItemDetail>(
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

/** 获取个人或项目素材库媒体预览 GET /api/media/library/items/${param0}/preview */
export async function previewLibraryMediaAsset(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: MediaAPI.previewLibraryMediaAssetParams,
  options?: import("../../auth/request").RequestOptions,
) {
  const { item_id: param0, ...queryParams } = params;
  return request<MediaAPI.LibraryMediaPreview>(
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

/** 上传人工确认的个人素材 POST /api/media/library/uploads */
export async function uploadPersonalMediaAsset(
  body: {
    /** 已确认内容和使用权限且不含需要授权的真人 */
    local_review_confirmed: boolean;
  },
  file?: File,
  options?: import("../../auth/request").RequestOptions,
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

  return request<MediaAPI.PersonalUploadResult>("/api/media/library/uploads", {
    method: "POST",
    data: formData,
    requestType: "form",
    ...(options || {}),
  });
}
