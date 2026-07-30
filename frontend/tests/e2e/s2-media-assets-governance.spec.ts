import { expect, test } from "@playwright/test";

const ONE_PIXEL_PNG = Buffer.from(
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=",
  "base64",
);

test("S2 媒体、资产与授权准备度联合闭环", async ({ page }) => {
  test.setTimeout(90_000);
  const unique = `${Date.now()}-${test.info().workerIndex}`;
  const projectName = `S2-资产契约-${unique}`;
  const mediaName = `character-${unique}.png`;
  const assetName = `顾清禾-${unique}`;

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
  await page.getByLabel("选择媒体文件").setInputFiles({
    name: mediaName,
    mimeType: "image/png",
    buffer: ONE_PIXEL_PNG,
  });
  await page.getByRole("button", { name: "上传并开始探测" }).click();
  await expect(page.getByRole("status")).toContainText("媒体探测任务已创建");

  await expect
    .poll(
      async () => {
        await page.reload();
        return await page
          .locator("article")
          .filter({ hasText: mediaName })
          .textContent({ timeout: 2_000 })
          .catch(() => "");
      },
      { timeout: 20_000 },
    )
    .toContain("可用");

  await page.goto("/studio");
  await expect(page.getByRole("heading", { name: "资产库" })).toBeVisible();
  await page.getByRole("button", { name: "新建资产" }).click();
  await page.getByLabel("资产名称").fill(assetName);
  await page.getByLabel("别名（逗号分隔）").fill("清禾，顾小姐");
  await page.getByLabel("标签（逗号分隔）").fill("主角，第一季");
  await page.getByRole("button", { name: "创建资产" }).click();
  await expect(page.getByRole("status")).toContainText("资产身份已创建");

  await page.getByRole("button", { name: "添加新版本" }).click();
  await page.getByLabel("身份定位").fill("女主角");
  await page.getByLabel("外观描述").fill("乌发高髻，青灰色长衫，右眼有泪痣");
  await page.getByLabel("年龄观感").fill("二十五岁");
  await page.getByLabel("性格特征（逗号分隔）").fill("清冷，克制");
  await page.getByLabel("参考媒体").selectOption({
    label: `${mediaName} · v1 · ready`,
  });
  await page.getByLabel("提示词描述").fill("保持右眼泪痣与青灰色长衫。");
  await page.getByRole("button", { name: "保存版本" }).click();
  await expect(page.getByRole("status")).toContainText("准备度为已阻断");
  await expect(page.getByText("缺少覆盖当前用途的有效授权")).toBeVisible();

  await page.getByRole("link", { name: "前往授权治理" }).click();
  await expect(page.getByRole("heading", { name: "登记新授权" })).toBeVisible();
  await page.getByLabel("权利主体引用").fill(`fictional-adult-${unique}`);
  await page.getByLabel("登记说明").fill("虚构成年角色形象用于 AI 漫剧生成与平台预览");
  await page.getByRole("button", { name: "登记授权" }).click();
  await expect(page.getByRole("status")).toContainText("授权已登记");

  await page.goto("/studio");
  await expect(page.getByRole("button", { name: `选择资产 ${assetName}` })).toBeVisible();
  await expect(page.getByText("媒体、字段与授权范围均满足当前用途")).toBeVisible();

  await page.goto("/governance");
  await page.getByRole("button", { name: "撤销授权" }).click();
  await page.getByLabel("撤销原因").fill("验收回归：权利人撤回授权");
  await page.getByRole("button", { name: "确认撤销" }).click();
  await expect(page.getByRole("status")).toContainText("授权已撤销");

  await page.goto("/studio");
  await expect(page.getByText("授权已撤回，新的生成与交付已被阻止")).toBeVisible();
});
