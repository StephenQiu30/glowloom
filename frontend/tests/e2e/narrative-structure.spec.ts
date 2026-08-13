import { expect, test } from "@playwright/test";

import { registerUser } from "./auth-support";

test("发布剧本后人工修正稳定叙事单元", async ({ page }) => {
  test.setTimeout(60_000);
  const unique = `${Date.now()}-${test.info().workerIndex}`;
  const projectName = `MVP-A-叙事契约-${unique}`;

  await registerUser(page, {
    displayName: "MVP-A 叙事验收创作者",
    email: `mvp-a-narrative-${unique}@example.com`,
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
  await page
    .getByLabel("剧本文本")
    .fill(
      [
        "第一集",
        "内景·雾港控制室·夜",
        "林澜：封锁港口，先救孩子。",
        "警报突然响起，屏幕显示闸门失控。",
      ].join("\n"),
    );
  await page.getByRole("button", { name: "导入剧本" }).click();
  await page.getByRole("button", { name: "发布新版本" }).click();
  await expect(page.getByRole("status")).toContainText("已发布并设为当前版本");

  const structure = page.getByText("稳定叙事单元", { exact: true });
  await expect(structure).toBeVisible();
  await expect(page.getByText("3 个稳定单元")).toBeVisible();
  const dependency = page.getByText(/^dependency /);
  const previousDependency = await dependency.textContent();

  await page.getByLabel("必须被分镜覆盖").first().uncheck();
  await page.getByRole("button", { name: "保存结构修正" }).click();
  await expect(page.getByRole("status")).toContainText("叙事结构已追加 revision 2");
  await expect(page.getByText("结构 revision 2")).toBeVisible();
  await expect(dependency).not.toHaveText(previousDependency ?? "");

  await page.reload();
  await expect(page.getByText("结构 revision 2")).toBeVisible();
  await expect(page.getByLabel("必须被分镜覆盖").first()).not.toBeChecked();
});
