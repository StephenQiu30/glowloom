// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** List Deliveries GET /v1/episodes/${param0}/deliveries */
export async function listDeliveries(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listDeliveriesParams,
  options?: RequestOptions
) {
  const { episode_id: param0, ...queryParams } = params;
  return request<API.DeliveryListResponse>(
    `/v1/episodes/${param0}/deliveries`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}
