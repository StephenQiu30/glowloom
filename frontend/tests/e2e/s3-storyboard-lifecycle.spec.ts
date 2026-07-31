import { execFileSync } from "node:child_process";
import path from "node:path";

import { expect, test } from "@playwright/test";

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
  test.setTimeout(60_000);
  const unique = `${Date.now()}-${test.info().workerIndex}`;
  const projectName = `S3-分镜契约-${unique}`;

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

  await page.goto(`/studio/${episodeId}/storyboard`);
  await expect(page.getByRole("heading", { name: "分镜设计" })).toBeVisible();
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
  await page.getByLabel("首帧意图").fill("冷蓝月台全景，角色从画面右侧进入");
  await page.getByLabel("尾帧意图").fill("角色停在灯箱下方并回头");
  await page.getByRole("button", { name: "保存为新版本" }).click();
  await expect(page.getByRole("status")).toContainText("镜头规格 v1 已保存");

  await page.getByLabel("镜头目的").fill("强化角色听见异响后的停顿与回望");
  await page.getByRole("button", { name: "保存为新版本" }).click();
  await expect(page.getByRole("status")).toContainText("镜头规格 v2 已保存");
  await page.getByRole("button", { name: "v1 · 设为当前" }).click();
  await expect(page.getByRole("status")).toContainText("已切换到规格 v1");

  await page.getByRole("button", { name: "复制镜头" }).click();
  await expect(page.getByRole("status")).toContainText(
    "镜头“车站警觉”已复制",
  );
  await expect(page.getByText("2 个镜头", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "上移镜头" }).click();
  await expect(page.getByRole("status")).toContainText("镜头顺序已更新");
  const shotOrder = page.getByRole("list", { name: "镜头顺序列表" });
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
  await expect(page.getByText("2 个阻塞", { exact: true })).toBeVisible();
});
