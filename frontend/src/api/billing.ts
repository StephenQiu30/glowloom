// @ts-ignore
/* eslint-disable */
import { request } from "@/lib/request";
import type { RequestOptions } from "@/lib/request";

/** 创作空间积分钱包 GET /api/billing/wallet */
export async function getCreditWallet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getCreditWalletParams,
  options?: RequestOptions,
) {
  return request<API.WalletView>("/api/billing/wallet", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}
