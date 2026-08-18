import { expect, test } from "@playwright/test";

import { registerUser } from "./auth-support";

test("剧本改写无 Provider 时保留原稿并可恢复", async ({ page }) => {
  test.setTimeout(60_000);
  const unique = `${Date.now()}-${test.info().workerIndex}`;
  const projectName = `MVP-A-改写契约-${unique}`;
  const original = [
    "第一集",
    "内景·雾港控制室·夜",
    "林澜：封锁港口，先救孩子。",
    "警报突然响起，屏幕显示闸门失控。",
  ].join("\n");

  await registerUser(page, {
    displayName: "MVP-A 改写验收创作者",
    email: `mvp-a-adaptation-${unique}@example.com`,
  });
  await page.getByRole("button", { name: "创建项目" }).click();
  await page.getByLabel("项目名称").fill(projectName);
  await page.getByRole("button", { name: "确认创建" }).click();
  await page.getByRole("link", { name: `打开项目 ${projectName}` }).click();
  await page.getByRole("button", { name: /创建(?:第一集|单集)/ }).click();
  await page.getByLabel("单集名称", { exact: true }).fill("第一集 闸门失控");
  await page.getByRole("button", { name: "确认创建" }).click();
  await page.getByRole("link", { name: "进入第一集 闸门失控" }).click();

  await page.getByLabel("剧本标题").fill("雾港倒计时");
  await page.getByLabel("权利声明").fill("原创工程样例，仅用于 MVP-A 验收");
  await page.getByLabel("剧本文本").fill(original);
  await page.getByRole("button", { name: "导入剧本" }).click();
  await page.getByRole("button", { name: "发布新版本" }).click();
  await expect(page.getByRole("status")).toContainText("已发布并设为当前版本");

  await page
    .getByLabel("必须保留的核心情节")
    .fill("林澜封锁港口\n先救孩子\n结尾闸门失控");
  await page.getByRole("button", { name: "生成改写候选" }).click();
  await expect(page.getByRole("status")).toContainText("剧本改写任务已创建", {
    timeout: 20_000,
  });
  await expect(page.getByText(/ai_service_unavailable/)).toBeVisible({
    timeout: 20_000,
  });
  await expect(page.getByLabel("当前剧本文本")).toHaveValue(original);

  await page.reload();
  await expect(page.getByText(/ai_service_unavailable/)).toBeVisible({
    timeout: 15_000,
  });
  await expect(page.getByLabel("当前剧本文本")).toHaveValue(original);
  await page.getByRole("button", { name: "新建改写" }).click();
  await expect(
    page.getByRole("button", { name: "生成改写候选" }),
  ).toBeVisible();
});
