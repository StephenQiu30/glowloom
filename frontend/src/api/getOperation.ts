// @ts-ignore
/* eslint-disable */
import request, { type RequestOptions } from "@/lib/request";

/** 此处后端没有提供注释 GET /api/operations/${param0} */
export async function getOperation(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getOperationParams,
  options?: RequestOptions
) {
  const { operation_id: param0, ...queryParams } = params;
  return request<API.OperationResponse>(`/api/operations/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}
