// @vitest-environment node
import { expect, it } from "vitest";
import { GET } from "@/app/projects/route";

it("旧画布入口统一到资产画布，不再打开归档和回收站管理页", () => {
  for (const query of ["", "?state=archived", "?state=deleted"]) {
    const result = GET(new Request(`http://localhost:1122/projects${query}`));
    expect(result.status).toBe(307);
    expect(result.headers.get("location")).toBe("/assets?view=canvas");
    expect(result.headers.get("cache-control")).toBe("no-store");
  }
});

it("保留已提交搜索，不接受外部重定向目标或旧筛选", () => {
  const result = GET(
    new Request(
      "http://localhost:1122/projects?q=%20%E6%B5%B7%E8%BE%B9%20&state=deleted&redirect=https://example.com",
    ),
  );
  expect(result.headers.get("location")).toBe(
    "/assets?view=canvas&q=%E6%B5%B7%E8%BE%B9",
  );
});

it("代理内部主机和端口不会进入公开跳转地址", () => {
  const result = GET(new Request("http://0.0.0.0:3000/projects"));
  expect(result.headers.get("location")).toBe("/assets?view=canvas");
});
