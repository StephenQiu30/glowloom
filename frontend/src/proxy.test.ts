import { expect, it } from "vitest";
import { unstable_doesMiddlewareMatch } from "next/experimental/testing/server";
import { config } from "./proxy";

it("公开构建资源供 Next 图片优化器读取，其余页面和接口仍经过来源校验", () => {
  const matches = (url: string) =>
    unstable_doesMiddlewareMatch({ config, nextConfig: {}, url });
  expect(matches("/design/reference/amber-perfume.png")).toBe(false);
  expect(matches("/_next/static/chunks/example.js")).toBe(false);
  for (const path of [
    "/",
    "/assets",
    "/account",
    "/api/session",
    "/api/media/library",
    "/api/media?path=/design/reference/amber-perfume.png",
    "/_next/image?url=%2Fdesign%2Freference%2Famber-perfume.png&w=640&q=75",
    "/design/reference-private/file.png",
  ]) {
    expect(matches(path)).toBe(true);
  }
});
