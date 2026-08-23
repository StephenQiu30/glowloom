import { expect, test } from "@playwright/test";

import { registerUser } from "./auth-support";

test("首次登录后创建项目并恢复项目工作流事实", async ({ page }) => {
  const unique = `${Date.now()}-${test.info().workerIndex}`;
  const email = `creator-${unique}@example.com`;
  const projectName = `海边来信-${unique}`;

  await registerUser(page, { displayName: "验收创作者", email });

  await expect(
    page.getByRole("heading", { name: "项目管理", exact: true }),
  ).toBeVisible();

  await page.getByRole("button", { name: "创建项目" }).click();
  await page.getByLabel("项目名称").fill(projectName);
  await page.getByLabel("项目简介").fill("用于验证 S1 纵向业务闭环");
  await page.getByRole("button", { name: "确认创建" }).click();

  const projectLink = page.getByRole("link", { name: `打开项目 ${projectName}` });
  await expect(projectLink).toBeVisible();
  const projectHref = await projectLink.getAttribute("href");
  expect(projectHref).not.toBeNull();
  await projectLink.click();

  await expect(page.getByRole("heading", { name: projectName })).toBeVisible();
  await expect(
    page.getByRole("region", { name: "整剧导入与格式体检" }),
  ).toBeVisible();
  await expect(page.getByRole("button", { name: "上传并预览" })).toBeDisabled();

  await page.reload();
  await expect(page.getByRole("heading", { name: projectName })).toBeVisible();
  await expect(
    page.getByRole("region", { name: "整剧导入与格式体检" }),
  ).toBeVisible();

  await page.getByRole("button", { name: "验收创作者" }).click();
  await page.getByRole("menuitem", { name: "退出登录" }).click();
  await expect(page).toHaveURL(/\/login$/);
  await page.goto(projectHref!);
  await expect(
    page.getByRole("heading", { name: "需要登录后继续" }),
  ).toBeVisible();
  await expect(page.getByRole("link", { name: "前往登录" })).toHaveAttribute(
    "href",
    "/login",
  );
  await expect(page.getByRole("heading", { name: projectName })).toHaveCount(0);
});
