/* eslint-disable */
import { request } from "@/components/canvas/runtime/lib/request";
import type { AxiosRequestConfig } from "axios";

/** 尝试取消本人排队中的原视频，运行中不支持 POST /generation/cancel */
export async function cancelGenerationTask(
  body: CanvasContract.GenerationRefreshRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseGenerationTaskVO>(
    "/generation/cancel",
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

/** 恢复本人提交回执 GET /generation/command */
export async function getGenerationCommand(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CanvasContract.getGenerationCommandParams,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseGenerationTaskVO>(
    "/generation/command",
    {
      method: "GET",
      params: {
        ...params,
      },
      ...(options || {}),
    },
  );
}

/** 读取本人持久任务 GET /generation/get */
export async function getGenerationTask(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CanvasContract.getGenerationTaskParams,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseGenerationTaskVO>(
    "/generation/get",
    {
      method: "GET",
      params: {
        ...params,
      },
      ...(options || {}),
    },
  );
}

/** 分页读取本人生成历史 GET /generation/history */
export async function listGenerationHistory(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CanvasContract.listGenerationHistoryParams,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseListGenerationTaskVO>(
    "/generation/history",
    {
      method: "GET",
      params: {
        ...params,
      },
      ...(options || {}),
    },
  );
}

/** 读取可执行图像模型 GET /generation/models/image */
export async function listImageGenerationModels(options?: AxiosRequestConfig) {
  return request<CanvasContract.BaseResponseListModelExecutionVO>(
    "/generation/models/image",
    {
      method: "GET",
      ...(options || {}),
    },
  );
}

/** 读取可执行文本模型 GET /generation/models/text */
export async function listTextGenerationModels(options?: AxiosRequestConfig) {
  return request<CanvasContract.BaseResponseListModelExecutionVO>(
    "/generation/models/text",
    {
      method: "GET",
      ...(options || {}),
    },
  );
}

/** 读取可执行视频模型 GET /generation/models/video */
export async function listVideoGenerationModels(options?: AxiosRequestConfig) {
  return request<CanvasContract.BaseResponseListModelExecutionVO>(
    "/generation/models/video",
    {
      method: "GET",
      ...(options || {}),
    },
  );
}

/** 读取生成参数定义 GET /generation/options */
export async function getGenerationOptions(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CanvasContract.getGenerationOptionsParams,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseGenerationOptionsVO>(
    "/generation/options",
    {
      method: "GET",
      params: {
        ...params,
      },
      ...(options || {}),
    },
  );
}

/** 删除本人终态视频的供应商记录，保留平台历史与文件 POST /generation/provider/delete */
export async function deleteGenerationProviderRecord(
  body: CanvasContract.GenerationRefreshRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseGenerationTaskVO>(
    "/generation/provider/delete",
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

/** 读取冻结输入的模型积分报价及授权上限 POST /generation/quote */
export async function quoteGenerationCredits(
  body: CanvasContract.GenerationSubmitRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseCreditQuoteVO>(
    "/generation/quote",
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

/** 授权下载本人生成任务的单项产物 POST /generation/read */
export async function readGenerationFile(
  body: CanvasContract.GenerationReadRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseAssetDownloadVO>(
    "/generation/read",
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

/** 查询原视频或恢复原输出归档，不重提生成 POST /generation/refresh */
export async function refreshGenerationTask(
  body: CanvasContract.GenerationRefreshRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseGenerationTaskVO>(
    "/generation/refresh",
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

/** 提交本人单节点任务，同一命令不重复计费 POST /generation/submit */
export async function submitGenerationTask(
  body: CanvasContract.GenerationSubmitRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseGenerationTaskVO>(
    "/generation/submit",
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
