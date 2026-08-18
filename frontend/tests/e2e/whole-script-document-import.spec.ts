import { readFileSync } from "node:fs";
import path from "node:path";

import { expect, test } from "@playwright/test";

import { registerUser } from "./auth-support";
test("上传整剧样本并恢复确定性格式体检结果", async ({ page }) => {
  test.setTimeout(120_000);
  const unique = `${Date.now()}-${test.info().workerIndex}`;
  const projectName = `MVP-A-整剧导入-${unique}`;
  const sampleFixturePath = path.resolve(
    process.cwd(),
    "../backend/tests/fixtures/mvp_a/golden_candidate_harbor_countdown.json",
  );
  const sample = JSON.parse(readFileSync(sampleFixturePath, "utf8")) as {
    full_script: string;
  };

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
  await importCard.getByLabel("剧本文档").setInputFiles({
    name: "golden-candidate.md",
    mimeType: "text/markdown",
    buffer: Buffer.from(sample.full_script, "utf8"),
  });
  await importCard
    .getByLabel("我确认拥有该剧本用于本项目制作与分析的权利")
    .check();
  await importCard.getByRole("button", { name: "上传并预览" }).click();

  const preview = importCard.getByRole("region", { name: "剧本内容预览" });
  await expect(preview).toContainText(sample.full_script.slice(0, 24), {
    timeout: 30_000,
  });
  await expect(page.getByRole("link", { name: /进入第/ })).toHaveCount(0);

  await importCard
    .getByRole("button", { name: "确认剧本并开始解析" })
    .click();

  await expect(importCard.getByRole("status")).toContainText(
    "剧本已固定为不可变修订",
    { timeout: 30_000 },
  );
  await expect(importCard.getByText("可确定性分集")).toBeVisible();
  await expect(importCard.getByText("集标记").locator("..")).toContainText("5");
  await expect(importCard.getByText("集号连续且没有阻断问题")).toBeVisible();
  await expect(importCard.getByText("golden-candidate.md")).toBeVisible();
  await expect(importCard.getByText("文件", { exact: true })).toBeVisible();
  await expect(page.getByRole("link", { name: /进入第/ })).toHaveCount(0);

  await page.reload();
  await expect(
    page.getByRole("region", { name: "整剧导入与格式体检" }).getByText(
      "golden-candidate.md",
    ),
  ).toBeVisible();
  await expect(page.getByRole("link", { name: /进入第/ })).toHaveCount(0);
});
