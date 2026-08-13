import { readFileSync } from "node:fs";
import path from "node:path";

import { expect, test } from "@playwright/test";

import { registerUser } from "./auth-support";

test("审阅五集计划后原子创建并批量发布单集剧本", async ({ page }) => {
  test.setTimeout(120_000);
  const unique = `${Date.now()}-${test.info().workerIndex}`;
  const projectName = `MVP-A-分集计划-${unique}`;
  const sample = readFileSync(
    path.resolve(
      process.cwd(),
      "../docs/fixtures/mvp_a/002-雾港倒计时整剧导入样例.txt",
    ),
    "utf8",
  );

  await registerUser(page, {
    displayName: "分集计划验收创作者",
    email: `episode-plan-${unique}@example.com`,
  });
  await page.getByRole("button", { name: "创建项目" }).click();
  await page.getByLabel("项目名称").fill(projectName);
  await page.getByRole("button", { name: "确认创建" }).click();
  await page.getByRole("link", { name: `打开项目 ${projectName}` }).click();

  const importCard = page.getByRole("region", {
    name: "整剧导入与格式体检",
  });
  await importCard.getByLabel("整剧文本").fill(sample);
  await importCard
    .getByLabel("我确认拥有该剧本用于本项目制作与分析的权利")
    .check();
  await importCard.getByRole("button", { name: "导入并分析" }).click();
  await expect(importCard.getByRole("status")).toContainText(
    "整剧原稿已保存为不可变修订",
  );
  await expect(page.getByRole("link", { name: /进入第/ })).toHaveCount(0);

  const planner = page.getByRole("region", {
    name: "分集计划与批量创建",
  });
  await planner
    .getByRole("button", { name: "生成确定性分集计划" })
    .click();
  await expect(planner.getByText("候选集数").locator("..")).toContainText("5");
  await expect(
    planner.getByRole("textbox", { name: "第 1 集标题" }),
  ).toHaveValue("警报前夜");
  await expect(
    planner.getByRole("textbox", { name: "第 5 集标题" }),
  ).toHaveValue("公开日志");
  await expect(planner.getByText("置信度 100%")).toHaveCount(5);
  await expect(planner.getByText(/沈岚把铜制检修钥匙插进手动井/)).toHaveCount(2);
  await expect(page.getByRole("link", { name: /进入第/ })).toHaveCount(0);

  await planner.getByRole("button", { name: "确认分集计划" }).click();
  await expect(planner.getByText("已确认", { exact: true })).toBeVisible();
  await planner.getByRole("button", { name: "原子创建 5 集" }).click();
  await planner.getByRole("button", { name: "发布 5 集剧本" }).click();
  await expect(planner.getByRole("status")).toContainText("5 集剧本已批量发布");
  await expect(page.getByRole("link", { name: /进入/ })).toHaveCount(5);

  await page.reload();
  await expect(page.getByRole("link", { name: "进入警报前夜" })).toBeVisible();
  await expect(page.getByRole("link", { name: "进入公开日志" })).toBeVisible();
});
