import { expect, test } from "@playwright/test";

import {
  ONE_PIXEL_PNG,
  createReadyAsset,
  fillAssetSpec,
  testToneWav,
  uploadAndWait,
  type AssetFixture,
} from "./asset-support";

test("S2 媒体、三类资产与授权准备度联合闭环", async ({ page }) => {
  test.setTimeout(120_000);
  const unique = `${Date.now()}-${test.info().workerIndex}`;
  const projectName = `S2-资产契约-${unique}`;
  const characterMediaName = `character-${unique}.png`;
  const locationMediaName = `location-${unique}.png`;
  const voiceMediaName = `voice-${unique}.wav`;

  await page.goto("/register");
  await page.getByLabel("显示名称").fill("S2 资产验收创作者");
  await page.getByLabel("邮箱").fill(`s2-assets-${unique}@example.com`);
  await page.getByLabel("密码").fill("playwright-secure-password");
  await page.getByRole("button", { name: "注册并开始创作" }).click();

  await page.getByRole("button", { name: "创建项目" }).click();
  await page.getByLabel("项目名称").fill(projectName);
  await page.getByRole("button", { name: "确认创建" }).click();
  await page.getByRole("link", { name: `打开项目 ${projectName}` }).click();
  await page.getByRole("button", { name: "创建单集" }).click();
  await page.getByLabel("单集名称", { exact: true }).fill("第一集 雨巷");
  await page.getByRole("button", { name: "确认创建" }).click();
  await page.getByRole("link", { name: "进入第一集 雨巷" }).click();
  await expect(page).toHaveURL(/\/studio\/[^/]+\/script$/);
  const episodeId = new URL(page.url()).pathname.split("/")[2];

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

  const fixtures: AssetFixture[] = [
    {
      kind: "character",
      tabName: "角色",
      name: `顾清禾-${unique}`,
      mediaName: characterMediaName,
      consentReference: `fictional-character-${unique}`,
    },
    {
      kind: "location",
      tabName: "场景",
      name: `长安雨巷-${unique}`,
      mediaName: locationMediaName,
      consentReference: `fictional-location-${unique}`,
    },
    {
      kind: "voice",
      tabName: "声音",
      name: `顾清禾声线-${unique}`,
      mediaName: voiceMediaName,
      consentReference: `fictional-voice-${unique}`,
    },
  ];
  for (const fixture of fixtures) await createReadyAsset(page, fixture);

  await page.goto(`/studio/${episodeId}/assets`);
  await expect(page.getByLabel("生产摘要").getByText("3 / 3")).toBeVisible();
  await expect(page.getByText("已有 ready 版本")).toHaveCount(3);

  const character = fixtures[0];
  const renamedCharacter = `${character.name}（雨巷）`;
  await page.goto("/studio");
  await page.getByRole("tab", { name: character.tabName }).click();
  await page.getByRole("button", { name: `选择资产 ${character.name}` }).click();

  await page.getByRole("button", { name: "编辑资产身份" }).click();
  const editDialog = page.getByRole("dialog", { name: "编辑资产身份" });
  await editDialog.getByLabel("资产名称").fill(renamedCharacter);
  await editDialog.getByLabel("别名（逗号分隔）").fill("清禾，顾小姐");
  await editDialog.getByLabel("标签（逗号分隔）").fill("S2验收，雨巷主角");
  await editDialog.getByRole("button", { name: "保存身份信息" }).click();
  await expect(page.getByRole("status")).toContainText(
    `资产身份已更新：${renamedCharacter}`,
  );

  await page.getByRole("button", { name: "添加新版本" }).click();
  await fillAssetSpec(page, character);
  await page.getByLabel("参考媒体").selectOption({
    label: `${characterMediaName} · v1 · ready`,
  });
  await page.getByLabel("提示词描述").fill("雨巷造型调整，旧版本保持可追溯。");
  await page.getByRole("button", { name: "保存版本" }).click();
  await expect(page.getByRole("status")).toContainText("版本 v2 已保存");

  await page.getByRole("button", { name: "设为当前资产版本 v1" }).click();
  await expect(page.getByRole("status")).toContainText(
    "资产已切换到版本 v1；既有镜头引用保持不变。",
  );

  await page.getByRole("button", { name: "归档", exact: true }).click();
  await expect(page.getByRole("status")).toContainText("资产已归档。");
  await expect(page.getByRole("button", { name: "恢复", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "恢复", exact: true }).click();
  await expect(page.getByRole("status")).toContainText("资产已恢复。");

  await page.getByRole("button", { name: "删除资产身份" }).click();
  const deleteDialog = page.getByRole("dialog", { name: "删除资产身份" });
  await expect(deleteDialog.getByText("当前不能删除")).toBeVisible();
  await expect(deleteDialog.getByText(/2 个不可变版本/)).toBeVisible();
  await expect(
    deleteDialog.getByRole("button", { name: "确认删除空资产" }),
  ).toHaveCount(0);
  await deleteDialog.getByRole("button", { name: "取消" }).click();

  const revoked = fixtures[2];
  await page.goto("/governance");
  await page.getByRole("button", { name: revoked.consentReference }).click();
  await page.getByRole("button", { name: "撤销授权" }).click();
  await page.getByLabel("撤销原因").fill("验收回归：权利人撤回授权");
  await page.getByRole("button", { name: "确认撤销" }).click();
  await expect(page.getByRole("status")).toContainText("授权已撤销");

  await page.goto("/studio");
  await page.getByRole("tab", { name: revoked.tabName }).click();
  await expect(page.getByText("授权已撤回，新的生成与交付已被阻止")).toBeVisible();

  await page.goto(`/studio/${episodeId}/assets`);
  await expect(page.getByLabel("生产摘要").getByText("2 / 3")).toBeVisible();
  await expect(page.getByText("已有 ready 版本")).toHaveCount(2);
});
