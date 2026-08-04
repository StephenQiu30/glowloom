import { expect, test } from "@playwright/test";

import {
  ONE_PIXEL_PNG,
  createReadyAsset,
  fillAssetSpec,
  testToneWav,
  uploadAndWait,
  type AssetFixture,
} from "./asset-support";

test("媒体、三类资产与授权准备度联合闭环", async ({ page }) => {
  test.setTimeout(120_000);
  const unique = `${Date.now()}-${test.info().workerIndex}`;
  const projectName = `S2-资产契约-${unique}`;
  const characterMediaName = `character-${unique}.png`;
  const characterMediaV2Name = `character-v2-${unique}.png`;
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
  const characterMedia = page
    .locator("article")
    .filter({ hasText: characterMediaName });
  await characterMedia
    .getByRole("button", { name: "追加媒体版本" })
    .click();
  const appendDialog = page.getByRole("dialog", { name: "追加媒体版本" });
  await appendDialog.getByLabel("选择新的媒体文件").setInputFiles({
    name: characterMediaV2Name,
    mimeType: "image/png",
    buffer: ONE_PIXEL_PNG,
  });
  await appendDialog.getByRole("button", { name: "上传为新版本" }).click();
  await expect(page.getByRole("status")).toContainText(
    `${characterMediaV2Name} 已追加为 v2`,
  );
  await expect
    .poll(
      async () => {
        await page.reload();
        return await page
          .locator("article")
          .filter({ hasText: characterMediaV2Name })
          .textContent({ timeout: 2_000 })
          .catch(() => "");
      },
      { timeout: 20_000 },
    )
    .toContain("可用");

  const versionedCharacterMedia = page
    .locator("article")
    .filter({ hasText: characterMediaV2Name });
  await versionedCharacterMedia
    .getByRole("button", { name: "设为当前媒体版本 v1" })
    .click();
  await expect(page.getByRole("status")).toContainText(
    `${characterMediaName} 已设为当前媒体版本`,
  );
  await expect(versionedCharacterMedia.getByText("当前版本 v1")).toBeVisible();

  await versionedCharacterMedia
    .getByRole("button", { name: "管理媒体版本 v1 的存储位置" })
    .click();
  const locationDialog = page.getByRole("dialog", { name: "存储位置治理" });
  await expect(locationDialog.getByText("当前读取", { exact: true })).toBeVisible();
  await locationDialog.getByRole("button", { name: "迁移当前版本" }).click();
  await expect(page.locator('[role="status"]')).toContainText(
    "位置迁移任务已创建",
  );
  await expect
    .poll(async () => locationDialog.textContent(), { timeout: 20_000 })
    .toContain("回滚保护中");
  await locationDialog.getByRole("button", { name: "回滚到此位置" }).click();
  await expect(page.locator('[role="status"]')).toContainText(
    "位置回滚任务已创建",
  );
  await expect(
    locationDialog.getByText("位置 2", { exact: true }).locator(".."),
  ).toContainText("当前读取", { timeout: 20_000 });
  await locationDialog.getByRole("button", { name: "迁移当前版本" }).click();
  await expect(page.locator('[role="status"]')).toContainText(
    "位置迁移任务已创建",
  );
  await expect(
    locationDialog.getByText(/^\u4f4d\u7f6e \d+$/),
  ).toHaveCount(3, { timeout: 20_000 });
  await locationDialog.getByRole("button", { name: "关闭" }).first().click();

  await versionedCharacterMedia
    .getByRole("button", { name: "归档媒体" })
    .click();
  await expect(page.getByRole("status")).toContainText(
    `${characterMediaName} 已归档`,
  );
  await expect(versionedCharacterMedia.getByText("已归档")).toBeVisible();
  await expect(
    versionedCharacterMedia.getByRole("button", { name: "追加媒体版本" }),
  ).toHaveCount(0);

  await versionedCharacterMedia
    .getByRole("button", { name: "恢复媒体" })
    .click();
  await expect(page.getByRole("status")).toContainText(
    `${characterMediaName} 已恢复`,
  );
  await expect(
    versionedCharacterMedia.getByRole("button", { name: "追加媒体版本" }),
  ).toBeVisible();

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
  const auditTrail = page.getByRole("region", { name: "操作审计" });
  await expect(auditTrail.getByText("授权撤销")).toBeVisible();
  await expect(auditTrail.getByText("资产更新")).toBeVisible();
  await expect(auditTrail.getByText("资产当前版本切换")).toBeVisible();
  await expect(auditTrail.getByText("资产归档")).toBeVisible();
  await expect(auditTrail.getByText("资产恢复")).toBeVisible();
  await expect(auditTrail.getByText("资产版本创建").first()).toBeVisible();
  await auditTrail.getByRole("button", { name: "筛选" }).click();
  await auditTrail.getByLabel("动作").selectOption("asset.current_changed");
  await auditTrail.getByRole("button", { name: "应用审计筛选" }).click();
  await expect(auditTrail.getByText(/1 条只追加事件/)).toBeVisible();
  await auditTrail.getByLabel("动作").selectOption("consent.revoked");
  await auditTrail.getByRole("button", { name: "应用审计筛选" }).click();
  await expect(auditTrail.getByText(/1 条只追加事件/)).toBeVisible();

  await page.goto("/studio");
  await page.getByRole("tab", { name: revoked.tabName }).click();
  await expect(page.getByText("授权已撤回，新的生成与交付已被阻止")).toBeVisible();

  await page.goto(`/studio/${episodeId}/assets`);
  await expect(page.getByLabel("生产摘要").getByText("2 / 3")).toBeVisible();
  await expect(page.getByText("已有 ready 版本")).toHaveCount(2);
});
