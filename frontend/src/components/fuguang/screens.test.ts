import { expect, it } from "vitest";
import { screenHref, screens } from "./screens";

it("首页和所有设计页面使用正式产品地址", () => {
  expect(screenHref("home")).toBe("/");
  for (const screen of screens.filter(({ id }) => id !== "home")) {
    expect(screenHref(screen.id)).toBe(`/${screen.id}`);
  }
});
