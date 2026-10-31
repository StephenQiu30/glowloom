/* eslint-disable */
import { request } from "@/components/canvas/runtime/lib/request";
import type { AxiosRequestConfig } from "axios";

/** 读取本人画布原任务及结果 GET /canvas/generation/list */
export async function listCanvasGenerations(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CanvasContract.listCanvasGenerationsParams,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseListCanvasGenerationVO>(
    "/canvas/generation/list",
    {
      method: "GET",
      params: {
        ...params,
      },
      ...(options || {}),
    },
  );
}

/** 报价已保存画布节点 POST /canvas/generation/quote */
export async function quoteCanvasGenerationCredits(
  body: CanvasContract.CanvasGenerateRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseCreditQuoteVO>(
    "/canvas/generation/quote",
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

/** 查询原任务或恢复原输出归档 POST /canvas/generation/refresh */
export async function refreshCanvasGeneration(
  body: CanvasContract.CanvasGenerationRefreshRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseCanvasGenerationVO>(
    "/canvas/generation/refresh",
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

/** 主动重试原冻结提交 POST /canvas/generation/retry */
export async function retryCanvasGeneration(
  body: CanvasContract.CanvasGenerationRefreshRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseCanvasGenerationVO>(
    "/canvas/generation/retry",
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

/** 冻结并提交画布节点 POST /canvas/generation/submit */
export async function submitCanvasGeneration(
  body: CanvasContract.CanvasGenerateRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseCanvasGenerationVO>(
    "/canvas/generation/submit",
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
