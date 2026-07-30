import { expect, test } from "@playwright/test";

test("S0 首页展示平台定位和后端状态", async ({ page }) => {
  await page.goto("/");
  await expect(
    page.getByRole("heading", {
      name: "从剧本到可复用资产，按真实阶段推进每一集",
    }),
  ).toBeVisible();
  await expect(page.getByRole("link", { name: "创建账户" })).toBeVisible();

  const readiness = await page.request.get("http://127.0.0.1:8001/readyz");
  expect(readiness.ok()).toBe(true);
  expect(await readiness.json()).toMatchObject(
    { status: "ready" },
  );
});
