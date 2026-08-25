import { readFileSync } from "node:fs";
import path from "node:path";

import { expect, test } from "@playwright/test";

import { registerUser } from "./auth-support";

test("整剧经制作圣经、分集、结构提取和人工审核生成正式分镜", async ({ page }) => {
  test.setTimeout(10_800_000);
  const codexStageTimeout = 5_400_000;
  const unique = `${Date.now()}-${test.info().workerIndex}`;
  const projectName = `MVP-A-分集计划-${unique}`;
  const fixture = JSON.parse(readFileSync(
    path.resolve(
      process.cwd(),
      "../agent/tests/fixtures/mvp_a/golden_candidate_harbor_countdown.json",
    ),
    "utf8",
  )) as { full_script: string };

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
  await importCard.getByLabel("剧本文档").setInputFiles({
    name: "golden-candidate.md",
    mimeType: "text/markdown",
    buffer: Buffer.from(fixture.full_script, "utf8"),
  });
  await importCard.getByRole("button", { name: "上传并预览" }).click();
  await expect(
    importCard.getByRole("region", { name: "剧本内容预览" }),
  ).toContainText(fixture.full_script.slice(0, 24));
  await importCard
    .getByRole("button", { name: "确认剧本并开始解析" })
    .click();
  await expect(importCard.getByRole("status")).toContainText(
    "剧本已固定为不可变修订",
  );
  await expect(page.getByRole("link", { name: /进入第/ })).toHaveCount(0);

  const productionBible = page.getByRole("region", {
    name: "项目制作圣经",
  });
  await productionBible
    .getByRole("button", { name: "生成项目制作圣经" })
    .click();
  await expect(
    productionBible.getByRole("button", { name: "确认制作圣经" }),
  ).toBeVisible({ timeout: codexStageTimeout });
  await expect(
    productionBible.getByRole("region", { name: "制作圣经实体" }),
  ).not.toBeEmpty();
  await productionBible
    .getByRole("button", { name: "确认制作圣经" })
    .click();
  await expect(productionBible.getByText("制作圣经已确认", { exact: true })).toBeVisible({
    timeout: 30_000,
  });

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
  const episodeWorkspace = page.getByRole("region", { name: "单集工作区" });
  await expect(episodeWorkspace.getByRole("link", { name: /进入/ })).toHaveCount(5);

  await page.reload();
  const reloadedEpisodeWorkspace = page.getByRole("region", { name: "单集工作区" });
  await expect(reloadedEpisodeWorkspace.getByRole("link", { name: "进入警报前夜" })).toBeVisible();
  await expect(reloadedEpisodeWorkspace.getByRole("link", { name: "进入公开日志" })).toBeVisible();

  await reloadedEpisodeWorkspace
    .getByRole("link", { name: "进入警报前夜" })
    .click();
  await expect(page).toHaveURL(/\/studio\/[^/]+\/script$/);
  await expect(page.getByText(/^\d+ 项建议 · 已完成$/)).toBeVisible({
    timeout: codexStageTimeout,
  });

  const productionTaskRegions = page.locator(
    'section[aria-label$="制作任务"]',
  );
  await expect(productionTaskRegions.first()).toBeVisible();

  const pendingRequiredCandidates = page
    .locator("article")
    .filter({ hasText: "必需" })
    .filter({ hasText: "pending" });
  while ((await pendingRequiredCandidates.count()) > 0) {
    const before = await pendingRequiredCandidates.count();
    await pendingRequiredCandidates
      .first()
      .getByRole("button", { name: "接受", exact: true })
      .click();
    await expect(pendingRequiredCandidates).toHaveCount(before - 1, {
      timeout: 30_000,
    });
  }

  await page.getByRole("button", { name: "确认剧本结构" }).click();
  await expect(page.getByText("稳定叙事单元", { exact: true })).toBeVisible({
    timeout: 30_000,
  });
  await expect(page.getByText(/结构已确认，生成剧本 v/)).toBeVisible();
  await page.getByRole("button", { name: "使用确认版本" }).click();
  const versionImpact = page.getByRole("dialog", { name: "版本切换影响" });
  await expect(versionImpact).toContainText("当前指针已经切换", {
    timeout: 30_000,
  });
  await versionImpact.getByRole("button", { name: "知道了" }).click();

  await page.getByRole("link", { name: /分镜设计/ }).click();
  await expect(page).toHaveURL(/\/studio\/[^/]+\/storyboard$/);
  const createDraft = page.getByRole("button", { name: "生成待审核草案" });
  await expect(createDraft).toBeEnabled({ timeout: 30_000 });
  await createDraft.click();

  const acceptDraftButtons = page.getByRole("button", { name: "接受此镜" });
  await expect(acceptDraftButtons.first()).toBeVisible({ timeout: codexStageTimeout });
  const draftCount = await acceptDraftButtons.count();
  const acceptedDecisions = page.getByText("accepted", { exact: true });
  for (let index = 0; index < draftCount; index += 1) {
    await acceptDraftButtons.nth(index).click();
    await expect(acceptedDecisions).toHaveCount(index + 1, { timeout: 30_000 });
  }

  await page.getByRole("button", { name: "批准整批草案" }).click();
  await page.getByRole("button", { name: "预检写入影响" }).click();
  const applyDraft = page.getByRole("button", { name: "原子写入正式分镜" });
  await expect(applyDraft).toBeEnabled({ timeout: 30_000 });
  await applyDraft.click();
  await expect(
    page.getByRole("status").filter({ hasText: /已原子写入 \d+ 个正式镜头/ }),
  ).toBeVisible();
  await expect(
    page.getByLabel("分镜准备度摘要").getByText(/[1-9]\d* 个镜头/),
  ).toBeVisible();

  await page.getByRole("button", { name: "检查导出条件" }).click();
  const exportPreflight = page.getByLabel("分镜包预检结果");
  await expect(exportPreflight).toContainText("预检结果：存在内容阻断", {
    timeout: 30_000,
  });
  await expect(exportPreflight).toContainText(/ASSET_NOT_READY|MEDIA_REFERENCE_UNAVAILABLE/);
  await expect(page.getByRole("button", { name: "生成分镜包" })).toBeDisabled();
});
