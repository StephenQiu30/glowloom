// @ts-ignore
/* eslint-disable */
import request, { type RequestOptions } from "@/lib/request";

/** 此处后端没有提供注释 POST /api/generation-plans/${param0}/approve */
export async function approveGenerationPlan(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.approveGenerationPlanParams,
  body: API.ApproveGenerationPlanRequest,
  options?: RequestOptions
) {
  const { plan_id: param0, ...queryParams } = params;
  return request<API.GenerationPlanResponse>(
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
