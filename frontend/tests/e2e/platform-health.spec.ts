import { expect, test } from "@playwright/test";

test("首页展示平台定位和后端状态", async ({ page }) => {
  const backendPort = process.env.LANVERSE_E2E_BACKEND_PORT ?? "8001";
  await page.goto("/");
  await expect(
    page.getByRole("heading", {
      name: /把剧本，变成.*可追踪的成片。/,
    }),
  ).toBeVisible();
  await expect(page.getByRole("link", { name: "导入剧本" })).toHaveAttribute(
    "href",
    "/register",
  );
  await expect(page.getByRole("link", { name: "继续制作" })).toHaveAttribute(
    "href",
    "/login",
  );

  const readiness = await page.request.get(
    `http://127.0.0.1:${backendPort}/readyz`,
  );
  expect(readiness.ok()).toBe(true);
  expect(await readiness.json()).toMatchObject(
    { status: "ready" },
  );
});
