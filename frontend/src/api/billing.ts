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

/** 提交空间积分指令 POST /api/platform/billing/scopes/${param0}/credit-commands */
export async function submitScopeCreditCommand(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.submitScopeCreditCommandParams,
  body: API.creditCommandRequest,
  options?: RequestOptions,
) {
  const { scopeID: param0, ...queryParams } = params;
  return request<any>(
    `/api/platform/billing/scopes/${param0}/credit-commands`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 查询空间积分指令回执 GET /api/platform/billing/scopes/${param0}/credit-commands/${param1} */
export async function getScopeCreditCommand(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getScopeCreditCommandParams,
  options?: RequestOptions,
) {
  const { scopeID: param0, commandID: param1, ...queryParams } = params;
  return request<API.CreditCommandReceipt>(
    `/api/platform/billing/scopes/${param0}/credit-commands/${param1}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 查询指定创作空间积分钱包 GET /api/platform/billing/scopes/${param0}/wallet */
export async function getScopeCreditWallet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getScopeCreditWalletParams,
  options?: RequestOptions,
) {
  const { scopeID: param0, ...queryParams } = params;
  return request<API.WalletView>(
    `/api/platform/billing/scopes/${param0}/wallet`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    },
  );
}
