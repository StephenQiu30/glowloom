import { expect, test } from "@playwright/test";

test("S1 用户管理资料、工作空间和账户凭据", async ({ page }) => {
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

  await page.getByRole("link", { name: "账户与工作空间" }).click();
  await expect(page.getByRole("heading", { name: "账户与工作空间" })).toBeVisible();

  await page.getByLabel("显示名称").fill("更新后的管理员");
  await page.getByRole("button", { name: "保存个人资料" }).click();
  await expect(page.getByText("个人资料已更新。")).toBeVisible();

  await page.getByLabel("工作空间名称").fill("第二创作空间");
  await page.getByRole("button", { name: "创建工作空间" }).click();
  await expect(page.getByText("第二创作空间", { exact: true })).toBeVisible();

  await page.getByLabel("重命名 第二创作空间").fill("正式创作空间");
  await page.getByRole("button", { name: "保存 第二创作空间" }).click();
  await expect(page.getByText("正式创作空间", { exact: true })).toBeVisible();

  await page.getByRole("button", { name: "归档 正式创作空间" }).click();
  await expect(page.getByRole("button", { name: "恢复 正式创作空间" })).toBeVisible();
  await page.getByRole("button", { name: "恢复 正式创作空间" }).click();
  await expect(page.getByRole("button", { name: "归档 正式创作空间" })).toBeVisible();

  await page.getByRole("link", { name: "打开项目" }).last().click();
  await expect(page.getByLabel("当前工作空间")).toHaveValue(/.+/);
  await expect(page.getByLabel("当前工作空间").locator("option:checked")).toHaveText(
    "正式创作空间",
  );

  await page.getByRole("link", { name: "账户与工作空间" }).click();
  await page.getByLabel("当前密码").fill(firstPassword);
  await page.getByLabel("新密码").fill(secondPassword);
  await page.getByRole("button", { name: "修改密码" }).click();
  await expect(page).toHaveURL(/\/login$/);

  await page.getByLabel("邮箱").fill(email);
  await page.getByLabel("密码").fill(secondPassword);
  await page.getByRole("button", { name: "登录" }).click();
  await expect(page).toHaveURL(/\/projects$/);

  await page.getByRole("link", { name: "账户与工作空间" }).click();
  await page.getByLabel("输入 DEACTIVATE 确认").fill("DEACTIVATE");
  await page.getByRole("button", { name: "停用账户" }).click();
  await expect(page).toHaveURL(/\/login$/);

  await page.getByLabel("邮箱").fill(email);
  await page.getByLabel("密码").fill(secondPassword);
  await page.getByRole("button", { name: "登录" }).click();
  await expect(page.getByText("邮箱或密码不正确，请重新输入。")).toBeVisible();
});
