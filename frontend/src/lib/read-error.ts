import { RequestError } from "./request";

/** Read failures must not expose transport details or imply a write succeeded. */
export function readErrorMessage(error: unknown): string {
  if (error instanceof RequestError) {
    if (error.status === 401) return "登录已失效，请重新登录。";
    if (error.status === 403)
      return "当前账号没有访问权限，请切换账号或联系管理员。";
    if (error.status === 404) return "内容不存在或当前账号无法访问。";
    if (error.status === 429) return "请求较多，请稍后重试。";
    if (error.status >= 500 || error.code === "dependency_unavailable")
      return "服务暂时不可用，请稍后重试。";
    if (error.code === "network_error" || error.code === "network_unavailable")
      return "连接暂时中断，请检查网络后重试。";
  }
  return "暂时无法读取内容，请稍后重试。";
}
