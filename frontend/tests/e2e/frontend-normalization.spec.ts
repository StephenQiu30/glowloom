import { expect, test } from "@playwright/test";

import { registerUser } from "./auth-support";

for (const viewport of [
  { width: 1440, height: 1000 },
  { width: 390, height: 844 },
]) {
  test(`标准组件与账户项目流程 ${viewport.width}px`, async ({ page }, info) => {
    test.setTimeout(60_000);
    await page.emulateMedia({ colorScheme: "light" });
    await page.setViewportSize(viewport);
    const name = `规范验收-${viewport.width}-${Date.now()}`;
    await registerUser(page, {
      displayName: "规范验收创作者",
      email: `frontend-${viewport.width}-${Date.now()}@example.com`,
    });
    const create = page.getByRole("button", { name: "创建项目", exact: true });
    await create.click();
    await expect(page.getByRole("dialog", { name: "创建项目" })).toBeVisible();
    await page.getByLabel("项目名称").fill("   ");
    await page.getByRole("button", { name: "确认创建" }).click();
    await expect(page.getByText("请输入项目名称")).toBeVisible();
    await expect(page.getByLabel("项目名称")).toHaveAttribute("aria-invalid", "true");
    await page.keyboard.press("Escape");
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await expect(create).toBeFocused();
    await create.click();
    await page.getByLabel("项目名称").fill(name);
    await page
      .getByLabel("项目简介")
      .fill("以标准表单创建项目，验证键盘、移动布局与服务端持久化。");
    await page.getByRole("button", { name: "确认创建" }).click();
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await expect(page.getByRole("link", { name: `打开项目 ${name}` })).toBeVisible();
    await page.screenshot({ path: info.outputPath("projects.png"), fullPage: true });
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(
      true,
    );

    await page.getByRole("link", { name: `打开项目 ${name}` }).click();
    await page.getByRole("link", { name: "审核队列" }).click();
    const filter = page.getByRole("combobox", { name: "任务状态筛选" });
    await expect(filter).toBeVisible();
    await filter.focus();
    await page.keyboard.press("Enter");
    await expect(page.getByRole("listbox")).toBeVisible();
    await page.keyboard.press("ArrowDown");
    await page.keyboard.press("Enter");
    await expect(page.getByRole("listbox")).toHaveCount(0);
    await page.screenshot({ path: info.outputPath("reviews.png"), fullPage: true });
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(
      true,
    );

    await page.goto("/workspaces");
    await page.getByLabel("显示名称", { exact: true }).fill("规范验收导演");
    await page.getByRole("button", { name: "保存个人资料" }).click();
    await expect(page.getByText("个人资料已保存。")).toBeVisible();
    await page.getByRole("button", { name: "切换主题" }).click();
    await expect(page.locator("html")).toHaveClass(/dark/);
    await page.screenshot({ path: info.outputPath("workspaces-dark.png"), fullPage: true });
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(
      true,
    );
    await page.reload();
    await expect(page.getByLabel("显示名称", { exact: true })).toHaveValue("规范验收导演");
  });
}
