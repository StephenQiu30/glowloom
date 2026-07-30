import { expect, test } from "@playwright/test";

test("S2 真实 DeepSeek 提取、人工决议并确认结构", async ({ page }) => {
  test.setTimeout(180_000);
  const unique = `${Date.now()}-${test.info().workerIndex}`;
  const projectName = `S2-DeepSeek-联合契约-${unique}`;

  await page.goto("/register");
  await page.waitForLoadState("networkidle");
  await page.getByLabel("显示名称").fill("S2 DeepSeek 验收创作者");
  await page.getByLabel("邮箱").fill(`s2-deepseek-${unique}@example.com`);
  await page.getByLabel("密码").fill("playwright-secure-password");
  await page.getByRole("button", { name: "注册并开始创作" }).click();

  await page.getByRole("button", { name: "创建项目" }).click();
  await page.getByLabel("项目名称").fill(projectName);
  await page.getByRole("button", { name: "确认创建" }).click();
  await page.getByRole("link", { name: `打开项目 ${projectName}` }).click();
  await page.getByRole("button", { name: "创建单集" }).click();
  await page.getByLabel("单集名称", { exact: true }).fill("第一集 雨巷来信");
  await page.getByRole("button", { name: "确认创建" }).click();
  await page.getByRole("link", { name: "进入第一集 雨巷来信" }).click();

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
    .getByText("提取候选", { exact: true })
    .locator("xpath=../..");
  await expect(candidateCard.getByText(/项建议 · 已完成/)).toBeVisible({
    timeout: 150_000,
  });

  const sceneCandidate = candidateCard
    .locator("article")
    .filter({ has: page.getByText("场次", { exact: true }) })
    .first();
  const dialogueCandidate = candidateCard
    .locator("article")
    .filter({ has: page.getByText("对白", { exact: true }) })
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
    .filter({ has: page.getByText("必需", { exact: true }) })
    .filter({ has: page.getByText("pending", { exact: true }) });
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
  await expect(page.getByRole("status")).toContainText("已确认结构的剧本版本已设为当前入口");

  await page.reload();
  await expect(page.getByText("结构已确认", { exact: true })).toBeVisible({
    timeout: 15_000,
  });
  await expect(page.getByLabel("当前剧本文本")).toHaveValue(/明天之前/);
});
