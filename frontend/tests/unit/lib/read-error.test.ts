import { describe, expect, it } from "vitest";
import { RequestError } from "@/lib/request";
import { readErrorMessage } from "@/lib/read-error";

describe("读取状态的用户提示", () => {
  it.each([
    [
      new RequestError("dependency_unavailable", 503),
      "服务暂时不可用，请稍后重试。",
    ],
    [new RequestError("session_expired", 401), "登录已失效，请重新登录。"],
    [
      new RequestError("forbidden", 403),
      "当前账号没有访问权限，请切换账号或联系管理员。",
    ],
    [new RequestError("not_found", 404), "内容不存在或当前账号无法访问。"],
    [new RequestError("rate_limited", 429), "请求较多，请稍后重试。"],
    [new RequestError("network_error"), "连接暂时中断，请检查网络后重试。"],
  ])("按读取失败原因给出可执行提示", (error, message) => {
    expect(readErrorMessage(error)).toBe(message);
  });
  it("未知异常不会泄露服务码、内部信息或谎报为空列表", () => {
    const result = readErrorMessage(new Error("postgres://private/details"));
    expect(result).toBe("暂时无法读取内容，请稍后重试。");
    expect(
      readErrorMessage(new RequestError("private_internal_error", 500)),
    ).not.toContain("private_internal_error");
  });
});
