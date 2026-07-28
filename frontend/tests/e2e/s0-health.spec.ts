import { expect, test } from "@playwright/test";

test("S0 首页展示平台定位和后端状态", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "从剧本到成片，保持每一步可控" })).toBeVisible();
  await expect(page.getByText("后端服务正常")).toBeVisible();
  await expect(page.getByRole("link", { name: "查看接口状态" })).toHaveAttribute(
    "href",
    "http://127.0.0.1:8000/readyz",
  );
});
