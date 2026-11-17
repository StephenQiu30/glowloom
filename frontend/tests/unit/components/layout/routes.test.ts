import { expect, it } from "vitest";
import {
  resolveAdminSection,
  screenHref,
  screens,
} from "@/components/layout/routes";

it("首页和所有设计页面使用正式产品地址", () => {
  expect(screenHref("home")).toBe("/");
  for (const screen of screens.filter(({ id }) => id !== "home")) {
    expect(screenHref(screen.id)).toBe(`/${screen.id}`);
  }
});

it("日志与审计共用管理布局，个人分析不会获得管理外壳", () => {
  expect(resolveAdminSection("health")).toBe("health");
  expect(resolveAdminSection("audit")).toBe("audit");
  expect(resolveAdminSection("analytics")).toBeUndefined();
  expect(resolveAdminSection("analytics", true)).toBe("analytics");
});
