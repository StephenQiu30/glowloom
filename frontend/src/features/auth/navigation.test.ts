import { describe, expect, it } from "vitest";
import { loginDestination } from "./navigation";

describe("登录后的工作区导航", () => {
  it("保留项目与画布深链接", () => {
    expect(loginDestination("/canvas?project=abc&canvas=def")).toBe(
      "/canvas?project=abc&canvas=def",
    );
    expect(loginDestination("/projects/abc/canvas")).toBe(
      "/projects/abc/canvas",
    );
  });
  it("拒绝外部源、协议、反斜线和不属于工作区的路径", () => {
    for (const value of [
      "https://evil.invalid",
      "//evil.invalid",
      "/\\evil.invalid",
      "javascript:alert(1)",
      "/login",
      "/api/auth/logout",
      "/canvas-foreign",
    ]) {
      expect(loginDestination(value)).toBe("/canvas");
    }
  });
});
