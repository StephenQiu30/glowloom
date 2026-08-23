// @ts-ignore
/* eslint-disable */
import request, { type RequestOptions } from "@/lib/request";

/** 查询镜头 GET /api/projects/${param0}/content-units/${param1}/shots */
export async function shotList(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.shotListParams,
  options?: RequestOptions
) {
  const { projectID: param0, contentUnitID: param1, ...queryParams } = params;
  return request<API.ShotListEnvelope>(
    `/api/projects/${param0}/content-units/${param1}/shots`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** 创建镜头 POST /api/projects/${param0}/content-units/${param1}/shots */
export async function shotCreate(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.shotCreateParams,
  body: API.createShotsRequest,
  options?: RequestOptions
) {
  const { projectID: param0, contentUnitID: param1, ...queryParams } = params;
  return request<API.ShotListEnvelope>(
    `/api/projects/${param0}/content-units/${param1}/shots`,
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
