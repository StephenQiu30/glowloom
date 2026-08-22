// @ts-ignore
/* eslint-disable */
import request, { type RequestOptions } from "@/lib/request";

/** 此处后端没有提供注释 POST /api/generation-plans */
export async function createGenerationPlan(
  body: API.CreateGenerationPlanRequest,
  options?: RequestOptions
) {
  return request<API.GenerationPlanResponse>("/api/generation-plans", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}
