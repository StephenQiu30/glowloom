// @ts-ignore
/* eslint-disable */
import request, { type RequestOptions } from "@/lib/request";

/** 此处后端没有提供注释 GET /api/projects/${param0}/content-units/${param1}/shots */
export async function listShots(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listShotsParams,
  options?: RequestOptions
) {
  const {
    project_id: param0,
    content_unit_id: param1,
    ...queryParams
  } = params;
  return request<API.ShotListResponse>(
    `/api/projects/${param0}/content-units/${param1}/shots`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}
