// @ts-ignore
/* eslint-disable */
import request, { type RequestOptions } from "@/lib/request";

/** 查询任务状态 GET /api/operations/${param0} */
export async function operationGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.operationGetParams,
  options?: RequestOptions
) {
  const { operationID: param0, ...queryParams } = params;
  return request<API.OperationEnvelope>(`/api/operations/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}
