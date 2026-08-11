import { expect, test } from "@playwright/test";

import {
  ONE_PIXEL_PNG,
  createReadyAsset,
  testToneWav,
  uploadAndWait,
  type AssetFixture,
} from "./asset-support";
import { registerUser } from "./auth-support";

test("真实 DeepSeek 提取、人工决议并确认结构", async ({ page }) => {
  test.setTimeout(360_000);
  const unique = `${Date.now()}-${test.info().workerIndex}`;
  const projectName = `S2-DeepSeek-联合契约-${unique}`;
  const characterMediaName = `deepseek-character-${unique}.png`;
  const locationMediaName = `deepseek-location-${unique}.png`;
  const voiceMediaName = `deepseek-voice-${unique}.wav`;
  const characterFixture: AssetFixture = {
    kind: "character",
    tabName: "角色",
    name: `顾清禾角色-${unique}`,
    mediaName: characterMediaName,
    consentReference: `fictional-deepseek-character-${unique}`,
  };
  const locationFixture: AssetFixture = {
    kind: "location",
    tabName: "场景",
    name: `雨巷场景-${unique}`,
    mediaName: locationMediaName,
    consentReference: `fictional-deepseek-location-${unique}`,
  };
  const voiceFixture: AssetFixture = {
    kind: "voice",
    tabName: "声音",
    name: `顾清禾声线-${unique}`,
    mediaName: voiceMediaName,
    consentReference: `fictional-deepseek-voice-${unique}`,
  };

  await registerUser(page, {
    displayName: "S2 DeepSeek 验收创作者",
    email: `s2-deepseek-${unique}@example.com`,
  });

  await page.getByRole("button", { name: "创建项目" }).click();
  await page.getByLabel("项目名称").fill(projectName);
  await page.getByRole("button", { name: "确认创建" }).click();
  await page.getByRole("link", { name: `打开项目 ${projectName}` }).click();
  await page.getByRole("button", { name: "创建单集" }).click();
  await page.getByLabel("单集名称", { exact: true }).fill("第一集 雨巷来信");
  await page.getByRole("button", { name: "确认创建" }).click();
  const episodeLink = page.getByRole("link", { name: "进入第一集 雨巷来信" });
  const episodeHref = await episodeLink.getAttribute("href");
  expect(episodeHref).toMatch(/^\/studio\/[0-9a-f-]+\/script$/);
  const episodeId = episodeHref!.split("/")[2];
  await episodeLink.click();

  await page.getByLabel("剧本标题").fill("雨巷来信");
  await page.getByLabel("权利声明").fill("原创验收文本，仅用于本项目制作");
  await page.getByLabel("剧本文本").fill(
    [
      "第一场 长安雨巷 夜",
      "顾清禾撑伞站在灯笼下。",
      "顾清禾：你终于来了。",
      "陆沉舟：这封信不能见光。",
      "第二场 旧书铺 内 夜",
      "陆沉舟把信放在桌上。",
      "顾清禾：明天之前，我会给你答案。",
    ].join("\n"),
  );
  await page.getByRole("button", { name: "导入剧本" }).click();
  await expect(page.getByRole("status")).toContainText("已导入为 v1 草稿");
  await page.getByRole("button", { name: "发布新版本" }).click();
  await expect(page.getByRole("status")).toContainText("已发布并设为当前版本");

  await page.getByRole("button", { name: "开始结构提取" }).click();
  await expect(page.getByRole("status")).toContainText("提取任务已创建");
  const candidateCard = page
    .locator('[data-slot="card"]')
    .filter({ hasText: "提取候选" });
  const terminalProviderStatus = candidateCard.getByText(
    /项建议 · (已完成|失败|待对账)/,
  );
  await expect(terminalProviderStatus).toBeVisible({ timeout: 150_000 });
  if (!(await terminalProviderStatus.textContent())?.includes("已完成")) {
    await expect(page.getByText("提取未完成", { exact: true })).toBeVisible();
    await page.getByRole("button", { name: "重新提取结构" }).click();
    await expect(page.getByRole("status")).toContainText("提取任务已创建");
  }
  await expect(candidateCard.getByText(/项建议 · 已完成/)).toBeVisible({
    timeout: 150_000,
  });

  const sceneCandidate = candidateCard
    .locator("article")
    .filter({ hasText: "场次" })
    .first();
  const dialogueCandidate = candidateCard
    .locator("article")
    .filter({ hasText: "对白" })
    .first();
  await expect(sceneCandidate).toBeVisible();
  await expect(dialogueCandidate).toBeVisible();

  await sceneCandidate
    .getByRole("button", { name: /修改 .* 后接受/ })
    .click();
  await page.getByLabel("候选标题").fill("第一场 · 雨巷（人工校正）");
  await page.getByLabel("候选说明").fill("顾清禾与陆沉舟在雨巷交换不能见光的信件。");
  await page.getByRole("button", { name: "保存并接受" }).click();
  await expect(page.getByRole("status")).toContainText("已完成决议");
  await expect(sceneCandidate.getByText("accepted", { exact: true })).toBeVisible();

  const pendingRequired = candidateCard
    .locator("article")
    .filter({ hasText: "必需" })
    .filter({ hasText: "pending" });
  while ((await pendingRequired.count()) > 0) {
    const count = await pendingRequired.count();
    await pendingRequired
      .first()
      .getByRole("button", { name: "接受", exact: true })
      .click();
    await expect(pendingRequired).toHaveCount(count - 1);
  }

  await page.getByRole("button", { name: "确认剧本结构" }).click();
  await expect(page.getByRole("status")).toContainText("结构已确认，生成剧本");
  await page.getByRole("button", { name: "使用确认版本" }).click();
  const impactDialog = page.getByRole("dialog", { name: "版本切换影响" });
  await expect(impactDialog).toBeVisible();
  await expect(impactDialog.getByText("0 个镜头仍引用其他剧本版本")).toBeVisible();
  await impactDialog.getByRole("button", { name: "知道了" }).click();

  await page.reload();
  await expect(page.getByText("结构已确认", { exact: true })).toBeVisible({
    timeout: 15_000,
  });
  await expect(page.getByLabel("当前剧本文本")).toHaveValue(/明天之前/);

  await page.goto(`/studio/${episodeId}/media`);
  await uploadAndWait(page, {
    name: characterMediaName,
    mimeType: "image/png",
    buffer: ONE_PIXEL_PNG,
  });
  await uploadAndWait(page, {
    name: locationMediaName,
    mimeType: "image/png",
    buffer: ONE_PIXEL_PNG,
  });
  await uploadAndWait(page, {
    name: voiceMediaName,
    mimeType: "audio/wav",
    buffer: testToneWav(),
  });
  await createReadyAsset(page, characterFixture);
  await createReadyAsset(page, voiceFixture);
  await createReadyAsset(page, locationFixture);

  await page.goto(`/studio/${episodeId}/storyboard`);
  await expect(page.getByRole("heading", { name: "分镜设计" })).toBeVisible();
  await page.getByRole("button", { name: "新建镜头" }).click();
  await page.getByLabel("新镜头标题").fill("雨巷交接");
  await page.getByRole("button", { name: "创建空镜头" }).click();
  await expect(page.getByRole("status")).toContainText("镜头“雨巷交接”已加入清单");

  await page.getByLabel("镜头目的").fill("建立顾清禾与陆沉舟交换密信的关键情节");
  await page.getByLabel("时长（毫秒）").fill("4200");
  await page.getByLabel("连续性备注").fill("承接雨巷等待动作，保持人物视线方向");
  await page.getByRole("combobox", { name: "景别" }).click();
  await page.getByRole("option", { name: "中近景" }).click();
  await page.getByRole("combobox", { name: "机位" }).click();
  await page.getByRole("option", { name: "平视" }).click();
  await page.getByRole("combobox", { name: "运镜" }).click();
  await page.getByRole("option", { name: "固定" }).click();
  await page.getByLabel("构图").fill("顾清禾位于右侧三分线，陆沉舟从雨幕中进入");
  await page.getByLabel("环境", { exact: true }).fill("长安雨巷夜景，灯笼映照湿润青石路");
  await page.getByLabel("情绪与光线").fill("冷雨与暖灯形成秘密会面的紧张反差");
  await page.getByLabel("动作节拍").fill("顾清禾停步\n陆沉舟递出密信");
  await page.getByRole("button", { name: /顾清禾.*你终于来了/ }).click();
  const fixedAssets = page.getByRole("region", { name: "固定资产版本" });
  await fixedAssets
    .getByRole("button", { name: characterFixture.name, exact: true })
    .click();
  await page
    .getByLabel(`${characterFixture.name}画面位置`)
    .fill("画面右侧，面向陆沉舟");
  await fixedAssets
    .getByRole("button", { name: voiceFixture.name, exact: true })
    .click();
  await page
    .getByRole("button", {
      name: `为对白 顾清禾 选择声音 ${voiceFixture.name}`,
    })
    .click();
  await page.getByLabel("顾清禾表演提示").fill("克制地确认来人身份");
  await fixedAssets
    .getByRole("button", { name: locationFixture.name, exact: true })
    .click();
  await page.getByLabel("环境声").fill("持续雨声与远处更鼓");
  await page.getByLabel("音效（逗号分隔）").fill("伞面雨滴，脚步声");
  await page.getByRole("combobox", { name: "生成方式" }).click();
  await page.getByRole("option", { name: "参考图转视频" }).click();
  await page.getByLabel("关键帧备注").fill("固定角色与雨巷资产版本");
  await page.getByLabel("首帧意图").fill("顾清禾独自站在灯笼下等待");
  await page.getByLabel("尾帧意图").fill("陆沉舟递出不能见光的信");
  await page.getByRole("button", { name: "保存为新版本" }).click();
  await expect(page.getByRole("status")).toContainText("镜头规格 v1 已保存");
  await expect(page.getByText("当前规格可进入生产预检")).toBeVisible();

  const productionSummary = page.getByRole("region", { name: "生产摘要" });
  await expect(
    productionSummary.getByText("Ready 分镜").locator(".."),
  ).toContainText("1 / 1");
  await expect(page.getByText(/服务端计算 90%/)).toBeVisible();
});
