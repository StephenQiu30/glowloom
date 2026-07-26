// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** Derive Subtitle Draft POST /v1/subtitle-versions/${param0}/drafts */
export async function deriveSubtitleDraft(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.deriveSubtitleDraftParams,
  options?: RequestOptions
) {
  const { version_id: param0, ...queryParams } = params;
  return request<API.SubtitleVersionResponse>(
    `/v1/subtitle-versions/${param0}/drafts`,
    {
      method: "POST",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}
