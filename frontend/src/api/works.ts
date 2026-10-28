// @ts-ignore
/* eslint-disable */
import { request } from "@/lib/request";
import type { RequestOptions } from "@/lib/request";

/** 查看已发布作品 GET /api/works/${param0} */
export async function getPublishedWork(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getPublishedWorkParams,
  options?: RequestOptions,
) {
  const { work_id: param0, ...queryParams } = params;
  return request<API.PublishedWork>(`/api/works/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}
