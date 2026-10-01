// @ts-ignore
/* eslint-disable */
import { request } from "@/lib/request";

/** 批次汇总与逐项任务 GET /api/batches/${param0} */
export async function getOperationBatch(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getOperationBatchParams,
  options?: import("@/lib/request").RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalOperationApplicationBatchDetail>(
    `/api/batches/${param0}`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    },
  );
}

/** 请求取消批次 POST /api/batches/${param0}/cancel */
export async function cancelOperationBatch(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.cancelOperationBatchParams,
  body: API.internalOperationAdapterHttpProjectRequest,
  options?: import("@/lib/request").RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<any>(`/api/batches/${param0}/cancel`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 确认批量报价 POST /api/batches/${param0}/confirm */
export async function confirmBatchQuote(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.confirmBatchQuoteParams,
  body: API.internalOperationAdapterHttpConfirmBatchRequest,
  options?: import("@/lib/request").RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.internalOperationAdapterHttpConfirmBatchResponse>(
    `/api/batches/${param0}/confirm`,
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

/** 请求继续暂停批次 POST /api/batches/${param0}/resume */
export async function resumeOperationBatch(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.resumeOperationBatchParams,
  body: API.internalOperationAdapterHttpProjectRequest,
  options?: import("@/lib/request").RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<any>(`/api/batches/${param0}/resume`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 任务详情与候选 GET /api/operations/${param0} */
export async function getOperation(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getOperationParams,
  options?: import("@/lib/request").RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalOperationApplicationTaskDetail>(
    `/api/operations/${param0}`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    },
  );
}

/** 请求取消任务 POST /api/operations/${param0}/cancel */
export async function cancelOperation(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.cancelOperationParams,
  body: API.internalOperationAdapterHttpProjectRequest,
  options?: import("@/lib/request").RequestOptions,
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
  params: API.confirmOperationQuoteParams,
  body: API.internalOperationAdapterHttpProjectRequest,
  options?: import("@/lib/request").RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.internalOperationAdapterHttpConfirmResponse>(
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

/** 自由生成报价 POST /api/projects/${param0}/free-operations */
export async function createFreeQuote(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.createFreeQuoteParams,
  body: API.internalOperationAdapterHttpQuoteItemRequest,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.internalOperationAdapterHttpQuoteResponse>(
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

/** 项目任务中心 GET /api/projects/${param0}/operations */
export async function listProjectOperations(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listProjectOperationsParams,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.internalOperationAdapterHttpTaskPageResponse>(
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

/** 单项或批量自由生成报价 POST /api/projects/${param0}/quotes */
export async function createGenerationQuotes(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.createGenerationQuotesParams,
  body: API.internalOperationAdapterHttpQuotesRequest,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalOperationApplicationCreateBatchFreeQuoteResult>(
    `/api/projects/${param0}/quotes`,
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
