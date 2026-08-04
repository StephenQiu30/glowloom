import { expect, test } from "@playwright/test";

test("用户管理资料、工作空间和账户凭据", async ({ page }) => {
  const unique = `${Date.now()}-${test.info().workerIndex}`;
  const email = `workspace-${unique}@example.com`;
  const firstPassword = "playwright-secure-password";
  const secondPassword = "playwright-updated-password";

  await page.goto("/register");
  await page.getByLabel("显示名称").fill("空间管理员");
  await page.getByLabel("邮箱").fill(email);
  await page.getByLabel("密码").fill(firstPassword);
  await page.getByRole("button", { name: "注册并开始创作" }).click();
  await expect(page).toHaveURL(/\/projects$/);

  await page.locator('a[href="/workspaces"]').click();
  await expect(page.getByRole("heading", { name: "账户与工作空间" })).toBeVisible();

  await page.getByLabel("显示名称").fill("更新后的管理员");
  await page.getByRole("button", { name: "保存个人资料" }).click();
  await expect(page.getByText("个人资料已保存。")).toBeVisible();

  await page.getByLabel("空间名称").fill("第二创作空间");
  await page.getByRole("button", { name: "创建工作空间" }).click();
  await expect(page.getByText("第二创作空间", { exact: true })).toBeVisible();

  await page.getByLabel("重命名 第二创作空间").fill("正式创作空间");
  await page.getByRole("button", { name: "保存 第二创作空间" }).click();
  await expect(page.getByText("正式创作空间", { exact: true })).toBeVisible();

  await page.getByRole("button", { name: "归档 正式创作空间" }).click();
  await expect(page.getByRole("button", { name: "恢复 正式创作空间" })).toBeVisible();
  await page.getByRole("button", { name: "恢复 正式创作空间" }).click();
  await expect(page.getByRole("button", { name: "归档 正式创作空间" })).toBeVisible();

  await page.getByRole("link", { name: "查看项目" }).last().click();
  await expect(page.getByText("正式创作空间", { exact: true })).toBeVisible();

  await page.locator('a[href="/workspaces"]').click();
  await page.getByLabel("当前密码").fill(firstPassword);
  await page.getByLabel("新密码").fill(secondPassword);
  await page.getByRole("button", { name: "修改密码" }).click();
  await expect(page).toHaveURL(/\/login$/);

  await page.getByLabel("邮箱").fill(email);
  await page.getByLabel("密码").fill(secondPassword);
  await page.getByRole("button", { name: "登录" }).click();
  await expect(page).toHaveURL(/\/projects$/);

  await page.goto("/governance");
  const auditTrail = page.getByRole("region", { name: "操作审计" });
  await expect(auditTrail.getByText("登录成功").first()).toBeVisible();
  await expect(auditTrail.getByText("密码修改")).toBeVisible();
  await expect(auditTrail.getByText("资料更新")).toBeVisible();
  await auditTrail.getByRole("button", { name: "筛选" }).click();
  await auditTrail.getByLabel("动作").selectOption("identity.password_changed");
  await auditTrail.getByRole("button", { name: "应用审计筛选" }).click();
  await expect(auditTrail.getByText(/1 条只追加事件/)).toBeVisible();

  await page.locator('a[href="/workspaces"]').click();
  await page.getByLabel("输入 DEACTIVATE 确认").fill("DEACTIVATE");
  await page.getByRole("button", { name: "停用账户" }).click();
  await expect(page).toHaveURL(/\/login$/);

  await page.getByLabel("邮箱").fill(email);
  await page.getByLabel("密码").fill(secondPassword);
  await page.getByRole("button", { name: "登录" }).click();
  await expect(page.getByText("邮箱或密码不正确，请重新输入。")).toBeVisible();
});
