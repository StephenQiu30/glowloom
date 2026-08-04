import { expect, test } from "@playwright/test";

test("无 Ark Key 时任务中心明确阻断生成能力且费用保持零事实", async ({
  page,
}) => {
  const unique = `${Date.now()}-${test.info().workerIndex}`;
  const projectName = `生成预检项目-${unique}`;

  await page.goto("/register");
  await page.getByLabel("显示名称").fill("生成事实验收员");
  await page.getByLabel("邮箱").fill(`generation-${unique}@example.com`);
  await page.getByLabel("密码").fill("playwright-secure-password");
  await page.getByRole("button", { name: "注册并开始创作" }).click();

  await page.getByRole("button", { name: "创建项目" }).click();
  await page.getByLabel("项目名称").fill(projectName);
  await page.getByRole("button", { name: "确认创建" }).click();
  await page.getByRole("link", { name: `打开项目 ${projectName}` }).click();

  await page.getByRole("button", { name: "创建单集" }).click();
  await page.getByLabel("单集名称").fill("第一集");
  await page.getByRole("button", { name: "确认创建" }).click();
  await page.getByRole("link", { name: "进入第一集", exact: true }).click();
  await page.getByRole("link", { name: /任务.*状态恢复与失败原因/ }).click();

  await expect(
    page.getByRole("heading", { name: "AI 生成能力与费用事实" }),
  ).toBeVisible();
  await expect(page.getByText("doubao-seedream-5-0-lite-260128")).toBeVisible();
  await expect(page.getByText("doubao-seedance-2-0-260128")).toBeVisible();
  await expect(
    page.getByText("真实账号参数、计费和权限契约尚未验收").first(),
  ).toBeVisible();
  await expect(page.getByText("还没有费用记录；生成预检不会创建预占。"))
    .toBeVisible();
  await expect(page.getByText("暂不可用")).toHaveCount(2);
});
