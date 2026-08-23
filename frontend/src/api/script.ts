// @ts-ignore
/* eslint-disable */
import request, { type RequestOptions } from "@/lib/request";

/** 创建剧本修订 POST /api/projects/${param0}/script-revisions */
export async function scriptRevisionCreate(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.scriptRevisionCreateParams,
  body: {},
  file?: File,
  options?: RequestOptions
) {
  const { projectID: param0, ...queryParams } = params;
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
          formData.append(ele, JSON.stringify(item));
        }
      } else {
        formData.append(ele, item);
      }
    }
  });

  return request<API.ScriptRevisionEnvelope>(
    `/api/projects/${param0}/script-revisions`,
    {
      method: "POST",
      params: { ...queryParams },
      data: formData,
      requestType: "form",
      ...(options || {}),
    }
  );
}

/** 获取分析草稿 GET /api/script-revisions/${param0}/analysis-draft */
export async function scriptAnalysisDraft(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.scriptAnalysisDraftParams,
  options?: RequestOptions
) {
  const { revisionID: param0, ...queryParams } = params;
  return request<API.AnalysisEnvelope>(
    `/api/script-revisions/${param0}/analysis-draft`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** 排队分析剧本 POST /api/script-revisions/${param0}/analyze */
export async function scriptAnalysisQueue(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.scriptAnalysisQueueParams,
  options?: RequestOptions
) {
  const { revisionID: param0, ...queryParams } = params;
  return request<any>(`/api/script-revisions/${param0}/analyze`, {
    method: "POST",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 批准剧本分析 POST /api/script-revisions/${param0}/approve */
export async function scriptAnalysisApprove(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.scriptAnalysisApproveParams,
  options?: RequestOptions
) {
  const { revisionID: param0, ...queryParams } = params;
  return request<API.AnalysisEnvelope>(
    `/api/script-revisions/${param0}/approve`,
    {
      method: "POST",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}
