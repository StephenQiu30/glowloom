// @ts-ignore
/* eslint-disable */
import { request } from "@/lib/request";

/** 项目模型目录与参数表单 GET /api/models */
export async function listProjectModels(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listProjectModelsParams,
  options?: import("@/lib/request").RequestOptions,
) {
  return request<API.internalCatalogAdapterHttpModelPage>("/api/models", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}
