import { createApi, fakeBaseQuery } from "@reduxjs/toolkit/query/react";
import { ApiClientError } from "@/lib/request";

export type AppApiError = {
  message: string;
  code: string;
  nextAction?: string;
  details?: unknown;
};

const errorMessages: Record<string, string> = {
  dependency_unavailable: "注册服务暂时不可用，请稍后重试。",
  invalid_verification_code: "验证码不正确，请检查后重新输入。",
  rate_limited: "验证码发送过于频繁，请等待倒计时结束后重试。",
  resource_conflict: "请求所依据的服务端版本已经变化，请刷新后重试。",
  validation_failed: "提交内容与服务端契约不一致，请刷新后重试。",
  unauthenticated: "邮箱或密码不正确，请重新输入。",
  verification_expired: "验证码或注册凭证已失效，请重新发送验证码。",
  creation_unavailable: "文本创作服务尚未配置，请联系管理员启用。",
  head_conflict: "正式原稿已被更新，请刷新确认后重新操作。",
  source_hash_drift: "原稿内容与固定版本不一致，已停止创作。",
  revision_conflict: "当前版本已变化，请刷新后重新审阅。",
  proposal_has_unresolved_blockers: "请逐项处理待确认问题并填写理由。",
};

export function appApiErrorMessage(error: unknown): string {
  const apiError = error as Partial<AppApiError> | undefined;
  return apiError?.code
    ? (errorMessages[apiError.code] ?? apiError.message ?? "服务暂时不可用，请稍后重试。")
    : (apiError?.message ?? "服务暂时不可用，请稍后重试。");
}

export async function runRequest<T>(
  operation: () => Promise<{ data: T }>,
): Promise<{ data: T } | { error: AppApiError }> {
  try {
    const response = await operation();
    return { data: response.data };
  } catch (error: unknown) {
    if (error instanceof ApiClientError) {
      return {
        error: {
          message: error.message,
          code: error.code,
          nextAction: error.nextAction,
          details: error.details,
        },
      };
    }
    return {
      error: { message: "服务暂时不可用，请稍后重试。", code: "request_failed" },
    };
  }
}

export const appApi = createApi({
  reducerPath: "appApi",
  baseQuery: fakeBaseQuery<AppApiError>(),
  tagTypes: [
    "Me",
    "Workspaces",
    "Projects",
    "Project",
    "Episodes",
    "Media",
    "ScriptDocuments",
    "ProductionBible",
    "EpisodePlans",
    "HumanTasks",
    "WorkflowRuns",
    "CreationRuns",
  ],
  endpoints: () => ({}),
});
