// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** Get Delivery GET /v1/deliveries/${param0} */
export async function getDelivery(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getDeliveryParams,
  options?: RequestOptions
) {
  const { delivery_id: param0, ...queryParams } = params;
  return request<API.DeliveryDetailResponse>(`/v1/deliveries/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}
