// @ts-ignore
/* eslint-disable */
import { request } from "@umijs/max";

/** 创建生成计划 POST /api/generation-plans */
export async function generationPlanCreate(
  body: API.createRequest,
  options?: { [key: string]: any }
) {
  return request<Record<string, any>>("/api/generation-plans", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 查询生成计划 GET /api/generation-plans/${param0} */
export async function generationPlanGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.generationPlanGetParams,
  options?: { [key: string]: any }
) {
  const { planID: param0, ...queryParams } = params;
  return request<Record<string, any>>(`/api/generation-plans/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 批准生成计划 POST /api/generation-plans/${param0}/approve */
export async function generationPlanApprove(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.generationPlanApproveParams,
  body: API.approveRequest,
  options?: { [key: string]: any }
) {
  const { planID: param0, ...queryParams } = params;
  return request<Record<string, any>>(
    `/api/generation-plans/${param0}/approve`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    }
  );
}

/** 预检生成计划 POST /api/generation-plans/${param0}/preflight */
export async function generationPlanPreflight(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.generationPlanPreflightParams,
  options?: { [key: string]: any }
) {
  const { planID: param0, ...queryParams } = params;
  return request<Record<string, any>>(
    `/api/generation-plans/${param0}/preflight`,
    {
      method: "POST",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}
