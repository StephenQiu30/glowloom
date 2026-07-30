import { expect, test } from "@playwright/test";

test("S2 导入发布剧本并在 DeepSeek 未配置时可恢复地失败", async ({ page }) => {
  test.setTimeout(60_000);
  const unique = `${Date.now()}-${test.info().workerIndex}`;
  const projectName = `S2-剧本契约-${unique}`;

  await page.goto("/register");
  await page.waitForLoadState("networkidle");
  await page.getByLabel("显示名称").fill("S2 验收创作者");
  await page.getByLabel("邮箱").fill(`s2-script-${unique}@example.com`);
  await page.getByLabel("密码").fill("playwright-secure-password");
  await page.getByRole("button", { name: "注册并开始创作" }).click();

  await page.getByRole("button", { name: "创建项目" }).click();
  await page.getByLabel("项目名称").fill(projectName);
  await page.getByRole("button", { name: "确认创建" }).click();
  await page.getByRole("link", { name: `打开项目 ${projectName}` }).click();
  await page.getByRole("button", { name: "创建单集" }).click();
  await page.getByLabel("单集名称", { exact: true }).fill("第一集 雨夜");
  await page.getByRole("button", { name: "确认创建" }).click();
  await page.getByRole("link", { name: "进入第一集 雨夜" }).click();

  await page.getByLabel("剧本标题").fill("雨夜来信");
  await page.getByLabel("权利声明").fill("原创验收文本，仅用于本项目制作");
  await page
    .getByLabel("剧本文本")
    .fill("第一场 雨巷 夜\n顾清禾：你终于来了。\n陆沉舟：这封信不能见光。");
  await page.getByRole("button", { name: "导入剧本" }).click();

  await expect(page.getByRole("status")).toContainText("已导入为 v1 草稿");
  await expect(page.getByLabel("当前剧本文本")).toHaveValue(
    /这封信不能见光/,
  );
  await page.getByRole("button", { name: "发布新版本" }).click();
  await expect(page.getByRole("status")).toContainText("已发布并设为当前版本");

  await page.getByRole("button", { name: "开始结构提取" }).click();
  await expect(page.getByRole("status")).toContainText("提取任务已创建");
  await expect(
    page.getByText("提取未完成", { exact: true }),
  ).toBeVisible({ timeout: 20_000 });
  await expect(
    page.getByText("AI extraction service is not configured"),
  ).toBeVisible();

  await page.reload();
  await expect(
    page.getByText("提取未完成", { exact: true }),
  ).toBeVisible({ timeout: 15_000 });
  await expect(page.getByRole("button", { name: "重新提取结构" })).toBeVisible();
});
