import { expect, test } from "@playwright/test";

import { registerUser } from "./auth-support";

test("创作者管理项目和单集完整生命周期", async ({ page }) => {
  test.setTimeout(60_000);
  const unique = `${Date.now()}-${test.info().workerIndex}`;
  const email = `project-lifecycle-${unique}@example.com`;
  const projectName = `生命周期项目-${unique}`;

  await registerUser(page, { displayName: "项目管理员", email });

  await page.getByRole("button", { name: "创建项目" }).click();
  await page.getByLabel("项目名称").fill(projectName);
  await page.getByRole("button", { name: "确认创建" }).click();
  await page.getByRole("link", { name: `打开项目 ${projectName}` }).click();

  await page.getByRole("button", { name: /创建(?:第一集|单集)/ }).click();
  await page.getByLabel("单集名称", { exact: true }).fill("第一集");
  await page.getByRole("button", { name: "确认创建" }).click();
  await expect(page.getByRole("link", { name: "进入第一集", exact: true })).toBeVisible();
  await page.getByRole("button", { name: /创建(?:第一集|单集)/ }).click();
  await page.getByLabel("单集名称", { exact: true }).fill("第二集");
  await page.getByRole("button", { name: "确认创建" }).click();
  await expect(page.getByRole("link", { name: "进入第二集", exact: true })).toBeVisible();

  await page.getByLabel("项目名称").fill(`${projectName}-更新`);
  await page.getByLabel("项目简介").fill("已完成生命周期编辑");
  await page.getByRole("button", { name: "保存项目信息" }).click();
  await expect(page.getByRole("heading", { name: `${projectName}-更新` })).toBeVisible();

  await page.getByLabel("预算上限").fill("99.99");
  await page.getByRole("button", { name: "更新预算" }).click();
  await expect(page.getByText("项目预算已更新。")).toBeVisible();

  await page.getByLabel("单集名称 第一集").fill("开端");
  await page.getByLabel("目标秒数").nth(1).fill("105");
  await page.getByRole("button", { name: "保存 第一集" }).click();
  await expect(page.getByRole("link", { name: "进入开端", exact: true })).toBeVisible();

  await page.getByRole("button", { name: "下移 开端" }).click();
  await expect(page.getByText("第 2 集 · 105 秒")).toBeVisible();

  await page.getByRole("button", { name: "归档 开端" }).click();
  await expect(page.getByRole("button", { name: "恢复 开端" })).toBeVisible();
  await page.getByRole("button", { name: "恢复 开端" }).click();
  await expect(page.getByRole("button", { name: "归档 开端" })).toBeVisible();

  await page.getByRole("button", { name: "归档项目" }).click();
  await expect(page.getByRole("button", { name: "恢复项目" })).toBeVisible();
  await page.getByRole("button", { name: "恢复项目" }).click();
  await expect(page.getByRole("button", { name: "归档项目" })).toBeVisible();

  await page.getByRole("button", { name: "检查项目删除条件" }).click();
  await expect(page.getByText("项目包含 2 个单集")).toBeVisible();

  await page.getByRole("button", { name: "检查删除 开端" }).click();
  await page.getByRole("button", { name: "确认删除 开端" }).click();
  await expect(page.getByText("开端", { exact: true })).toHaveCount(0);

  await page.getByRole("button", { name: "检查删除 第二集" }).click();
  await page.getByRole("button", { name: "确认删除 第二集" }).click();
  await expect(page.getByText("还没有单集")).toBeVisible();

  await page.getByRole("button", { name: "检查项目删除条件" }).click();
  await page.getByRole("button", { name: "确认删除项目" }).click();
  await expect(page).toHaveURL(/\/projects$/);
  await expect(page.getByRole("link", { name: `打开项目 ${projectName}-更新` })).toHaveCount(0);

  await page.goto("/governance");
  const auditTrail = page.getByRole("region", { name: "操作审计" });
  await expect(auditTrail.getByText("项目预算更新")).toBeVisible();
  await expect(auditTrail.getByText("单集排序")).toBeVisible();
  await expect(auditTrail.getByText("项目删除")).toBeVisible();
  await expect(auditTrail.getByText("单集删除").first()).toBeVisible();
  await auditTrail.getByRole("button", { name: "筛选" }).click();
  await auditTrail.getByLabel("动作").selectOption("episode.deleted");
  await auditTrail.getByRole("button", { name: "应用审计筛选" }).click();
  await expect(auditTrail.getByText(/2 条只追加事件/)).toBeVisible();
  await expect(
    auditTrail.locator("article").first().getByText("单集删除"),
  ).toBeVisible();
  await auditTrail.getByLabel("动作").selectOption("project.deleted");
  await auditTrail.getByRole("button", { name: "应用审计筛选" }).click();
  await expect(auditTrail.getByText(/1 条只追加事件/)).toBeVisible();
  await expect(
    auditTrail.locator("article").first().getByText("项目删除"),
  ).toBeVisible();
});
