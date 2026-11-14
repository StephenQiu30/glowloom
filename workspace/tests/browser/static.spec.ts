import { expect, test } from "@playwright/test";
test("reads the real Workspace design, its source anchor, diagram and raw document", async ({
  page,
}) => {
  await page.goto("/");
  await expect(
    page.getByRole("heading", { name: "知识库", exact: true }),
  ).toBeVisible();
  const flight = await page.request.get("/read/design-43/index.txt");
  expect(flight.headers()["content-type"]).toMatch(/^text\/plain/);
  await page.evaluate(() =>
    Object.defineProperty(window, "__workspaceNavigationProbe", {
      value: "preserved",
      configurable: true,
    }),
  );
  await page.locator('main a[href="/read/design-43/"]').first().click();
  await expect(
    page.getByRole("heading", {
      name: "Workspace 知识库与研发工作流服务设计",
      exact: true,
    }),
  ).toBeVisible();
  await expect(page.locator(".prose figure svg").first()).toBeVisible();
  expect(
    await page.evaluate(() =>
      Reflect.get(window, "__workspaceNavigationProbe"),
    ),
  ).toBe("preserved");
  await page.goto("/read/requirement-28/#src-ws-07");
  await expect(page.locator("#src-ws-07")).toBeAttached();
  const raw = await page.request.get("/raw/requirement-28.md");
  expect(raw.status()).toBe(200);
  expect(await raw.text()).toContain("SRC-WS-07");
});
test("searches Chinese terms and business IDs and reports no results truthfully", async ({
  page,
}) => {
  await page.goto("/search/");
  await page.getByRole("searchbox", { name: "全文搜索" }).fill("FR-WS-01");
  await expect(page.locator(".search-result").first()).toBeVisible();
  await page.getByRole("searchbox", { name: "全文搜索" }).fill("静态知识库");
  await expect(page.locator(".search-result").first()).toBeVisible();
  await page
    .getByRole("searchbox", { name: "全文搜索" })
    .fill("unlikely-unfindable-qqq-709113");
  await expect(page.getByText("没有找到相关文档")).toBeVisible();
});
test("keeps narrow content contained and restores keyboard focus after closing the directory", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/read/prd-29/");
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  const opener = page.getByRole("button", { name: "打开项目目录" });
  await opener.click();
  await expect(page.getByRole("dialog")).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(opener).toBeFocused();
  await page.getByRole("button", { name: "切换主题" }).click();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await page.goto("/tasks/");
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
});
test("exports a bounded source-verified static handoff without implying execution authorization", async ({
  page,
}) => {
  await page.goto("/tasks/");
  await page.getByLabel("交接目标").fill("核对Workspace需求与代码入口");
  await page.getByLabel("交接接受标准").fill("列出实际来源和未确认项");
  const download = page.waitForEvent("download");
  await page.getByRole("button", { name: "导出交接包" }).click();
  const stream = await (await download).createReadStream();
  const chunks = [];
  for await (const c of stream!) chunks.push(c);
  const pack = JSON.parse(Buffer.concat(chunks).toString());
  expect(pack.executionAuthorization).toBe(false);
  expect(
    pack.documents.some((d: { id: string }) => d.id === "root-agents"),
  ).toBe(true);
  expect(
    pack.documents.reduce(
      (n: number, d: { excerpt: string }) => n + d.excerpt.length,
      0,
    ),
  ).toBeLessThanOrEqual(12000);
});
