import { execFileSync } from "node:child_process";
import path from "node:path";

import { expect, test } from "@playwright/test";

import {
  ONE_PIXEL_PNG,
  createReadyAsset,
  fillAssetSpec,
  setCurrentAssetVersion,
  testToneWav,
  uploadAndWait,
  type AssetFixture,
} from "./asset-support";
import { registerUser } from "./auth-support";

const e2eDatabaseUrl =
  "postgresql+asyncpg://postgres@127.0.0.1:5432/lanverse_test";

function seedConfirmedStructure(episodeId: string) {
  const backendDirectory = path.resolve(process.cwd(), "../backend");
  execFileSync(
    path.join(backendDirectory, ".venv/bin/python"),
    [
      "-m",
      "tests.support.seed_storyboard_e2e",
      "--episode-id",
      episodeId,
    ],
    {
      cwd: backendDirectory,
      env: {
        ...process.env,
        DATABASE_URL: e2eDatabaseUrl,
        DEEPSEEK_API_KEY: "",
        ENVIRONMENT: "test",
        JWT_SECRET_KEY: "playwright-only-jwt-secret-with-at-least-32-bytes",
      },
      stdio: "pipe",
    },
  );
}

function seedAssetCandidateReference(episodeId: string, assetId: string) {
  const backendDirectory = path.resolve(process.cwd(), "../backend");
  execFileSync(
    path.join(backendDirectory, ".venv/bin/python"),
    [
      "-m",
      "tests.support.seed_storyboard_e2e",
      "--episode-id",
      episodeId,
      "--asset-id",
      assetId,
    ],
    {
      cwd: backendDirectory,
      env: {
        ...process.env,
        DATABASE_URL: e2eDatabaseUrl,
        DEEPSEEK_API_KEY: "",
        ENVIRONMENT: "test",
        JWT_SECRET_KEY: "playwright-only-jwt-secret-with-at-least-32-bytes",
      },
      stdio: "pipe",
    },
  );
}

