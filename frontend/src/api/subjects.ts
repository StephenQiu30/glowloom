// @ts-ignore
/* eslint-disable */
import { request } from "@/lib/request";
import type { RequestOptions } from "@/lib/request";

/** 查证主体命令的已提交结果 GET /api/subject-commands/${param0} */
export async function getSubjectCommand(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getSubjectCommandParams,
  options?: RequestOptions,
) {
  const { key: param0, ...queryParams } = params;
  return request<API.Recovery>(`/api/subject-commands/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 查证智能配音永久命令 GET /api/subject-voice-commands/${param0} */
export async function getSubjectVoiceCommand(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getSubjectVoiceCommandParams,
  options?: RequestOptions,
) {
  const { key: param0, ...queryParams } = params;
  return request<API.VoiceRecovery>(`/api/subject-voice-commands/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 查询当前空间主体 GET /api/subjects */
export async function listSubjects(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listSubjectsParams,
  options?: RequestOptions,
) {
  return request<API.Page>("/api/subjects", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** 新建主体 POST /api/subjects */
export async function createSubject(
  body: API.SaveInput,
  options?: RequestOptions,
) {
  return request<API.SubjectDetail>("/api/subjects", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 读取主体设置 GET /api/subjects/${param0} */
export async function getSubject(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getSubjectParams,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.SubjectDetail>(`/api/subjects/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 更新主体设置 PUT /api/subjects/${param0} */
export async function updateSubject(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.updateSubjectParams,
  body: API.SaveInput,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.SubjectDetail>(`/api/subjects/${param0}`, {
    method: "PUT",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 删除主体（不提供恢复） DELETE /api/subjects/${param0} */
export async function deleteSubject(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.deleteSubjectParams,
  body: API.RevisionInput,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.ItemResult>(`/api/subjects/${param0}`, {
    method: "DELETE",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 收藏或取消收藏主体 PUT /api/subjects/${param0}/favorite */
export async function setSubjectFavorite(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.setSubjectFavoriteParams,
  body: API.FavoriteInput,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.ItemResult>(`/api/subjects/${param0}/favorite`, {
    method: "PUT",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 读取固定主体版本 GET /api/subjects/${param0}/versions/${param1} */
export async function getSubjectVersion(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getSubjectVersionParams,
  options?: RequestOptions,
) {
  const { id: param0, version: param1, ...queryParams } = params;
  return request<API.SubjectDetail>(
    `/api/subjects/${param0}/versions/${param1}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 读取主体版本的正式参考素材 GET /api/subjects/${param0}/versions/${param1}/media/${param2} */
export async function getSubjectMedia(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getSubjectMediaParams,
  options?: RequestOptions,
) {
  const {
    id: param0,
    version: param1,
    binding: param2,
    ...queryParams
  } = params;
  return request<Blob>(
    `/api/subjects/${param0}/versions/${param1}/media/${param2}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || { responseType: "blob" }),
    },
  );
}

/** 批量删除或收藏主体 POST /api/subjects/batch */
export async function batchSubjects(
  body: API.BatchInput,
  options?: RequestOptions,
) {
  return request<API.BatchResult>("/api/subjects/batch", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 下载选中主体的版本及正式媒体 POST /api/subjects/export */
export async function exportSubjects(
  body: API.ExportInput,
  options?: RequestOptions,
) {
  return request<Blob>("/api/subjects/export", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || { responseType: "blob" }),
  });
}

/** 选择主体参考资产 GET /api/subjects/sources */
export async function listSubjectSources(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listSubjectSourcesParams,
  options?: RequestOptions,
) {
  return request<API.SubjectSourcePage>("/api/subjects/sources", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** 核对主体来源资产 GET /api/subjects/sources/${param0} */
export async function getSubjectSource(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getSubjectSourceParams,
  options?: RequestOptions,
) {
  const { item_id: param0, ...queryParams } = params;
  return request<API.SubjectSourceItem>(`/api/subjects/sources/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 预览主体来源资产 GET /api/subjects/sources/${param0}/preview */
export async function previewSubjectSource(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.previewSubjectSourceParams,
  options?: RequestOptions,
) {
  const { item_id: param0, ...queryParams } = params;
  return request<API.LibraryMediaPreview>(
    `/api/subjects/sources/${param0}/preview`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 上传主体参考或音色资产 POST /api/subjects/sources/uploads */
export async function uploadSubjectSource(
  body: {
    /** 已确认内容及使用权限 */
    local_review_confirmed: boolean;
  },
  file?: File,
  options?: RequestOptions,
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

  return request<API.PersonalUploadResult>("/api/subjects/sources/uploads", {
    method: "POST",
    data: formData,
    requestType: "form",
    ...(options || {}),
  });
}

/** 创建智能配音任务 POST /api/subjects/voice-jobs */
export async function createSubjectVoiceJob(
  body: API.CreateVoiceJobInput,
  options?: RequestOptions,
) {
  return request<any>("/api/subjects/voice-jobs", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 读取智能配音任务 GET /api/subjects/voice-jobs/${param0} */
export async function getSubjectVoiceJob(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getSubjectVoiceJobParams,
  options?: RequestOptions,
) {
  const { job_id: param0, ...queryParams } = params;
  return request<API.VoiceJob>(`/api/subjects/voice-jobs/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 智能配音模型与音色 GET /api/subjects/voice-options */
export async function listSubjectVoiceOptions(options?: RequestOptions) {
  return request<API.VoiceOptions>("/api/subjects/voice-options", {
    method: "GET",
    ...(options || {}),
  });
}
