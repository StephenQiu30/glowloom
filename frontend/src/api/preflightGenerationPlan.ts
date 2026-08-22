// @ts-ignore
/* eslint-disable */
import request, { type RequestOptions } from "@/lib/request";

/** 此处后端没有提供注释 POST /api/generation-plans/${param0}/preflight */
export async function preflightGenerationPlan(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.preflightGenerationPlanParams,
  options?: RequestOptions
) {
  const { plan_id: param0, ...queryParams } = params;
  return request<API.GenerationPlanResponse>(
    `/api/generation-plans/${param0}/preflight`,
    {
      method: "POST",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}
