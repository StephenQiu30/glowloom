// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** Derive Storyboard Draft POST /v1/shot-spec-versions/${param0}/drafts */
export async function deriveStoryboardDraft(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.deriveStoryboardDraftParams,
  options?: RequestOptions
) {
  const { version_id: param0, ...queryParams } = params;
  return request<API.StoryboardGenerationResponse>(
    `/v1/shot-spec-versions/${param0}/drafts`,
    {
      method: "POST",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}
