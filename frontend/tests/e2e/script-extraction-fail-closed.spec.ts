import { expect, test } from "@playwright/test";

import { registerUser } from "./auth-support";
import { chooseOption } from "./select-support";

test("导入发布剧本并在 DeepSeek 未配置时可恢复地失败", async ({ page }) => {
  test.setTimeout(60_000);
  const unique = `${Date.now()}-${test.info().workerIndex}`;
  const projectName = `S2-剧本契约-${unique}`;

  await registerUser(page, {
    displayName: "S2 验收创作者",
    email: `s2-script-${unique}@example.com`,
  });

  await page.getByRole("button", { name: "创建项目" }).click();
  await page.getByLabel("项目名称").fill(projectName);
  await page.getByRole("button", { name: "确认创建" }).click();
  await page.getByRole("link", { name: `打开项目 ${projectName}` }).click();
  await page.getByRole("button", { name: /创建(?:第一集|单集)/ }).click();
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
  await expect(page.getByText("提取未完成", { exact: true })).toBeVisible({
    timeout: 20_000,
  });
  await expect(
    page.getByText("AI extraction service is not configured"),
  ).toBeVisible();

  await page.reload();
  await expect(page.getByText("提取未完成", { exact: true })).toBeVisible({
    timeout: 15_000,
  });
  await expect(page.getByRole("button", { name: "重新提取结构" })).toBeVisible();

  await page.goto("/projects");
  await page.getByRole("link", { name: `打开项目 ${projectName}` }).click();
  await page.getByRole("button", { name: "检查删除 第一集 雨夜" }).click();
  await expect(page.getByText("单集已有 2 个剧本版本")).toBeVisible();
  await expect(page.getByText("单集已有 1 个任务")).toBeVisible();
  await page.getByRole("button", { name: "检查项目删除条件" }).click();
  await expect(page.getByText("项目包含 1 个单集")).toBeVisible();
  await expect(page.getByText("项目关联 2 个剧本版本")).toBeVisible();
  await expect(page.getByText("项目关联 1 个任务")).toBeVisible();

  await page.goto("/governance");
  const auditTrail = page.getByRole("region", { name: "操作审计" });
  await expect(auditTrail.getByText("剧本初始版本创建")).toBeVisible();
  await expect(auditTrail.getByText("剧本版本发布")).toBeVisible();
  await expect(auditTrail.getByText("任务创建")).toBeVisible();
  await expect(auditTrail.getByText("任务失败")).toBeVisible();
  await auditTrail.getByRole("button", { name: "筛选" }).click();
  await chooseOption(auditTrail.getByLabel("动作"), "任务失败");
  await auditTrail.getByRole("button", { name: "应用审计筛选" }).click();
  await expect(auditTrail.getByText(/1 条只追加事件/)).toBeVisible();
  await expect(auditTrail.getByText("queued → failed")).toBeVisible();
});