test("从本地确认结构完成镜头规格与生命周期闭环", async ({ page }) => {
  test.setTimeout(120_000);
  const unique = `${Date.now()}-${test.info().workerIndex}`;
  const projectName = `S3-分镜契约-${unique}`;
  const characterMediaName = `storyboard-character-${unique}.png`;
  const locationMediaName = `storyboard-location-${unique}.png`;
  const voiceMediaName = `storyboard-voice-${unique}.wav`;
  const characterFixture: AssetFixture = {
    kind: "character",
    tabName: "角色",
    name: `林澈角色-${unique}`,
    mediaName: characterMediaName,
    consentReference: `fictional-storyboard-character-${unique}`,
  };
  const locationFixture: AssetFixture = {
    kind: "location",
    tabName: "场景",
    name: `月台场景-${unique}`,
    mediaName: locationMediaName,
    consentReference: `fictional-storyboard-location-v1-${unique}`,
  };
  const voiceFixture: AssetFixture = {
    kind: "voice",
    tabName: "声音",
    name: `林澈声线-${unique}`,
    mediaName: voiceMediaName,
    consentReference: `fictional-storyboard-voice-${unique}`,
  };

  await registerUser(page, {
    displayName: "S3 验收创作者",
    email: `s3-storyboard-${unique}@example.com`,
  });

  await page.getByRole("button", { name: "创建项目" }).click();
  await page.getByLabel("项目名称").fill(projectName);
  await page.getByRole("button", { name: "确认创建" }).click();
  await page.getByRole("link", { name: `打开项目 ${projectName}` }).click();
  await page.getByRole("button", { name: /创建(?:第一集|单集)/ }).click();
  await page.getByLabel("单集名称", { exact: true }).fill("第一集 分镜");
  await page.getByRole("button", { name: "确认创建" }).click();

  const episodeLink = page.getByRole("link", { name: "进入第一集 分镜" });
  const episodeHref = await episodeLink.getAttribute("href");
  expect(episodeHref).toMatch(/^\/studio\/[0-9a-f-]+\/script$/);
  const episodeId = episodeHref!.split("/")[2];
  seedConfirmedStructure(episodeId);

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

  const referencedEmptyAssetName = `候选引用空角色-${unique}`;
  await page.getByRole("button", { name: "新建资产" }).click();
  const emptyAssetDialog = page.getByRole("dialog", { name: "新建资产身份" });
  await emptyAssetDialog.getByLabel("资产类型").selectOption("character");
  await emptyAssetDialog.getByLabel("资产名称").fill(referencedEmptyAssetName);
  const emptyAssetResponse = page.waitForResponse(
    (response) =>
      response.request().method() === "POST" &&
      /\/api\/v1\/projects\/[0-9a-f-]+\/assets$/.test(response.url()),
  );
  await emptyAssetDialog.getByRole("button", { name: "创建资产" }).click();
  const emptyAssetId = (await (await emptyAssetResponse).json()).data.id as string;
  await expect(page.getByRole("status")).toContainText("资产身份已创建");
  seedAssetCandidateReference(episodeId, emptyAssetId);
  await page.getByRole("button", { name: "删除资产身份" }).click();
  const deleteAssetDialog = page.getByRole("dialog", { name: "删除资产身份" });
  await expect(deleteAssetDialog).toContainText(
    "资产已被 1 条剧本候选决议关联，只能归档。",
  );
  await expect(
    deleteAssetDialog.getByRole("button", { name: "确认删除空资产" }),
  ).toHaveCount(0);
  await deleteAssetDialog.getByRole("button", { name: "关闭" }).click();

  const relatedPropName = `角色佩剑-${unique}`;
  await page.getByRole("tab", { name: "道具" }).click();
  await page.getByRole("button", { name: "新建资产" }).click();
  const propDialog = page.getByRole("dialog", { name: "新建资产身份" });
  await propDialog.getByLabel("资产类型").selectOption("prop");
  await propDialog.getByLabel("资产名称").fill(relatedPropName);
  await propDialog.getByRole("button", { name: "创建资产" }).click();
  await expect(page.getByRole("status")).toContainText("资产身份已创建");
  await page.getByRole("button", { name: "添加新版本" }).click();
  const propVersionDialog = page.getByRole("dialog", { name: "添加道具版本" });
  await propVersionDialog.getByLabel("外观描述").fill("青铜剑身，深色皮革剑柄");
  await propVersionDialog.getByLabel("材质").fill("青铜与皮革");
  await propVersionDialog.getByLabel("使用场景").fill("角色随身佩戴");
  await propVersionDialog
    .getByLabel("持有角色")
    .selectOption({ label: referencedEmptyAssetName });
  await propVersionDialog.getByRole("button", { name: "保存版本" }).click();
  await expect(page.getByRole("status")).toContainText("版本 v1 已保存");

  await page.getByRole("tab", { name: "角色" }).click();
  await page
    .getByRole("button", { name: `选择资产 ${referencedEmptyAssetName}` })
    .click();
  await page.getByRole("button", { name: "删除资产身份" }).click();
  const relatedDeleteDialog = page.getByRole("dialog", { name: "删除资产身份" });
  await expect(relatedDeleteDialog).toContainText(
    "资产已被 1 个道具或服装版本引用，只能归档。",
  );
  await expect(
    relatedDeleteDialog.getByRole("button", { name: "确认删除空资产" }),
  ).toHaveCount(0);
  await relatedDeleteDialog.getByRole("button", { name: "关闭" }).click();

  await page.getByRole("tab", { name: locationFixture.tabName }).click();
  await page
    .getByRole("button", { name: `选择资产 ${locationFixture.name}` })
    .click();

  await page.getByRole("button", { name: "添加新版本" }).click();
  await fillAssetSpec(page, locationFixture);
  await page.getByLabel("参考媒体").selectOption({
    label: `${locationMediaName} · v1 · ready`,
  });
  await page.getByLabel("提示词描述").fill("月台灯箱亮起，保留旧版本历史。");
  await page.getByRole("button", { name: "保存版本" }).click();
  await expect(page.getByRole("status")).toContainText("版本 v2 已保存");
  await expect(page.getByText("缺少覆盖当前用途的有效授权")).toBeVisible();

  await page.getByRole("link", { name: "前往授权治理" }).click();
  await page
    .getByLabel("权利主体引用")
    .fill(`fictional-storyboard-location-v2-${unique}`);
  await page.getByLabel("登记说明").fill("场景资产 v2 用于分镜升级验收");
  await page.getByRole("button", { name: "登记授权" }).click();
  await expect(page.getByRole("status")).toContainText("授权已登记");

  await page.goto("/studio");
  await page.getByRole("tab", { name: locationFixture.tabName }).click();
  await page
    .getByRole("button", { name: `选择资产 ${locationFixture.name}` })
    .click();
  await expect(page.getByText("媒体、字段与授权范围均满足当前用途")).toBeVisible();
  await setCurrentAssetVersion(page, 1);
  await expect(page.getByRole("status")).toContainText(
    "资产已切换到版本 v1；既有镜头引用保持不变。",
  );

  await page.goto(`/studio/${episodeId}/storyboard`);
  await expect(page.getByRole("heading", { name: "分镜设计" })).toBeVisible();
  await expect(page.getByText("0 个镜头", { exact: true })).toBeVisible();

  await page.getByRole("button", { name: "新建镜头" }).click();
  await page
    .getByRole("button", { name: "从候选建立 本地候选：进入车站" })
    .click();
  await expect(page.getByRole("status")).toContainText(
    "已确认候选“本地候选：进入车站”已加入镜头清单",
  );
  await expect(page.getByText("1 个镜头", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "归档镜头" }).click();
  await expect(page.getByRole("status")).toContainText(
    "镜头“本地候选：进入车站”已归档",
  );
  await expect(page.getByText("0 个镜头", { exact: true })).toBeVisible();

  await page.getByRole("button", { name: "新建镜头" }).click();
  await page.getByLabel("新镜头标题").fill("月台警觉");
  await page.getByRole("button", { name: "创建空镜头" }).click();
  await expect(page.getByRole("status")).toContainText(
    "镜头“月台警觉”已加入清单",
  );

  await page.getByRole("button", { name: "修改镜头标题" }).click();
  await page.getByRole("dialog").getByLabel("镜头标题", { exact: true }).fill(
    "车站警觉",
  );
  await page.getByRole("button", { name: "保存标题" }).click();
  await expect(page.getByRole("status")).toContainText(
    "镜头标题已更新为“车站警觉”",
  );

  await page.getByLabel("镜头目的").fill("建立角色对空旷月台的警觉反应");
  await page.getByLabel("时长（毫秒）").fill("4800");
  await page.getByLabel("连续性备注").fill("承接入站动作，保持人物朝向");
  await page.getByRole("combobox", { name: "景别" }).click();
  await page.getByRole("option", { name: "中近景" }).click();
  await page.getByRole("combobox", { name: "机位" }).click();
  await page.getByRole("option", { name: "仰拍" }).click();
  await page.getByRole("combobox", { name: "运镜" }).click();
  await page.getByRole("option", { name: "推拉" }).click();
  await page.getByLabel("构图").fill("人物位于右侧三分线，灯箱形成纵深引导");
  await page
    .getByLabel("环境", { exact: true })
    .fill("深夜旧车站月台，雨水沿顶棚滴落");
  await page.getByLabel("情绪与光线").fill("冷蓝环境光与暖色灯箱形成反差");
  await page.getByLabel("动作节拍").fill("林澈停下脚步\n抬头寻找声音来源");
  await page.getByRole("button", { name: "林澈 有人吗？", exact: true }).click();
  const fixedAssets = page.getByRole("region", { name: "固定资产版本" });
  await fixedAssets
    .getByRole("button", {
      name: `${characterFixture.name} · 基础状态`,
      exact: true,
    })
    .click();
  await page
    .getByLabel(`${characterFixture.name}画面位置`)
    .fill("画面右侧，面向月台深处");
  await fixedAssets
    .getByRole("button", {
      name: `${voiceFixture.name} · 基础状态`,
      exact: true,
    })
    .click();
  await page
    .getByRole("button", {
      name: `为对白 林澈 选择声音 ${voiceFixture.name}`,
    })
    .click();
  await page.getByLabel("林澈表演提示").fill("压低声音，短暂停顿后试探询问");
  await fixedAssets
    .getByRole("button", {
      name: `${locationFixture.name} · 基础状态`,
      exact: true,
    })
    .click();
  await page.getByLabel("环境声").fill("雨声与远处列车低鸣");
  await page.getByLabel("音效（逗号分隔）").fill("脚步声，灯箱电流声");
  await page.getByRole("combobox", { name: "生成方式" }).click();
  await page.getByRole("option", { name: "参考图转视频" }).click();
  await page.getByLabel("关键帧备注").fill("以角色与月台固定资产版本约束关键帧");
  await page.getByLabel("首帧意图").fill("冷蓝月台全景，角色从画面右侧进入");
  await page.getByLabel("尾帧意图").fill("角色停在灯箱下方并回头");
  await page.getByRole("button", { name: "保存为新版本" }).click();
  await expect(page.getByRole("status")).toContainText("镜头规格 v1 已保存");
  await page.getByRole("button", { name: "编辑来源映射" }).click();
  await page
    .getByLabel("映射叙事单元 动作：雨夜车站")
    .click();
  await page
    .getByLabel("映射叙事单元 台词：林澈：有人吗？")
    .click();
  await page.getByRole("button", { name: "保存来源映射" }).click();
  await expect(page.getByRole("status")).toContainText(
    "镜头“车站警觉”的叙事来源已保存为新规格版本",
  );

  await page.goto("/studio");
  await page.getByRole("tab", { name: locationFixture.tabName }).click();
  await page
    .getByRole("button", { name: `选择资产 ${locationFixture.name}` })
    .click();
  await setCurrentAssetVersion(page, 2);
  await expect(page.getByRole("status")).toContainText(
    "资产已切换到版本 v2；既有镜头引用保持不变。",
  );
  await page.getByRole("checkbox", { name: "选择镜头 车站警觉" }).click();
  await page.getByRole("button", { name: "生成升级预检" }).click();
  const upgradeDialog = page.getByRole("dialog", { name: "确认资产版本升级" });
  await expect(upgradeDialog.getByText("旧规格和历史引用会继续保留")).toBeVisible();
  await expect(upgradeDialog.getByText("系统将为 1 个镜头")).toBeVisible();
  await upgradeDialog
    .getByRole("button", { name: "应用升级并创建新规格版本" })
    .click();
  await expect(page.getByRole("status")).toContainText(
    "已为 1 个镜头创建新的规格版本",
  );
  await expect(page.getByText("本页历史引用 2")).toBeVisible();
  await expect(page.getByText("本页当前引用 0")).toBeVisible();
  await page.getByLabel("检查引用的资产版本").selectOption({
    label: "v2（资产当前版本）",
  });
  await expect(page.getByText("本页当前引用 1")).toBeVisible();

  await page.goto(`/studio/${episodeId}/storyboard`);
  await expect(page.getByRole("button", { name: "v3 · 当前" })).toBeDisabled();
  await expect(page.getByText("当前规格可进入生产预检")).toBeVisible();
  await page.getByRole("button", { name: "v1 · 设为当前" }).click();
  await expect(page.getByRole("status")).toContainText("已切换到规格 v1");
  await page.getByLabel("镜头目的").fill("强化角色听见异响后的停顿与回望");
  await page.getByRole("button", { name: "保存为新版本" }).click();
  await expect(page.getByRole("status")).toContainText("镜头规格 v4 已保存");
  await page.getByRole("button", { name: "v3 · 设为当前" }).click();
  await expect(page.getByRole("status")).toContainText("已切换到规格 v3");

  await page.getByRole("button", { name: "复制镜头" }).click();
  await expect(page.getByRole("status")).toContainText(
    "镜头“车站警觉”已复制",
  );
  await expect(page.getByText("2 个镜头", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "林澈 有人吗？", exact: true }).click();
  await page.getByRole("button", { name: "保存为新版本" }).click();
  await expect(page.getByRole("status")).toContainText("镜头规格 v2 已保存");
  await page.getByRole("button", { name: "编辑来源映射" }).click();
  await page
    .getByLabel("映射叙事单元 动作：雨夜车站")
    .click();
  await page
    .getByLabel("映射叙事单元 台词：林澈：有人吗？")
    .click();
  await page.getByRole("button", { name: "保存来源映射" }).click();
  await expect(page.getByRole("status")).toContainText(
    "镜头“车站警觉 · 副本”的叙事来源已保存为新规格版本",
  );
  await page
    .getByRole("button", { name: "拖动镜头 车站警觉 · 副本" })
    .dragTo(
      page.getByRole("listitem", { name: "镜头 车站警觉 顺序项" }),
    );
  await expect(page.getByRole("status")).toContainText("镜头顺序已更新");
  const shotOrder = page.getByRole("list", { name: "镜头顺序列表" });
  await expect(shotOrder.getByRole("listitem").first()).toContainText(
    "车站警觉 · 副本",
  );
  await page.getByRole("button", { name: "下移镜头" }).click();
  await expect(page.getByRole("status")).toContainText("镜头顺序已更新");
  await expect(shotOrder.getByRole("listitem").first()).toContainText(
    "车站警觉",
  );
  await page.getByRole("button", { name: "上移镜头" }).click();
  await expect(page.getByRole("status")).toContainText("镜头顺序已更新");
  await expect(shotOrder.getByRole("listitem").first()).toContainText(
    "车站警觉 · 副本",
  );
  await page.getByRole("button", { name: "下移镜头" }).click();
  await expect(page.getByRole("status")).toContainText("镜头顺序已更新");
  await expect(shotOrder.getByRole("listitem").first()).toContainText(
    "车站警觉",
  );

  await page.getByRole("button", { name: "合并" }).click();
  await page.getByRole("button", { name: "检查合并影响" }).click();
  await expect(page.getByText("影响已固定", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "确认合并" }).click();
  await expect(page.getByRole("status")).toContainText("相邻镜头已合并");
  await expect(page.getByText("1 个镜头", { exact: true })).toBeVisible();

  const mergedTitle = "车站警觉 + 车站警觉 · 副本";
  await page.getByRole("button", { name: "归档镜头" }).click();
  await expect(page.getByRole("status")).toContainText(`镜头“${mergedTitle}”已归档`);
  await page.getByRole("button", { name: `恢复${mergedTitle}` }).click();
  await expect(page.getByRole("status")).toContainText("已恢复到清单末尾");

  await page.getByRole("button", { name: "新建镜头" }).click();
  await page.getByLabel("新镜头标题").fill("待删除空镜头");
  await page.getByRole("button", { name: "创建空镜头" }).click();
  await page.getByRole("button", { name: "删除检查" }).click();
  await page.getByRole("button", { name: "检查删除条件" }).click();
  await expect(page.getByText("可以永久删除", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "永久删除空镜头" }).click();
  await expect(page.getByRole("status")).toContainText(
    "空镜头“待删除空镜头”已永久删除",
  );

  await page.getByRole("button", { name: "拆分" }).click();
  await page.getByRole("button", { name: "检查拆分影响" }).click();
  await expect(page.getByText("影响已固定", { exact: true })).toBeVisible();
  await expect(
    page.getByText("前段包含 2 个动作和 1 条对白；其余内容全部进入后段。"),
  ).toBeVisible();
  await page
    .getByLabel("叙事来源 雨夜车站 分配到后段")
    .click();
  await page
    .getByLabel("叙事来源 林澈：有人吗？ 分配到前段")
    .click();
  await page.getByRole("button", { name: "确认拆分" }).click();
  await expect(page.getByRole("status")).toContainText("镜头已拆分为两个目标");
  await expect(page.getByText("2 个镜头", { exact: true })).toBeVisible();

  await page.reload();
  await expect(
    page
      .getByRole("list", { name: "镜头顺序列表" })
      .getByRole("listitem"),
  ).toHaveCount(2);
  await expect(
    shotOrder.getByText(`${mergedTitle} · 前段`, { exact: true }),
  ).toBeVisible();
  await expect(
    shotOrder.getByText(`${mergedTitle} · 后段`, { exact: true }),
  ).toBeVisible();
  await expect(page.getByText("0 个阻塞", { exact: true })).toBeVisible();
  await expect(page.getByText("2 可生成", { exact: true })).toBeVisible();
  await expect(page.getByLabel("构图")).toHaveValue(
    /人物位于右侧三分线，灯箱形成纵深引导/,
  );
  await expect(page.getByLabel("动作节拍")).toHaveValue(
    "林澈停下脚步\n抬头寻找声音来源",
  );
  await expect(
    page.getByLabel(`${characterFixture.name}画面位置`),
  ).toHaveValue("画面右侧，面向月台深处");
  await expect(page.getByLabel("林澈表演提示")).toHaveValue(
    "压低声音，短暂停顿后试探询问",
  );
  await expect(page.getByLabel("环境声")).toHaveValue(/雨声与远处列车低鸣/);
  await expect(page.getByRole("combobox", { name: "生成方式" })).toHaveText(
    "参考图转视频",
  );

  await page.goto("/studio");
  await page.getByRole("tab", { name: locationFixture.tabName }).click();
  await page
    .getByRole("button", { name: `选择资产 ${locationFixture.name}` })
    .click();
  await setCurrentAssetVersion(page, 1);
  await expect(page.getByText("本页当前引用 2")).toBeVisible();
  await expect(page.getByText("本页历史引用 5")).toBeVisible();
  await page.getByRole("button", { name: "全选当前引用" }).click();
  await page.getByRole("button", { name: "生成升级预检" }).click();
  const finalUpgradeDialog = page.getByRole("dialog", {
    name: "确认资产版本升级",
  });
  await expect(finalUpgradeDialog.getByText("系统将为 2 个镜头")).toBeVisible();
  await finalUpgradeDialog
    .getByRole("button", { name: "返回检查" })
    .click();
  await setCurrentAssetVersion(page, 2);
  await expect(page.getByText("本页当前引用 0")).toBeVisible();
  await expect(page.getByText("已选择 0 个当前镜头")).toBeVisible();
  await expect(
    page.getByRole("button", { name: "生成升级预检" }),
  ).toBeDisabled();

  await page.goto(`/studio/${episodeId}/script`);
  await page.getByRole("button", { name: "设为当前 v1" }).click();
  const scriptImpactDialog = page.getByRole("dialog", {
    name: "版本切换影响",
  });
  await expect(
    scriptImpactDialog.getByText("2 个镜头仍引用其他剧本版本"),
  ).toBeVisible();
  await expect(
    scriptImpactDialog.getByText(
      "当前指针已经切换；系统没有修改任何既有镜头或规格版本。",
    ),
  ).toBeVisible();
  await scriptImpactDialog.getByRole("button", { name: "知道了" }).click();
  await page.getByRole("button", { name: "设为当前 v2" }).click();
  const restoredImpactDialog = page.getByRole("dialog", {
    name: "版本切换影响",
  });
  await expect(
    restoredImpactDialog.getByText("0 个镜头仍引用其他剧本版本"),
  ).toBeVisible();
  await restoredImpactDialog.getByRole("button", { name: "知道了" }).click();

  await page.goto("/governance");
  const auditTrail = page.getByRole("region", { name: "操作审计" });
  await auditTrail.getByRole("button", { name: "筛选" }).click();
  await auditTrail
    .getByLabel("动作")
    .selectOption("shot.spec_version_created");
  await auditTrail.getByRole("button", { name: "应用审计筛选" }).click();
  await expect(auditTrail.getByText(/8 条只追加事件/)).toBeVisible();
  await expect(
    auditTrail.locator("article").first().getByText("分镜规格版本创建"),
  ).toBeVisible();

  await auditTrail
    .getByLabel("动作")
    .selectOption("shot.current_spec_changed");
  await auditTrail.getByRole("button", { name: "应用审计筛选" }).click();
  await expect(auditTrail.getByText(/2 条只追加事件/)).toBeVisible();
  await expect(
    auditTrail.locator("article").first().getByText("分镜当前规格切换"),
  ).toBeVisible();

  await page.goto(`/studio/${episodeId}/storyboard`);
  const episodeSummary = page.getByRole("region", { name: "生产摘要" });
  await expect(
    episodeSummary.getByText("Ready 分镜").locator(".."),
  ).toContainText("2 / 2");
  await expect(page.getByText(/服务端计算 90%/)).toBeVisible();

  const exportCard = page
    .locator('[data-slot="card"]')
    .filter({ hasText: "可信分镜包" });
  await exportCard.getByRole("button", { name: "检查导出条件" }).click();
  await expect(page.getByRole("status")).toContainText("导出预检通过");
  await expect(exportCard.getByLabel("分镜包预检结果")).toContainText(
    "预检结果：可导出",
  );
  await exportCard.getByRole("button", { name: "生成分镜包" }).click();
  await expect(page.getByRole("status")).toContainText("可信分镜包任务已创建");
  await expect(exportCard.getByText("可下载", { exact: true })).toBeVisible({
    timeout: 30_000,
  });
  await expect(
    exportCard.getByRole("button", { name: "下载分镜包" }),
  ).toBeEnabled();

  await page.goto("/projects");
  await page.getByRole("link", { name: `打开项目 ${projectName}` }).click();
  const projectSummary = page.getByRole("region", { name: "项目生产摘要" });
  await expect(
    projectSummary.getByText("Ready 分镜").locator(".."),
  ).toContainText("2");
  await page.getByRole("button", { name: "检查删除 第一集 分镜" }).click();
  await expect(
    page.getByText("单集已有 6 个分镜镜头（10 个规格版本）"),
  ).toBeVisible();
  await page.getByRole("button", { name: "检查项目删除条件" }).click();
  await expect(
    page.getByText("项目关联 6 个分镜镜头（10 个规格版本）"),
  ).toBeVisible();
  await expect(
    page.getByText("项目已有 5 个资产（5 个版本）"),
  ).toBeVisible();
});
