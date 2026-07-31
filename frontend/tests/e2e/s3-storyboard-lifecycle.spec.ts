import { execFileSync } from "node:child_process";
import path from "node:path";

import { expect, test } from "@playwright/test";

import {
  ONE_PIXEL_PNG,
  createReadyAsset,
  fillAssetSpec,
  uploadAndWait,
  type AssetFixture,
} from "./asset-support";

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

test("S3 从本地确认结构完成镜头规格与生命周期闭环", async ({ page }) => {
  test.setTimeout(120_000);
  const unique = `${Date.now()}-${test.info().workerIndex}`;
  const projectName = `S3-分镜契约-${unique}`;
  const locationMediaName = `storyboard-location-${unique}.png`;
  const locationFixture: AssetFixture = {
    kind: "location",
    tabName: "场景",
    name: `月台场景-${unique}`,
    mediaName: locationMediaName,
    consentReference: `fictional-storyboard-location-v1-${unique}`,
  };

  await page.goto("/register");
  await page.getByLabel("显示名称").fill("S3 验收创作者");
  await page.getByLabel("邮箱").fill(`s3-storyboard-${unique}@example.com`);
  await page.getByLabel("密码").fill("playwright-secure-password");
  await page.getByRole("button", { name: "注册并开始创作" }).click();

  await page.getByRole("button", { name: "创建项目" }).click();
  await page.getByLabel("项目名称").fill(projectName);
  await page.getByRole("button", { name: "确认创建" }).click();
  await page.getByRole("link", { name: `打开项目 ${projectName}` }).click();
  await page.getByRole("button", { name: "创建单集" }).click();
  await page.getByLabel("单集名称", { exact: true }).fill("第一集 分镜");
  await page.getByRole("button", { name: "确认创建" }).click();

  const episodeLink = page.getByRole("link", { name: "进入第一集 分镜" });
  const episodeHref = await episodeLink.getAttribute("href");
  expect(episodeHref).toMatch(/^\/studio\/[0-9a-f-]+\/script$/);
  const episodeId = episodeHref!.split("/")[2];
  seedConfirmedStructure(episodeId);

  await page.goto(`/studio/${episodeId}/media`);
  await uploadAndWait(page, {
    name: locationMediaName,
    mimeType: "image/png",
    buffer: ONE_PIXEL_PNG,
  });
  await createReadyAsset(page, locationFixture);
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
  await page.getByRole("button", { name: "设为当前资产版本 v1" }).click();
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
  await page.getByLabel("动作节拍").fill("林澈停下脚步\n抬头寻找声音来源");
  await page.getByRole("button", { name: /林澈.*有人吗/ }).click();
  await page
    .getByRole("region", { name: "固定资产版本" })
    .getByRole("button", { name: locationFixture.name, exact: true })
    .click();
  await page.getByLabel("首帧意图").fill("冷蓝月台全景，角色从画面右侧进入");
  await page.getByLabel("尾帧意图").fill("角色停在灯箱下方并回头");
  await page.getByRole("button", { name: "保存为新版本" }).click();
  await expect(page.getByRole("status")).toContainText("镜头规格 v1 已保存");

  await page.goto("/studio");
  await page.getByRole("tab", { name: locationFixture.tabName }).click();
  await page
    .getByRole("button", { name: `选择资产 ${locationFixture.name}` })
    .click();
  await page.getByRole("button", { name: "设为当前资产版本 v2" }).click();
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
  await expect(page.getByText("本页历史引用 1")).toBeVisible();
  await expect(page.getByText("本页当前引用 0")).toBeVisible();
  await page.getByLabel("检查引用的资产版本").selectOption({
    label: "v2（资产当前版本）",
  });
  await expect(page.getByText("本页当前引用 1")).toBeVisible();

  await page.goto(`/studio/${episodeId}/storyboard`);
  await expect(page.getByRole("button", { name: "v2 · 当前" })).toBeDisabled();
  await expect(page.getByText("当前规格可进入生产预检")).toBeVisible();
  await page.getByRole("button", { name: "v1 · 设为当前" }).click();
  await expect(page.getByRole("status")).toContainText("已切换到规格 v1");
  await page.getByLabel("镜头目的").fill("强化角色听见异响后的停顿与回望");
  await page.getByRole("button", { name: "保存为新版本" }).click();
  await expect(page.getByRole("status")).toContainText("镜头规格 v3 已保存");
  await page.getByRole("button", { name: "v2 · 设为当前" }).click();
  await expect(page.getByRole("status")).toContainText("已切换到规格 v2");

  await page.getByRole("button", { name: "复制镜头" }).click();
  await expect(page.getByRole("status")).toContainText(
    "镜头“车站警觉”已复制",
  );
  await expect(page.getByText("2 个镜头", { exact: true })).toBeVisible();
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

  await page.getByRole("button", { name: "合并" }).click();
  await page.getByRole("button", { name: "检查合并影响" }).click();
  await expect(page.getByText("影响已固定", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "确认合并" }).click();
  await expect(page.getByRole("status")).toContainText("相邻镜头已合并");
  await expect(page.getByText("1 个镜头", { exact: true })).toBeVisible();

  const mergedTitle = "车站警觉 · 副本 + 车站警觉";
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

  await page.goto("/studio");
  await page.getByRole("tab", { name: locationFixture.tabName }).click();
  await page
    .getByRole("button", { name: `选择资产 ${locationFixture.name}` })
    .click();
  await page.getByRole("button", { name: "设为当前资产版本 v1" }).click();
  await expect(page.getByText("本页当前引用 2")).toBeVisible();
  await expect(page.getByText("本页历史引用 3")).toBeVisible();
  await page.getByRole("button", { name: "全选当前引用" }).click();
  await page.getByRole("button", { name: "生成升级预检" }).click();
  const finalUpgradeDialog = page.getByRole("dialog", {
    name: "确认资产版本升级",
  });
  await expect(finalUpgradeDialog.getByText("系统将为 2 个镜头")).toBeVisible();
  await finalUpgradeDialog
    .getByRole("button", { name: "返回检查" })
    .click();
  await page.getByRole("button", { name: "设为当前资产版本 v2" }).click();
  await expect(page.getByText("本页当前引用 0")).toBeVisible();
  await expect(page.getByText("已选择 0 个当前镜头")).toBeVisible();
  await expect(
    page.getByRole("button", { name: "生成升级预检" }),
  ).toBeDisabled();
});
