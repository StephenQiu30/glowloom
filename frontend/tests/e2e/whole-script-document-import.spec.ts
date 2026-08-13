import path from "node:path";

import { expect, test } from "@playwright/test";

import { registerUser } from "./auth-support";

test("上传整剧样本并恢复确定性格式体检结果", async ({ page }) => {
  test.setTimeout(120_000);
  const unique = `${Date.now()}-${test.info().workerIndex}`;
  const projectName = `MVP-A-整剧导入-${unique}`;
  const samplePath = path.resolve(
    process.cwd(),
    "../docs/fixtures/mvp_a/002-雾港倒计时整剧导入样例.txt",
  );

  await registerUser(page, {
    displayName: "整剧导入验收创作者",
    email: `whole-script-${unique}@example.com`,
  });

  await page.getByRole("button", { name: "创建项目" }).click();
  await page.getByLabel("项目名称").fill(projectName);
  await page.getByRole("button", { name: "确认创建" }).click();
  await page.getByRole("link", { name: `打开项目 ${projectName}` }).click();

  const importCard = page.getByRole("region", {
    name: "整剧导入与格式体检",
  });
  await importCard.getByLabel("导入方式").selectOption("media");
  await importCard.getByLabel("剧本文档").setInputFiles(samplePath);
  await importCard
    .getByLabel("我确认拥有该剧本用于本项目制作与分析的权利")
    .check();
  await importCard.getByRole("button", { name: "上传并分析" }).click();

  await expect(importCard.getByRole("status")).toContainText(
    "整剧原稿已保存为不可变修订",
    { timeout: 30_000 },
  );
  await expect(importCard.getByText("可确定性分集")).toBeVisible();
  await expect(importCard.getByText("集标记").locator("..")).toContainText("5");
  await expect(importCard.getByText("集号连续且没有阻断问题")).toBeVisible();
  await expect(importCard.getByText(`${projectName} · 整剧原稿`)).toBeVisible();
  await expect(importCard.getByText("文件", { exact: true })).toBeVisible();
  await expect(page.getByRole("link", { name: /进入第/ })).toHaveCount(0);

  await page.reload();
  await expect(
    page.getByRole("region", { name: "整剧导入与格式体检" }).getByText(
      `${projectName} · 整剧原稿`,
    ),
  ).toBeVisible();
  await expect(page.getByRole("link", { name: /进入第/ })).toHaveCount(0);
});
